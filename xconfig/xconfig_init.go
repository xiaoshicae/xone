package xconfig

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"github.com/xiaoshicae/xone/v2/internal/hookorder"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/spf13/viper"
)

var (
	vip   *viper.Viper
	vipMu sync.RWMutex
)

const (
	// profilesKey Server 下 Profiles 子块的 key（viper 内部统一小写）
	profilesKey = "profiles"

	// configSubKey / importSubKey Server.Config.Import 的两级子 key（viper 内部统一小写）
	configSubKey = "config"
	importSubKey = "import"

	// maskedValue 打印配置时敏感字段的替代值
	maskedValue = "******"
)

// envPlaceholderRegex 占位符语法：${VAR} / ${VAR:default}
//
// 分隔符是 `:`，与 Spring 一致。
var envPlaceholderRegex = regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)

// anyPlaceholderRegex 任意 ${...} 形态，用于发现不被支持的写法
//
// 只靠 envPlaceholderRegex 是「匹配不上就当普通文本」，写错的占位符会原样留在
// 配置值里：${} 这种不报错、不告警，值就真的变成了那串字面量，
// 错误要到用它的地方才以另一副面孔出现。
var anyPlaceholderRegex = regexp.MustCompile(`\$\{[^{}]*\}`)

// sensitiveKeyPattern 需要在打印配置时脱敏的字段名（大小写不敏感）
var sensitiveKeyPattern = regexp.MustCompile(`(?i)(password|passwd|secret|token|apikey|api_key|accesskey|access_key|privatekey|private_key|credential|dsn)`)

func init() {
	xhook.BeforeStart(initXConfig, xhook.ReservedOrder(hookorder.Token{}, hookorder.Config))
}

func initXConfig() error {
	configLocation := detectConfigLocation()
	if configLocation == "" {
		xutil.WarnIfEnableDebug("XOne initXConfig config file location not found, use default config")
		return nil
	}

	if err := loadDotEnvIfExist(configLocation); err != nil {
		return xerror.Newf("xconfig", "init", "invoke loadDotEnvIfExist failed, err=[%v]", err)
	}

	vp, err := parseConfig(configLocation)
	if err != nil {
		return err // parseConfig 已返回 xerror
	}

	printFinalConfig(vp) // 打印一下最终的配置信息

	vipMu.Lock()
	vip = vp
	vipMu.Unlock()
	return nil
}

func loadDotEnvIfExist(configLocation string) error {
	dotEnvFileFullPath := filepath.Join(filepath.Dir(configLocation), dotEnvFileName)
	if xutil.FileExist(dotEnvFileFullPath) {
		return godotenv.Load(dotEnvFileFullPath)
	}
	return nil
}

func parseConfig(configLocation string) (*viper.Viper, error) {
	baseViperConfig, err := loadLocalConfig(configLocation) // 加载基础配置文件
	if err != nil {
		return nil, xerror.Newf("xconfig", "parseConfig", "load viper config failed, err=[%v]", err)
	}

	profilesActive := detectProfilesActive(baseViperConfig) // 判断激活环境

	var envViperConfig *viper.Viper
	if profilesActive != "" {
		// 构造指定环境配置文件路径
		envConfigLocation, err := toProfilesActiveConfigLocation(configLocation, profilesActive)
		if err != nil {
			return nil, xerror.Newf("xconfig", "parseConfig", "parse profiles active config file failed, err=[%v]", err)
		}

		if !xutil.FileExist(envConfigLocation) {
			xutil.WarnIfEnableDebug("XOne profiles active config file not found, ignore, env_config_location=[%s]", envConfigLocation)
		} else {
			// 加载指定环境配置文件
			envViperConfig, err = loadLocalConfig(envConfigLocation)
			if err != nil {
				return nil, xerror.Newf("xconfig", "parseConfig", "load config file failed, env_config_location=[%s], err=[%v]", envConfigLocation, err)
			}
		}
	}

	// 合并全部来源
	baseViperConfig, err = mergeAllConfig(baseViperConfig, envViperConfig, configLocation, profilesActive)
	if err != nil {
		return nil, err // mergeAllConfig 已返回 xerror
	}

	// 合并完成后统一展开一次占位符，避免同一个值被展开两遍
	if err := expandEnvPlaceholders(baseViperConfig); err != nil {
		return nil, err // expandEnvPlaceholders 已返回 xerror
	}

	// 展开之后再校验：Server.Name 可能本身就是一个占位符
	if baseViperConfig.GetString(serverNameConfigKey) == "" {
		xutil.WarnIfEnableDebug("config Server.Name should not be empty, as it is used by many modules")
	}

	return baseViperConfig, nil
}

