package xgorm

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/v3/xconfig"
	"github.com/xiaoshicae/xone/v3/xerror"
	"github.com/xiaoshicae/xone/v3/xhook"
	"github.com/xiaoshicae/xone/v3/xtrace"
	"github.com/xiaoshicae/xone/v3/xutil"

	stdMysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

const (
	defaultClientName = "__default_client__"

	// dsnMask DSN 脱敏后的占位符
	dsnMask = "***"
	// dsnRedacted DSN 无法解析时的整串占位符
	dsnRedacted = "[redacted dsn]"

	// pingAttempts 建连验证的尝试次数
	pingAttempts = 3
	// pingRetryInterval 建连验证的重试间隔
	pingRetryInterval = time.Second
	// defaultPingTimeout 无法从配置推算时单次 Ping 的兜底超时
	defaultPingTimeout = time.Second
)

var (
	clientMap = make(map[string]*gorm.DB)
	clientMu  sync.RWMutex
)

func init() {
	xhook.BeforeStart(initXGorm)
	xhook.BeforeStop(closeXGorm)
}

func initXGorm() error {
	if !xconfig.ContainKey(XGormConfigKey) {
		xutil.WarnIfEnableDebug("XOne init %s failed, config key [%s] not exists", XGormConfigKey, XGormConfigKey)
		return nil
	}

	if xutil.IsSlice(xconfig.GetConfig(XGormConfigKey)) {
		return initMulti()
	}

	return initSingle()
}

func initSingle() error {
	config, err := getConfig()
	if err != nil {
		return xerror.Newf("xgorm", "init", "getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne init %s got config: %s", XGormConfigKey, xutil.ToJsonString(sanitizeConfigForLog(config)))

	client, err := newClient(config)
	if err != nil {
		return xerror.Newf("xgorm", "init", "newClient failed, err=[%v]", err)
	}

	setDefault(client)

	if config.metricEnabled() {
		registerPoolMetrics()
	}
	return nil
}

func initMulti() error {
	configs, err := getMultiConfig()
	if err != nil {
		return xerror.Newf("xgorm", "init", "getMultiConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne init %s got config: %s", XGormConfigKey, xutil.ToJsonString(sanitizeConfigsForLog(configs)))

	// 先创建所有 client，部分失败时回滚已创建的连接
	created := make([]*gorm.DB, 0, len(configs))
	for idx, config := range configs {
		client, err := newClient(config)
		if err != nil {
			// 回滚：关闭已创建的连接，并从 clientMap 中摘除
			// 只关不摘的话，回滚到 BeforeStop 执行之间 C() 会返回已关闭的 client
			for _, c := range created {
				closeGormDB(c)
			}
			removeClients(configs[:idx])
			return xerror.Newf("xgorm", "init", "newClient failed, name=[%v], err=[%v]", config.Name, err)
		}

		created = append(created, client)
		set(config.Name, client)

		// 第一个client为C()默认获取的client
		if idx == 0 {
			setDefault(client)
		}
	}

	// 指标是进程级的单个 collector，只要有任一 client 开启就注册
	for _, config := range configs {
		if config.metricEnabled() {
			registerPoolMetrics()
			break
		}
	}
	return nil
}

func closeXGorm() error {
	clientMu.Lock()
	defer clientMu.Unlock()

	// 用于去重，避免同一个 *gorm.DB 被关闭多次（multi模式下default指向第一个named client）
	closed := make(map[*gorm.DB]struct{})
	var errs []error

	for _, client := range clientMap {
		if _, ok := closed[client]; ok {
			continue
		}
		closed[client] = struct{}{}

		db, err := client.DB()
		if err != nil {
			errs = append(errs, xerror.Newf("xgorm", "close", "get underlying db failed, err=[%v]", err))
			continue
		}
		if err := db.Close(); err != nil {
			errs = append(errs, xerror.Newf("xgorm", "close", "close db failed, err=[%v]", err))
		}
	}
	clear(clientMap)
	return errors.Join(errs...)
}

func get(name ...string) *gorm.DB {
	n := defaultClientName
	if len(name) > 0 {
		n = name[0]
	}

	clientMu.RLock()
	defer clientMu.RUnlock()
	return clientMap[n]
}

func set(name string, client *gorm.DB) {
	clientMu.Lock()
	defer clientMu.Unlock()
	clientMap[name] = client
}

func setDefault(client *gorm.DB) {
	clientMu.Lock()
	defer clientMu.Unlock()
	clientMap[defaultClientName] = client
}

// removeClients 把指定配置对应的 client 从 clientMap 中摘除，用于初始化失败回滚
func removeClients(configs []*Config) {
	clientMu.Lock()
	defer clientMu.Unlock()
	for _, c := range configs {
		delete(clientMap, c.Name)
	}
	delete(clientMap, defaultClientName)
}

func newClient(c *Config) (*gorm.DB, error) {
	dialector, err := resolveDialector(c)
	if err != nil {
		return nil, xerror.Newf("xgorm", "newClient", "invoke resolveDialector failed, err=[%v]", err)
	}

	gormConfig := &gorm.Config{}
	if c.EnableLog {
		gormConfig.Logger = newGormLogger(c)
	}
	client, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		// gorm.Open 内部已经建好连接池才做自动 ping，失败时它不会关掉那个池子，
		// 留下的 database/sql 连接开启协程再也不会退出
		closeGormDB(client)
		return nil, xerror.Newf("xgorm", "newClient", "invoke gorm.Open failed, err=[%v]", err)
	}

	// 此后任一步失败都必须关掉连接池，否则每次建连失败泄漏一个常驻协程
	ok := false
	defer func() {
		if !ok {
			closeGormDB(client)
		}
	}()

	db, err := client.DB()
	if err != nil {
		return nil, xerror.Newf("xgorm", "newClient", "invoke client.DB failed, err=[%v]", err)
	}

	// 连接池参数配置
	db.SetMaxOpenConns(c.MaxOpenConns)
	db.SetMaxIdleConns(c.maxIdleConns())
	db.SetConnMaxLifetime(xutil.ToDuration(c.MaxLifetime))
	db.SetConnMaxIdleTime(xutil.ToDuration(c.MaxIdleTime))

	if err = pingWithRetry(db, c); err != nil {
		return nil, xerror.Newf("xgorm", "newClient", "invoke db.PingContext failed, err=[%v]", err)
	}

	if xtrace.TraceEnabled() {
		if err := client.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
			return nil, xerror.Newf("xgorm", "newClient", "use tracing.NewPlugin failed, err=[%v]", err)
		}
	}

	ok = true
	return client, nil
}

