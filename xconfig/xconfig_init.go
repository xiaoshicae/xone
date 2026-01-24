package xconfig

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/xiaoshicae/xone/xhook"
	"github.com/xiaoshicae/xone/xutil"

	"github.com/spf13/viper"
)

var vip *viper.Viper

// 预编译正则表达式，避免重复编译
var envPlaceholderRegex = regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

func init() {
	xhook.BeforeStart(initXConfig, xhook.Order(1))
}

func initXConfig() error {
	configLocation := detectConfigLocation()
	if configLocation == "" {
		xutil.WarnIfEnableDebug("XOne initXConfig config file location not found, use default config")
		return nil
	}

	if err := loadDotEnvIfExist(configLocation); err != nil {
		return fmt.Errorf("XOne initXConfig invoke loadDotEnvIfExist failed, err=[%v]", err)
	}

	vp, err := parseConfig(configLocation)
	if err != nil {
		return fmt.Errorf("XOne initXConfig invoke parseConfig failed, err=[%v]", err)
	}

	printFinalConfig(vp) // 打印一下最终的配置信息

	vip = vp
	return nil
}

func loadDotEnvIfExist(configLocation string) error {
	dotEnvFileFullPath := path.Join(path.Dir(configLocation), dotEnvFileName)
	if xutil.FileExist(dotEnvFileFullPath) {
		return godotenv.Load(dotEnvFileFullPath)
	}
	return nil
}

func parseConfig(configLocation string) (*viper.Viper, error) {
	baseViperConfig, err := loadLocalConfig(configLocation) // 加载基础配置文件
	if err != nil {
		return nil, fmt.Errorf("load viper config failed, err=[%v]", err)
	}

	if pa := detectProfilesActive(baseViperConfig); pa != "" { // 判断激活环境
		// 构造指定环境配置文件路径
		envConfigLocation, err := toProfilesActiveConfigLocation(configLocation, pa)
		if err != nil {
			return nil, fmt.Errorf("parse profiles active config file failed, err=[%v]", err)
		}

		// 加载指定环境配置文件
		envViperConfig, err := loadLocalConfig(envConfigLocation)
		if err != nil {
			return nil, fmt.Errorf("load config file failed, env_config_location=[%s], err=[%v]", envConfigLocation, err)
		}

		baseViperConfig = mergeProfilesViperConfig(baseViperConfig, envViperConfig)
	}

	if baseViperConfig.GetString(serverNameConfigKey) == "" {
		xutil.WarnIfEnableDebug("config Server.AppID should not be empty, as it is used by many modules")
	}

	// 展开环境变量占位符
	expandEnvPlaceholders(baseViperConfig)

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

func printFinalConfig(vp *viper.Viper) {
	debugMsg := `
************************************** XOne load config **************************************
%s
**********************************************************************************************

`
	if xutil.EnableDebug() {
		fmt.Printf(debugMsg, xutil.ToJsonStringIndent(vp.AllSettings()))
	}
}

// mergeProfilesViperConfig 合并不同环境的两个viper, vp2覆盖vp1
func mergeProfilesViperConfig(vp1, vp2 *viper.Viper) *viper.Viper {
	vp := viper.New()

	vp1TopLevelConfigs := getTopLevelConfigs(vp1)
	vp2TopLevelAndServerSecondLevelConfigs := getTopLevelAndServerSecondLevelConfigs(vp2)

	// 先处理vp1
	for k, v := range vp1TopLevelConfigs {
		vp.Set(k, v)
	}
	// vp2 覆盖 vp1
	for k, v := range vp2TopLevelAndServerSecondLevelConfigs {
		vp.Set(k, v)
	}
	return vp
}

func getTopLevelConfigs(vp *viper.Viper) map[string]interface{} {
	allSettings := vp.AllSettings()
	topLevelConfigs := make(map[string]interface{})
	for k, v := range allSettings {
		if !isNestedKey(k) {
			topLevelConfigs[k] = v
		}
	}
	return topLevelConfigs
}

func isNestedKey(key string) bool {
	return strings.Contains(key, ".")
}

// getTopLevelAndServerSecondLevelConfigs 获取所有一级key+server下的二级可以对应的配置
func getTopLevelAndServerSecondLevelConfigs(vp *viper.Viper) map[string]interface{} {
	serverSecondLevelKeySet := make(map[string]struct{})
	configs := make(map[string]interface{})
	for _, k := range vp.AllKeys() {
		// 忽略profiles配置
		if strings.Contains(k, "server.profiles") {
			continue
		}

		// 收集所有server二级key
		if strings.Contains(k, "server.") {
			items := strings.Split(k, ".")
			key := items[0] + "." + items[1]
			if _, ok := serverSecondLevelKeySet[key]; !ok {
				serverSecondLevelKeySet[key] = struct{}{}
				configs[key] = vp.Get(key)
			}
		}
	}

	// 收集剩下的一级key
	allSettings := vp.AllSettings()
	for k, v := range allSettings {
		if !isNestedKey(k) && k != "server" {
			configs[k] = v
		}
	}
	return configs
}

// expandEnvPlaceholders 递归展开配置中的 ${VAR} 或 ${VAR:-default} 占位符
// 支持的语法:
//   - ${VAR} - 从环境变量读取 VAR
//   - ${VAR:-default} - 从环境变量读取 VAR，如果不存在则使用 default
func expandEnvPlaceholders(vp *viper.Viper) {
	// 收集所有需要展开的 key-value
	expansions := make(map[string]string)

	for _, key := range vp.AllKeys() {
		val := vp.GetString(key)
		if val == "" {
			continue
		}

		expanded := envPlaceholderRegex.ReplaceAllStringFunc(val, func(match string) string {
			matches := envPlaceholderRegex.FindStringSubmatch(match)
			// 边界检查：确保正则匹配成功且有足够的捕获组
			if len(matches) < 2 {
				return match // 匹配失败，返回原值
			}
			envKey := matches[1]
			defaultVal := ""
			if len(matches) >= 3 {
				defaultVal = matches[2]
			}

			if envVal := os.Getenv(envKey); envVal != "" {
				return envVal
			}
			return defaultVal
		})

		if expanded != val {
			expansions[key] = expanded
		}
	}

	// 通过修改 AllSettings 返回的 map 来保持嵌套结构
	if len(expansions) > 0 {
		allSettings := vp.AllSettings()
		for key, val := range expansions {
			setNestedValue(allSettings, key, val)
		}
		// 重新加载所有配置
		for k, v := range allSettings {
			vp.Set(k, v)
		}
	}
}

// setNestedValue 在嵌套 map 中设置值，保持结构完整
func setNestedValue(m map[string]interface{}, key string, value interface{}) {
	keys := strings.Split(key, ".")
	current := m

	for i := 0; i < len(keys)-1; i++ {
		k := keys[i]
		if next, ok := current[k].(map[string]interface{}); ok {
			current = next
		} else {
			// 如果中间路径不存在，创建新的 map
			newMap := make(map[string]interface{})
			current[k] = newMap
			current = newMap
		}
	}

	current[keys[len(keys)-1]] = value
}