func loadLocalConfig(configLocation string) (*viper.Viper, error) {
	vp := viper.New()
	vp.SetConfigFile(configLocation)
	if err := vp.ReadInConfig(); err != nil {
		return nil, err
	}
	return vp, nil
}

// printFinalConfig 打印最终生效的配置，敏感字段脱敏后再输出
func printFinalConfig(vp *viper.Viper) {
	if !xutil.EnableXOneDebug() {
		return
	}

	banner := `
************************************** XOne load config **************************************
%s
**********************************************************************************************
`
	// 配置值经用户输入，不能进格式串
	xutil.InfoIfEnableDebug("%s", fmt.Sprintf(banner, xutil.ToJsonStringIndent(maskSensitive(vp.AllSettings()))))
}

// maskSensitive 递归拷贝配置并把敏感字段的值替换掉，绝不输出凭证明文
//
// 与 xgorm 的 DSN 脱敏、xredis 的密码脱敏同一原则：
// 配置是凭证的源头，打印前必须先脱敏。
func maskSensitive(settings map[string]any) map[string]any {
	masked := make(map[string]any, len(settings))
	for k, v := range settings {
		if sensitiveKeyPattern.MatchString(k) {
			masked[k] = maskedValue
			continue
		}
		masked[k] = maskSensitiveValue(v)
	}
	return masked
}

// maskSensitiveValue 对任意配置值递归脱敏
//
// 必须穿过列表：xgorm / xredis 的多实例形态就是一个 map 列表
//
//	XGorm:
//	  - Name: master
//	    DSN: "user:pass@tcp(...)/db"
//
// 只递归 map 的话，这里的 DSN 连同密码会原样打进启动日志。
func maskSensitiveValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return maskSensitive(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = maskSensitiveValue(item)
		}
		return out
	default:
		return v
	}
}

// lowerKey viper 内部统一把 key 转小写，比对时同样处理
func lowerKey(k string) string {
	return strings.ToLower(k)
}

// mergeAllConfig 按优先级合并主配置、导入的配置与环境配置
//
// 两条规则决定了顺序，都与 Spring Boot 对齐：
//
//  1. 被导入的文件覆盖导入它的文件（官方文档：values from the imported file take
//     precedence over the file that triggered the import）。这样 db.yml 就是数据库
//     配置的权威，不会被 application.yml 里一个忘删的残留字段悄悄压过。
//  2. 带环境后缀的文件整体压过不带的（官方文档：profile-specific files always
//     overriding the non-specific ones）。因此先合完所有无后缀的，再合所有带后缀的——
//     否则 db-dev.yml 会被后面声明的 redis.yml 压过，一个环境专属的值被非环境专属的
//     值覆盖，说不通。
//
// 最终顺序（后者覆盖前者）：
//
//	application.yml < db.yml < redis.yml < application-{env}.yml < db-{env}.yml < redis-{env}.yml
func mergeAllConfig(baseVp, envVp *viper.Viper, configLocation, profilesActive string) (*viper.Viper, error) {
	paths, err := collectImportPaths(baseVp, envVp)
	if err != nil {
		return nil, err // collectImportPaths 已返回 xerror
	}

	// 相对路径以主配置文件所在目录为基准，而不是进程工作目录：
	// db.yml 就放在 application.yml 旁边是最自然的布局，
	// 而工作目录在容器里取决于镜像的 WORKDIR，不该影响配置能不能找到
	importedPlain, importedProfile, err := loadImportedSettings(paths, filepath.Dir(configLocation), profilesActive)
	if err != nil {
		return nil, err // loadImportedSettings 已返回 xerror
	}

	settings := baseVp.AllSettings()
	settings = deepMerge(settings, importedPlain)
	if envVp != nil {
		settings = deepMerge(settings, profileSettingsOf(envVp))
	}
	settings = deepMerge(settings, importedProfile)

	vp := viper.New()
	for k, v := range settings {
		vp.Set(k, v)
	}
	return vp, nil
}