// pingWithRetry 建连验证，失败按固定间隔重试
//
// 用带 context 的重试：初始化跑在 BeforeStart 里，不可中断的重试会让
// 启动阶段收到的退出信号必须等满 attempts×sleep 才生效
func pingWithRetry(db *sql.DB, c *Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), pingTotalBudget(c))
	defer cancel()

	timeout := pingTimeout(c)
	return xutil.RetryWithContext(ctx, func(ctx context.Context) error {
		pingCtx, pingCancel := context.WithTimeout(ctx, timeout)
		defer pingCancel()
		return db.PingContext(pingCtx)
	}, pingAttempts, pingRetryInterval)
}

// pingTimeout 单次 Ping 的超时
//
// 不能只用 DialTimeout：Ping 的耗时是建连加一个往返，
// 拿建连预算当整体预算，会让连接刚建成就判超时
func pingTimeout(c *Config) time.Duration {
	d := xutil.ToDuration(c.DialTimeout)
	if c.GetDriver() == DriverMySQL {
		d += xutil.ToDuration(c.MySQL.ReadTimeout)
	}
	if d <= 0 {
		return defaultPingTimeout
	}
	return d
}

// pingTotalBudget 建连验证的总预算，兜住整轮重试的最坏耗时
func pingTotalBudget(c *Config) time.Duration {
	return pingTimeout(c)*pingAttempts + pingRetryInterval*(pingAttempts-1)
}

// closeGormDB 关闭 gorm 实例底层的连接池，用于建连失败时兜底
func closeGormDB(client *gorm.DB) {
	if client == nil {
		return
	}
	db, err := client.DB()
	if err != nil || db == nil {
		return
	}
	if cerr := db.Close(); cerr != nil {
		xutil.WarnIfEnableDebug("XOne xgorm close db after failed init got error, err=[%v]", cerr)
	}
}

