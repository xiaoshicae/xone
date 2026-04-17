package xgorm

import (
	"context"
	"errors"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xtrace"
	"github.com/xiaoshicae/xone/v2/xutil"

	stdMysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

const defaultClientName = "__default_client__"

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
			// 回滚已创建的连接
			for _, c := range created {
				if db, dbErr := c.DB(); dbErr == nil {
					_ = db.Close()
				}
			}
			return xerror.Newf("xgorm", "init", "newClient failed, name=[%v], err=[%v]", config.Name, err)
		}

		created = append(created, client)
		set(config.Name, client)

		// 第一个client为C()默认获取的client
		if idx == 0 {
			setDefault(client)
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
		return nil, xerror.Newf("xgorm", "newClient", "invoke gorm.Open failed, err=[%v]", err)
	}

	db, err := client.DB()
	if err != nil {
		return nil, xerror.Newf("xgorm", "newClient", "invoke client.DB failed, err=[%v]", err)
	}

	// 连接池参数配置
	db.SetMaxOpenConns(c.MaxOpenConns)
	db.SetMaxIdleConns(c.MaxIdleConns)
	db.SetConnMaxLifetime(xutil.ToDuration(c.MaxLifetime))
	db.SetConnMaxIdleTime(xutil.ToDuration(c.MaxIdleTime))

	pingTimeout := xutil.ToDuration(c.DialTimeout)
	err = xutil.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		return db.PingContext(ctx)
	}, 3, time.Second)
	if err != nil {
		return nil, xerror.Newf("xgorm", "newClient", "invoke db.PingContext failed, err=[%v]", err)
	}

	if xtrace.EnableTrace() {
		if err := client.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
			return nil, xerror.Newf("xgorm", "newClient", "use tracing.NewPlugin failed, err=[%v]", err)
		}
	}

	return client, nil
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
		return "", err
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
	for _, k := range sortedKeys(injects) {
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
	for _, k := range sortedKeys(injects) {
		if _, dup := existing[k]; dup {
			continue
		}
		v := injects[k]
		if sb.Len() > 0 && !strings.HasSuffix(sb.String(), " ") {
			sb.WriteByte(' ')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(quotePostgresKVValue(v))
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

// sortedKeys 返回 map 键的排序切片，保证注入顺序稳定（方便测试）
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
// 支持 URL 格式 (user:password@host) 和 Postgres key=value 格式 (password=xxx)
func sanitizeDSN(dsn string) string {
	// URL 格式: user:password@host
	atIdx := strings.Index(dsn, "@")
	if atIdx >= 0 {
		prefix := dsn[:atIdx]
		colonIdx := strings.LastIndex(prefix, ":")
		if colonIdx >= 0 {
			return prefix[:colonIdx+1] + "***" + dsn[atIdx:]
		}
		return dsn
	}

	// Postgres key=value 格式: password=xxx
	return sanitizeDSNPasswordKV(dsn)
}

// sanitizeDSNPasswordKV 对 key=value 格式 DSN 中的 password 字段脱敏
func sanitizeDSNPasswordKV(dsn string) string {
	const passwordKey = "password="
	idx := strings.Index(strings.ToLower(dsn), passwordKey)
	if idx < 0 {
		return dsn
	}
	start := idx + len(passwordKey)
	end := strings.IndexByte(dsn[start:], ' ')
	if end < 0 {
		// password 在末尾
		return dsn[:start] + "***"
	}
	return dsn[:start] + "***" + dsn[start+end:]
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