// profileSettingsOf 取出环境配置的设置并剔除 Server.Profiles
//
// 激活的环境由基础配置文件/命令行/环境变量决定，环境配置文件不应反过来改写它。
// 这也与 Spring 一致：spring.profiles.active 不允许出现在 profile-specific 文件里。
func profileSettingsOf(vp *viper.Viper) map[string]any {
	settings := vp.AllSettings()
	dropProfilesActive(settings)
	return settings
}

// deepMerge 递归合并两个配置 map，override 覆盖 base，返回新 map 不改动入参
//
// 不做环检测：入参只来自 vp.AllSettings()，而 YAML 表达不出自引用
// （递归锚点被解析器拒绝：anchor 'x' value contains itself），
// 嵌套深度也被解析器限制在 10000 层以内。
// 若改由别处传入任意 map，自引用会撑爆栈并触发 fatal error（recover 无法拦截）。
func deepMerge(base, override map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(override))
	maps.Copy(merged, base)

	for k, v := range override {
		subOverride, isMap := v.(map[string]any)
		if !isMap {
			merged[k] = v
			continue
		}
		subBase, baseIsMap := merged[k].(map[string]any)
		if !baseIsMap {
			merged[k] = v
			continue
		}
		merged[k] = deepMerge(subBase, subOverride)
	}
	return merged
}

// dropProfilesActive 从配置中移除 Server.Profiles，避免环境配置文件改写激活环境
func dropProfilesActive(settings map[string]any) {
	server, ok := settings[lowerKey(ServerConfigKey)].(map[string]any)
	if !ok {
		return
	}
	delete(server, profilesKey)
	if len(server) == 0 {
		delete(settings, lowerKey(ServerConfigKey))
	}
}

// placeholderIssues 展开占位符时发现的问题
//
// 分成两类而不是合成一个列表：「变量没设置」是部署环境的问题，
// 「写法不支持」是配置文件写错了，给出的修复动作完全不同。
type placeholderIssues struct {
	missing     []string // 必填但环境变量未设置
	unsupported []string // 写法不被支持，如 ${}
}

func (i *placeholderIssues) merge(o placeholderIssues) {
	i.missing = append(i.missing, o.missing...)
	i.unsupported = append(i.unsupported, o.unsupported...)
}

func (i placeholderIssues) empty() bool {
	return len(i.missing) == 0 && len(i.unsupported) == 0
}

// err 把问题转成错误，两类各自给出可操作的提示
func (i placeholderIssues) err(op string) error {
	if len(i.unsupported) > 0 {
		return xerror.Newf("xconfig", op,
			"unsupported placeholder syntax, supported forms are ${VAR} and ${VAR:default}, got=%v",
			distinct(i.unsupported))
	}
	if len(i.missing) > 0 {
		return xerror.Newf("xconfig", op,
			"required env placeholder not set, use ${VAR:default} to make it optional, missing=%v", distinct(i.missing))
	}
	return nil
}

// expandEnvPlaceholder 展开单个字符串中的环境变量占位符
//
// 用 os.LookupEnv 而非 os.Getenv：显式设为空串的环境变量是一个有效取值，
// 应当覆盖默认值，而不是被当作未设置。这一点与 Spring 相同。
func expandEnvPlaceholder(val string) (string, placeholderIssues) {
	var issues placeholderIssues

	// 写法不被支持的：它们匹配不上 envPlaceholderRegex，
	// 不检查的话会被当成普通文本原样留下
	for _, m := range anyPlaceholderRegex.FindAllString(val, -1) {
		if envPlaceholderRegex.FindString(m) != m {
			issues.unsupported = append(issues.unsupported, m)
		}
	}

	expanded := envPlaceholderRegex.ReplaceAllStringFunc(val, func(match string) string {
		idx := envPlaceholderRegex.FindStringSubmatchIndex(match)
		envKey := match[idx[2]:idx[3]]
		if envVal, ok := os.LookupEnv(envKey); ok {
			return envVal
		}
		// 分组 2 参与匹配即表示写了 ":"，无论默认值是否为空。
		// 用下标判断而不是查字符串里有没有 ":"，因为变量名里不可能有 ":"，
		// 但默认值里可能有（如 ${ADDR:127.0.0.1:6379}）
		if idx[4] >= 0 {
			return match[idx[4]:idx[5]]
		}
		issues.missing = append(issues.missing, envKey)
		return match
	})
	return expanded, issues
}

