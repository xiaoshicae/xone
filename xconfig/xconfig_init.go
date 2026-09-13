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
	// defaultValueSeparator 占位符中默认值的分隔符，如 ${VAR:-default}
	defaultValueSeparator = ":-"

	// profilesKey Server 下 Profiles 子块的 key（viper 内部统一小写）
	profilesKey = "profiles"

	// maskedValue 打印配置时敏感字段的替代值
	maskedValue = "******"
)

// 预编译正则表达式，避免重复编译
var envPlaceholderRegex = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

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

	if pa := detectProfilesActive(baseViperConfig); pa != "" { // 判断激活环境
		// 构造指定环境配置文件路径
		envConfigLocation, err := toProfilesActiveConfigLocation(configLocation, pa)
		if err != nil {
			return nil, xerror.Newf("xconfig", "parseConfig", "parse profiles active config file failed, err=[%v]", err)
		}

		if !xutil.FileExist(envConfigLocation) {
			xutil.WarnIfEnableDebug("XOne profiles active config file not found, ignore, env_config_location=[%s]", envConfigLocation)
		} else {
			// 加载指定环境配置文件
			envViperConfig, err := loadLocalConfig(envConfigLocation)
			if err != nil {
				return nil, xerror.Newf("xconfig", "parseConfig", "load config file failed, env_config_location=[%s], err=[%v]", envConfigLocation, err)
			}

			baseViperConfig = mergeProfilesViperConfig(baseViperConfig, envViperConfig)
		}
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
		if sub, ok := v.(map[string]any); ok {
			masked[k] = maskSensitive(sub)
			continue
		}
		masked[k] = v
	}
	return masked
}

// mergeProfilesViperConfig 将环境配置 vp2 深合并进基础配置 vp1
//
// 逐层合并而非整块替换：环境配置文件里只写要改的字段即可，
// 同一块下未提及的字段保留基础配置的值。列表整体替换——半个列表没有意义。
//
// Server.Profiles 不参与合并：激活的环境由基础配置文件/命令行/环境变量决定，
// 环境配置文件不应反过来改写它。
func mergeProfilesViperConfig(vp1, vp2 *viper.Viper) *viper.Viper {
	base := vp1.AllSettings()
	override := vp2.AllSettings()
	dropProfilesActive(override)

	vp := viper.New()
	for k, v := range deepMerge(base, override) {
		vp.Set(k, v)
	}
	return vp
}

// deepMerge 递归合并两个配置 map，override 覆盖 base，返回新 map 不改动入参
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
	server, ok := settings[strings.ToLower(ServerConfigKey)].(map[string]any)
	if !ok {
		return
	}
	delete(server, profilesKey)
	if len(server) == 0 {
		delete(settings, strings.ToLower(ServerConfigKey))
	}
}

// expandEnvPlaceholder 展开单个字符串中的环境变量占位符
//
// 返回展开结果，以及其中「未设置且没有默认值」的变量名列表。
// 用 os.LookupEnv 而非 os.Getenv：显式设为空串的环境变量是一个有效取值，
// 应当覆盖默认值，而不是被当作未设置。
func expandEnvPlaceholder(val string) (string, []string) {
	var missing []string
	expanded := envPlaceholderRegex.ReplaceAllStringFunc(val, func(match string) string {
		matches := envPlaceholderRegex.FindStringSubmatch(match)
		envKey := matches[1]
		if envVal, ok := os.LookupEnv(envKey); ok {
			return envVal
		}
		// matches[2] 是默认值；整个 ":-default" 段缺失时该分组不参与匹配
		if strings.Contains(match, defaultValueSeparator) {
			return matches[2]
		}
		missing = append(missing, envKey)
		return match
	})
	return expanded, missing
}

// expandEnvPlaceholders 展开配置中的 ${VAR} 或 ${VAR:-default} 占位符
//
// 支持的语法:
//   - ${VAR}           必填，环境变量未设置时初始化失败
//   - ${VAR:-default}  可选，环境变量未设置时使用 default（想要空值写 ${VAR:-}）
//
// 覆盖 string 叶子节点与字符串列表的元素；只展开一次，
// 环境变量的值里若恰好含有 ${...} 不会被再次解释。
func expandEnvPlaceholders(vp *viper.Viper) error {
	expansions := make(map[string]any)
	var missing []string

	for _, key := range vp.AllKeys() {
		expanded, ok, keyMissing := expandConfigValue(vp.Get(key))
		missing = append(missing, keyMissing...)
		if ok {
			expansions[key] = expanded
		}
	}

	if len(missing) > 0 {
		return xerror.Newf("xconfig", "expandEnvPlaceholders",
			"required env placeholder not set, use ${VAR:-default} to make it optional, missing=%v", distinct(missing))
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

// expandConfigValue 展开单个配置值，返回展开结果、是否发生变化、缺失的必填变量
//
// 只处理 string 与字符串列表：map 已被 viper 的 AllKeys 拆成独立叶子节点，
// 其余类型（数值、布尔）不含占位符。
func expandConfigValue(raw any) (any, bool, []string) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, false, nil
		}
		expanded, missing := expandEnvPlaceholder(v)
		return expanded, expanded != v, missing
	case []any:
		var missing []string
		changed := false
		items := make([]any, len(v))
		copy(items, v)
		for i, item := range v {
			str, ok := item.(string)
			if !ok || str == "" {
				continue
			}
			expanded, itemMissing := expandEnvPlaceholder(str)
			missing = append(missing, itemMissing...)
			if expanded != str {
				items[i] = expanded
				changed = true
			}
		}
		return items, changed, missing
	default:
		return nil, false, nil
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