// resolveDialector 根据 driver 类型返回对应的 gorm dialector
func resolveDialector(c *Config) (gorm.Dialector, error) {
	if c == nil {
		return nil, xerror.Newf("xgorm", "resolveDialector", "config can't be empty")
	}

	if c.DSN == "" {
		return nil, xerror.Newf("xgorm", "resolveDialector", "dsn can't be empty")
	}

	switch c.GetDriver() {
	case DriverMySQL:
		resolvedDSN, err := resolveMySQLDSN(c)
		if err != nil {
			return nil, xerror.Newf("xgorm", "resolveDialector", "resolve mysql dsn failed, err=[%v]", err)
		}
		xutil.InfoIfEnableDebug("XOne initXGorm newClient resolve MySQL DSN: %s", sanitizeDSN(resolvedDSN))
		return mysql.Open(resolvedDSN), nil

	case DriverPostgres:
		resolvedDSN, err := resolvePostgresDSN(c)
		if err != nil {
			return nil, xerror.Newf("xgorm", "resolveDialector", "resolve postgres dsn failed, err=[%v]", err)
		}
		xutil.InfoIfEnableDebug("XOne initXGorm newClient resolve Postgres DSN: %s", sanitizeDSN(resolvedDSN))
		return postgres.Open(resolvedDSN), nil

	default:
		return nil, xerror.Newf("xgorm", "resolveDialector", "unsupported driver: %s, supported: mysql, postgres", c.GetDriver())
	}
}

// resolveMySQLDSN 根据config构建MySQL DSN
// DSN协议: [username[:password]@][protocol[(address)]]/dbname[?param1=value1&param2=value2&...]
// 用户在 DSN 中显式写的 timeout/readTimeout/writeTimeout 不会被覆盖
func resolveMySQLDSN(c *Config) (string, error) {
	mysqlConfig, err := stdMysql.ParseDSN(c.DSN)
	if err != nil {
		// 驱动的解析错误可能回显 DSN 片段，不能原样外传
		return "", xerror.Newf("xgorm", "resolveMySQLDSN", "parse dsn failed, dsn=[%s], err=[%v]", sanitizeDSN(c.DSN), sanitizeDSN(err.Error()))
	}

	if mysqlConfig.ReadTimeout == 0 && c.MySQL.ReadTimeout != "" {
		mysqlConfig.ReadTimeout = xutil.ToDuration(c.MySQL.ReadTimeout)
	}

	if mysqlConfig.WriteTimeout == 0 && c.MySQL.WriteTimeout != "" {
		mysqlConfig.WriteTimeout = xutil.ToDuration(c.MySQL.WriteTimeout)
	}

	if mysqlConfig.Timeout == 0 && c.DialTimeout != "" {
		mysqlConfig.Timeout = xutil.ToDuration(c.DialTimeout)
	}

	return mysqlConfig.FormatDSN(), nil
}

// resolvePostgresDSN 根据config把 PG 相关超时/参数注入到 DSN 中
//
// 规则：
//  1. 支持 URL 格式（postgres://... 或 postgresql://...）与 key=value 格式两种 DSN
//  2. 用户在 DSN 中显式写的 key 不会被覆盖（字段值仅作为默认值）
//  3. DialTimeout    → connect_timeout（向上取整为秒）
//  4. StatementTimeout/LockTimeout/IdleInTxTimeout → 对应 PG GUC（毫秒）
//  5. Postgres.Params 中任意 key 直通注入
func resolvePostgresDSN(c *Config) (string, error) {
	injects := make(map[string]string)

	if c.DialTimeout != "" {
		if v := durationToSeconds(c.DialTimeout); v != "" {
			injects["connect_timeout"] = v
		}
	}

	pg := c.Postgres
	if pg.StatementTimeout != "" {
		if v := durationToMillis(pg.StatementTimeout); v != "" {
			injects["statement_timeout"] = v
		}
	}
	if pg.LockTimeout != "" {
		if v := durationToMillis(pg.LockTimeout); v != "" {
			injects["lock_timeout"] = v
		}
	}
	if pg.IdleInTxTimeout != "" {
		if v := durationToMillis(pg.IdleInTxTimeout); v != "" {
			injects["idle_in_transaction_session_timeout"] = v
		}
	}
	// 用户的 Params 优先级高于字段默认值：同 key 时以 Params 为准
	for k, v := range pg.Params {
		if v != "" {
			injects[k] = v
		}
	}

	return injectPostgresDSN(c.DSN, injects)
}

// injectPostgresDSN 根据 DSN 格式（URL 或 key=value）把 injects 中的键值追加到 DSN
// DSN 中已存在的 key 不会被覆盖
func injectPostgresDSN(dsn string, injects map[string]string) (string, error) {
	if len(injects) == 0 {
		return dsn, nil
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return injectPostgresURL(dsn, injects)
	}
	return injectPostgresKV(dsn, injects), nil
}