// expandEnvPlaceholders 展开配置中的 ${VAR} 或 ${VAR:default} 占位符
//
// 支持的语法（分隔符是 ":"，与 Spring 一致）:
//   - ${VAR}          必填，环境变量未设置时初始化失败
//   - ${VAR:default}  可选，环境变量未设置时使用 default（想要空值写 ${VAR:}）
//
// 覆盖所有 string 叶子节点，包括列表元素以及列表里嵌套的 map；只展开一次，
// 环境变量的值里若恰好含有 ${...} 不会被再次解释。
func expandEnvPlaceholders(vp *viper.Viper) error {
	expansions := make(map[string]any)
	var issues placeholderIssues

	for _, key := range vp.AllKeys() {
		expanded, ok, keyIssues := expandConfigValue(vp.Get(key))
		issues.merge(keyIssues)
		if ok {
			expansions[key] = expanded
		}
	}

	if !issues.empty() {
		return issues.err("expandEnvPlaceholders")
	}

	if len(expansions) == 0 {
		return nil
	}

	// 通过修改 AllSettings 返回的 map 来保持嵌套结构
	allSettings := vp.AllSettings()
	for key, val := range expansions {
		setNestedValue(allSettings, key, val)
	}
	for k, v := range allSettings {
		vp.Set(k, v)
	}
	return nil
}

// expandConfigValue 递归展开单个配置值，返回展开结果、是否发生变化、发现的问题
//
// 顶层 map 已被 viper 的 AllKeys 拆成独立叶子节点，但列表不会被拆——
// 列表里的 map 是 viper 眼中的一个整体叶子值。xgorm / xredis 的多实例形态
// 恰好就是这个形状：
//
//	XRedis:
//	  - Name: r1
//	    Password: "${REDIS_PW}"
//
// 因此这里必须自己往下走：只认字符串元素的话，多实例配置里的占位符
// 既不会被展开（密码变成字面量 "${REDIS_PW}"），缺失时也不会报错。
//
// 数值、布尔等其余类型不含占位符，原样返回。
func expandConfigValue(raw any) (any, bool, placeholderIssues) {
	var issues placeholderIssues

	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, false, issues
		}
		expanded, valIssues := expandEnvPlaceholder(v)
		return expanded, expanded != v, valIssues
	case []any:
		changed := false
		items := make([]any, len(v))
		copy(items, v)
		for i, item := range v {
			expanded, itemChanged, itemIssues := expandConfigValue(item)
			issues.merge(itemIssues)
			if itemChanged {
				items[i] = expanded
				changed = true
			}
		}
		return items, changed, issues
	case map[string]any:
		// 拷贝而非就地改：入参来自 viper 内部，展开不该顺手改写它的状态
		changed := false
		out := make(map[string]any, len(v))
		maps.Copy(out, v)
		for k, item := range v {
			expanded, itemChanged, itemIssues := expandConfigValue(item)
			issues.merge(itemIssues)
			if itemChanged {
				out[k] = expanded
				changed = true
			}
		}
		return out, changed, issues
	default:
		return nil, false, issues
	}
}

// distinct 去重并保持出现顺序
func distinct(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		if _, ok := seen[it]; ok {
			continue
		}
		seen[it] = struct{}{}
		out = append(out, it)
	}
	return out
}

// setNestedValue 在嵌套 map 中设置值，保持结构完整
func setNestedValue(m map[string]any, key string, value any) {
	keys := strings.Split(key, ".")
	current := m

	for i := 0; i < len(keys)-1; i++ {
		k := keys[i]
		if next, ok := current[k].(map[string]any); ok {
			current = next
		} else {
			// 如果中间路径不存在，创建新的 map
			newMap := make(map[string]any)
			current[k] = newMap
			current = newMap
		}
	}

	current[keys[len(keys)-1]] = value
}
