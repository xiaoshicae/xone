package xconfig

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"

	"github.com/xiaoshicae/xone/v3/internal/hookorder"
	"github.com/xiaoshicae/xone/v3/xerror"
	"github.com/xiaoshicae/xone/v3/xhook"
	"github.com/xiaoshicae/xone/v3/xutil"
)

const (
	dotEnvFileName = ".env"

	// maskedValue 打印配置时敏感字段的替代值
	maskedValue = "******"
)

// sensitiveKeyPattern 需要在打印配置时脱敏的字段名（大小写不敏感）
var sensitiveKeyPattern = regexp.MustCompile(`(?i)(password|passwd|secret|token|apikey|api_key|accesskey|access_key|privatekey|private_key|credential|dsn)`)

func init() {
	xhook.BeforeStart(initXConfig, xhook.ReservedOrder(hookorder.Token{}, hookorder.Config))
}

// initXConfig 完整的加载过程，每一步的产物都是下一步的唯一输入
func initXConfig() error {
	// 1. 定位主配置文件
	location := detectConfigLocation()
	if location == "" {
		xutil.WarnIfEnableDebug("XOne initXConfig config file location not found, use default config")
		return nil
	}

	// 2. 同目录下的 .env 先加载，让后续的占位符能引用其中的变量
	if err := loadDotEnv(location); err != nil {
		return err
	}

	// 3. 解析主配置，并据此判断激活的环境
	base, err := decodeFile(location)
	if err != nil {
		return err // decodeFile 已返回 xerror
	}
	active, err := detectProfilesActive(base)
	if err != nil {
		return err // detectProfilesActive 已返回 xerror
	}

	// 4. 排出全部配置层，逐层深合并
	layers, imports, err := buildLayers(location, base, active)
	if err != nil {
		return err // buildLayers 已返回 xerror
	}
	settings := mergeLayers(layers)

	// 5. 回填权威的加载指令，让最终配置与实际加载行为对得上
	annotate(settings, active, imports)

	// 6. 统一展开一次 ${VAR} 占位符
	settings, err = expandPlaceholders(settings)
	if err != nil {
		return err // expandPlaceholders 已返回 xerror
	}

	// 7. 交给 viper 作为只读存储
	return publish(settings, layers)
}

// loadDotEnv 加载主配置文件同目录下的 .env，不存在时跳过
func loadDotEnv(location string) error {
	path := filepath.Join(filepath.Dir(location), dotEnvFileName)
	if !xutil.FileExist(path) {
		return nil
	}
	if err := godotenv.Load(path); err != nil {
		return xerror.Newf("xconfig", "loadDotEnv", "load .env failed, location=[%s], err=[%v]", path, err)
	}
	return nil
}

// annotate 把权威的加载指令回填进合并结果
//
// 激活环境可能来自启动参数或环境变量，导入列表是主配置与环境变体的并集 ——
// 两者都不等于任何单个文件里写的内容。不回填的话，读到的 Server.Profiles.Active
// 会与实际加载的文件对不上，排查时极具误导性。
func annotate(settings map[string]any, active string, imports []string) {
	if active != "" {
		setKeyPath(settings, active, profilesActivePath...)
	}
	if len(imports) > 0 {
		values := make([]any, 0, len(imports))
		for _, imp := range imports {
			values = append(values, imp)
		}
		setKeyPath(settings, values, importPath...)
	}
}

// publish 把最终配置交给 viper，并解析出 Server 块供框架自身使用
func publish(settings map[string]any, layers []layer) error {
	vp := viper.New()
	if err := vp.MergeConfigMap(settings); err != nil {
		return xerror.Newf("xconfig", "publish", "build config store failed, err=[%v]", err)
	}

	s := Server{}
	if err := vp.UnmarshalKey(ServerConfigKey, &s); err != nil {
		return xerror.Newf("xconfig", "publish", "unmarshal %s failed, err=[%v]", ServerConfigKey, err)
	}
	// 展开之后再校验：Server.Name 可能本身就写成一个占位符
	if s.Name == "" {
		xutil.WarnIfEnableDebug("config %s.Name should not be empty, as it is used by many modules", ServerConfigKey)
	}

	from := make([]string, 0, len(layers))
	for _, l := range layers {
		from = append(from, l.from)
	}
	printFinalConfig(settings, from)

	storeMu.Lock()
	store, server, sources = vp, serverMergeDefault(s), from
	storeMu.Unlock()
	return nil
}

// printFinalConfig 打印最终生效的配置与它的全部来源，敏感字段脱敏后再输出
func printFinalConfig(settings map[string]any, sources []string) {
	if !xutil.EnableXOneDebug() {
		return
	}

	banner := `
************************************** XOne load config **************************************
sources (low -> high precedence):
%s
----------------------------------------------------------------------------------------------
%s
**********************************************************************************************
`
	list := make([]string, 0, len(sources))
	for i, s := range sources {
		list = append(list, fmt.Sprintf("  %d. %s", i+1, s))
	}

	// 配置值经用户输入，不能进格式串
	xutil.InfoIfEnableDebug("%s", fmt.Sprintf(banner,
		strings.Join(list, "\n"), xutil.ToJsonStringIndent(maskSensitive(settings))))
}

// maskSensitive 递归拷贝配置并把敏感字段的值替换掉，绝不输出凭证明文
//
// 与 xgorm 的 DSN 脱敏、xredis 的密码脱敏同一原则：配置是凭证的源头，打印前必须先脱敏。
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