// injectPostgresURL 向 URL 格式 DSN 的 query string 追加参数，已存在的 key 保留
func injectPostgresURL(dsn string, injects map[string]string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for _, k := range slices.Sorted(maps.Keys(injects)) {
		if _, exists := q[k]; exists {
			continue
		}
		q.Set(k, injects[k])
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// pgKeyRegex 匹配 key=value 格式 DSN 中的 key
// 简化实现：不处理 value 内部含 key= 子串的极端情况（生产中极少见）
var pgKeyRegex = regexp.MustCompile(`(?:^|\s)([a-zA-Z_][a-zA-Z0-9_]*)=`)

// injectPostgresKV 向 key=value 格式 DSN 追加参数，已存在的 key 保留
func injectPostgresKV(dsn string, injects map[string]string) string {
	existing := make(map[string]struct{})
	for _, m := range pgKeyRegex.FindAllStringSubmatch(dsn, -1) {
		existing[m[1]] = struct{}{}
	}

	var sb strings.Builder
	sb.WriteString(dsn)
	// 只跟踪末字符而不是每轮 sb.String()：后者每次都会把整串复制一遍，
	// 拼 n 个参数就是 O(n²)
	needSpace := len(dsn) > 0 && !strings.HasSuffix(dsn, " ")
	for _, k := range slices.Sorted(maps.Keys(injects)) {
		if _, dup := existing[k]; dup {
			continue
		}
		if needSpace {
			sb.WriteByte(' ')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(quotePostgresKVValue(injects[k]))
		needSpace = true
	}
	return sb.String()
}

// quotePostgresKVValue 如果 value 含空格/单引号/反斜杠，按 libpq 规则加引号并转义
func quotePostgresKVValue(v string) string {
	if v == "" {
		return "''"
	}
	if !strings.ContainsAny(v, " '\\") {
		return v
	}
	escaped := strings.ReplaceAll(v, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}

// durationToSeconds 把时长字符串向上取整为整数秒字符串（供 PG connect_timeout 使用）
// <= 0 的时长返回空串，由调用方决定是否注入
// < 1s 的时长向上取整为 1s（libpq 要求整数秒，最小 1）
func durationToSeconds(s string) string {
	d := xutil.ToDuration(s)
	if d <= 0 {
		return ""
	}
	secs := max(int64(math.Ceil(d.Seconds())), 1)
	return strconv.FormatInt(secs, 10)
}

// durationToMillis 把时长字符串转为毫秒整数字符串（供 PG statement_timeout 等 GUC 使用）
// <= 0 的时长返回空串
func durationToMillis(s string) string {
	d := xutil.ToDuration(s)
	if d <= 0 {
		return ""
	}
	return strconv.FormatInt(d.Milliseconds(), 10)
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XGormConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	if c.DSN == "" {
		return nil, xerror.Newf("xgorm", "getConfig", "config XGorm.DSN can not be empty")
	}
	return c, nil
}

func getMultiConfig() ([]*Config, error) {
	var multiConfig []*Config
	if err := xconfig.UnmarshalConfig(XGormConfigKey, &multiConfig); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(multiConfig))
	for i, c := range multiConfig {
		multiConfig[i] = configMergeDefault(c)
		c = multiConfig[i]
		if c.DSN == "" {
			return nil, xerror.Newf("xgorm", "getMultiConfig", "multi config XGorm.DSN can not be empty")
		}
		if c.Name == "" {
			return nil, xerror.Newf("xgorm", "getMultiConfig", "multi config XGorm.Name can not be empty")
		}
		if c.Name == defaultClientName {
			return nil, xerror.Newf("xgorm", "getMultiConfig", "multi config XGorm.Name can not be reserved name [%s]", defaultClientName)
		}
		if _, ok := seen[c.Name]; ok {
			return nil, xerror.Newf("xgorm", "getMultiConfig", "multi config XGorm.Name [%s] is duplicated", c.Name)
		}
		seen[c.Name] = struct{}{}
	}
	return multiConfig, nil
}

// sanitizeDSN 对 DSN 中的密码进行脱敏处理
//
// 按 DSN 的实际格式分派，而不是「看见 @ 就当成 URL」——
// key=value 格式的 DSN 里 @ 很常见（如 application_name=svc@cluster），
// 按 @ 判断会让整串 DSN 连同 password= 原样进入日志
func sanitizeDSN(dsn string) string {
	if isURLDSN(dsn) {
		return sanitizeURLDSN(dsn)
	}
	if strings.Contains(dsn, "=") {
		return sanitizeKVDSN(dsn)
	}
	// 既不是 URL 也不含 key=value，可能是 MySQL 的 user:pass@tcp(...)/db 形式
	return sanitizeMySQLDSN(dsn)
}

// isURLDSN 判断是否为 scheme://... 形式
func isURLDSN(dsn string) bool {
	i := strings.Index(dsn, "://")
	if i <= 0 {
		return false
	}
	for _, r := range dsn[:i] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.') {
			return false
		}
	}
	return true
}

// sanitizeURLDSN 脱敏 URL 形式 DSN 的 userinfo 与 query 中的密码
//
// 用 url.Parse 而非手工找冒号：密码里含 @ 时（p@ssw0rd），
// 按第一个 @ 切分会把密码的后半段留在日志里
func sanitizeURLDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		// 解析不了就整串遮掉，宁可少一条排查信息，也不能把凭证漏出去
		return dsnRedacted
	}
	if u.User != nil {
		if _, hasPwd := u.User.Password(); hasPwd {
			u.User = url.UserPassword(u.User.Username(), dsnMask)
		}
	}
	q := u.Query()
	changed := false
	for k := range q {
		if isSecretKey(k) {
			q.Set(k, dsnMask)
			changed = true
		}
	}
	if changed {
		// Encode 会重排 query 参数顺序，对日志用途无妨
		u.RawQuery = q.Encode()
	}
	// url.String 会把掩码里的 * 百分号编码，还原成可读形式
	return strings.ReplaceAll(u.String(), url.QueryEscape(dsnMask), dsnMask)
}

// sanitizeMySQLDSN 脱敏 user:password@protocol(addr)/db 形式
//
// 密码可能含 @，因此以最后一个 @ 为分界（host 部分不含 @）
func sanitizeMySQLDSN(dsn string) string {
	atIdx := strings.LastIndex(dsn, "@")
	if atIdx < 0 {
		return dsn
	}
	credentials := dsn[:atIdx]
	colonIdx := strings.Index(credentials, ":") // 用户名不含冒号，第一个冒号即分界
	if colonIdx < 0 {
		return dsn // 没有密码
	}
	return credentials[:colonIdx+1] + dsnMask + dsn[atIdx:]
}

// sanitizeKVDSN 脱敏 libpq key=value 形式 DSN 中的所有敏感字段
//
// 逐段扫描而非只找第一个 password=：DSN 里出现两次同名 key 是合法的
// （后者生效），只遮第一个会把真正生效的那个留在日志里。
// 同时按 libpq 规则处理单引号包裹的值，否则 password='my secret'
// 会在空格处被截断，后半段照样漏出去
func sanitizeKVDSN(dsn string) string {
	var sb strings.Builder
	sb.Grow(len(dsn))

	i := 0
	for i < len(dsn) {
		// 保留段前的空白
		start := i
		for i < len(dsn) && (dsn[i] == ' ' || dsn[i] == '\t') {
			i++
		}
		sb.WriteString(dsn[start:i])
		if i >= len(dsn) {
			break
		}

		// 读 key
		keyStart := i
		for i < len(dsn) && dsn[i] != '=' && dsn[i] != ' ' && dsn[i] != '\t' {
			i++
		}
		key := dsn[keyStart:i]
		sb.WriteString(key)
		if i >= len(dsn) || dsn[i] != '=' {
			continue // 没有 = 的孤立 token，原样保留
		}
		sb.WriteByte('=')
		i++

		valStart := i
		i = skipKVValue(dsn, i)
		if isSecretKey(key) {
			sb.WriteString(dsnMask)
		} else {
			sb.WriteString(dsn[valStart:i])
		}
	}
	return sb.String()
}

// skipKVValue 返回 libpq key=value 中一个 value 结束后的下标
// 单引号包裹的值内部可含空格，反斜杠用于转义
func skipKVValue(s string, i int) int {
	if i < len(s) && s[i] == '\'' {
		i++ // 跳过起始引号
		for i < len(s) {
			if s[i] == '\\' && i+1 < len(s) {
				i += 2
				continue
			}
			if s[i] == '\'' {
				return i + 1 // 含结束引号
			}
			i++
		}
		return i
	}
	for i < len(s) && s[i] != ' ' && s[i] != '\t' {
		i++
	}
	return i
}

// isSecretKey 判断 DSN 参数名是否承载凭证
func isSecretKey(k string) bool {
	switch strings.ToLower(k) {
	case "password", "passwd", "pwd", "sslpassword":
		return true
	default:
		return false
	}
}

// sanitizeConfigForLog 创建配置的脱敏副本用于日志输出
func sanitizeConfigForLog(c *Config) *Config {
	sc := *c
	sc.DSN = sanitizeDSN(sc.DSN)
	return &sc
}

// sanitizeConfigsForLog 创建多个配置的脱敏副本用于日志输出
func sanitizeConfigsForLog(configs []*Config) []*Config {
	result := make([]*Config, len(configs))
	for i, c := range configs {
		result[i] = sanitizeConfigForLog(c)
	}
	return result
}
