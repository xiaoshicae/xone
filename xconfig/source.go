package xconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xutil"
)

const (
	configLocationArgKey = "server.config.location"
	configLocationEnvKey = "SERVER_CONFIG_LOCATION"

	profilesActiveArgKey = "server.profiles.active"
	profilesActiveEnvKey = "SERVER_PROFILES_ACTIVE"
)

// configLocationPaths 主配置文件的约定搜索路径，按优先级排序
var configLocationPaths = []string{
	"./application.yml",
	"./application.yaml",
	"./conf/application.yml",
	"./conf/application.yaml",
	"./config/application.yml",
	"./config/application.yaml",
	"./../conf/application.yml",
	"./../conf/application.yaml",
	"./../config/application.yml",
	"./../config/application.yaml",
}

// profilesActivePattern 激活环境名的合法字符集
//
// 环境名会被拼进配置文件路径，必须限制字符集：否则 SERVER_PROFILES_ACTIVE=../../etc/x
// 会让进程去加载任意路径下的配置文件。
var profilesActivePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// layer 一份已解析的配置，以及它来自哪个文件
//
// 加载的产物是一串按优先级从低到高排好的 layer。把「加载哪些文件、什么顺序」
// 表达成一个列表，而不是散落在控制流里的若干次合并调用，优先级规则就只存在于
// 一个地方，也能直接回答「这个值是从哪个文件来的」。
type layer struct {
	from     string
	settings map[string]any
}

// detectConfigLocation 定位主配置文件：启动参数 > 环境变量 > 约定路径
func detectConfigLocation() string {
	fromArg, _ := xutil.GetConfigFromArgs(configLocationArgKey)

	candidates := []struct{ from, value string }{
		{"arg", fromArg},
		{"env", os.Getenv(configLocationEnvKey)},
	}
	for _, c := range candidates {
		if c.value != "" {
			xutil.InfoIfEnableDebug("XOne detect config location [%s] from %s", c.value, c.from)
			return c.value
		}
	}

	for _, loc := range configLocationPaths {
		if xutil.FileExist(loc) {
			xutil.InfoIfEnableDebug("XOne detect config location [%s] from current dir", loc)
			return loc
		}
	}
	return ""
}

// detectProfilesActive 判断激活的环境：启动参数 > 环境变量 > 主配置文件
//
// 环境配置文件与被导入的文件都不参与决定 —— 让被加载进来的文件反过来决定
// 「还要加载什么」会形成环，也让配置的来源无从追查。这一点与 Spring 相同。
func detectProfilesActive(base map[string]any) (string, error) {
	fromArg, _ := xutil.GetConfigFromArgs(profilesActiveArgKey)
	fromFile, _ := lookupKeyPath(base, profilesActivePath...).(string)

	candidates := []struct{ from, value string }{
		{"arg", fromArg},
		{"env", os.Getenv(profilesActiveEnvKey)},
		{"base config file", fromFile},
	}
	for _, c := range candidates {
		if c.value == "" {
			continue
		}

		// 此时整份配置尚未展开占位符，单独展开这一个值
		r := &resolver{}
		active := r.expand(c.value)
		if err := r.err("detectProfilesActive"); err != nil {
			return "", err
		}
		if !profilesActivePattern.MatchString(active) {
			return "", xerror.Newf("xconfig", "detectProfilesActive",
				"profiles active is invalid, only letters, digits, '_' and '-' are allowed, profiles_active=[%s]", active)
		}

		xutil.InfoIfEnableDebug("XOne detect profiles active [%s] from %s", active, c.from)
		return active, nil
	}

	xutil.InfoIfEnableDebug("XOne config profiles active not found")
	return "", nil
}

// decodeFile 把一个配置文件解析成 map，key 统一转成小写
//
// 自己解析而不是借道 viper：viper 的 AllSettings() 是按「点分 key 重新拼树」实现的，
// 含 "." 的 key（如 OpenTelemetry 的 service.name）经它往返一次就会被拆成多层，
// map[string]string 类型的字段会直接反序列化失败。
func decodeFile(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, xerror.Newf("xconfig", "decodeFile",
			"read config file failed, location=[%s], err=[%v]", path, err)
	}

	settings := make(map[string]any)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml":
		err = yaml.Unmarshal(raw, &settings)
	case ".json":
		err = json.Unmarshal(raw, &settings)
	default:
		return nil, xerror.Newf("xconfig", "decodeFile",
			"unsupported config file format, supported are .yml/.yaml/.json, location=[%s]", path)
	}
	if err != nil {
		return nil, xerror.Newf("xconfig", "decodeFile",
			"parse config file failed, location=[%s], err=[%v]", path, err)
	}
	return lowerKeys(settings), nil
}

// profileVariantPath 构造环境变体路径，例如 ./conf/db.yml + dev -> ./conf/db-dev.yml
func profileVariantPath(location, active string) string {
	ext := filepath.Ext(location)
	return strings.TrimSuffix(location, ext) + "-" + active + ext
}

// buildLayers 排出全部配置层，返回值按优先级从低到高，同时返回实际生效的导入列表
//
// 层序由两条规则决定，都与 Spring Boot 对齐：
//
//  1. 被导入的文件覆盖导入它的文件（官方文档：values from the imported file take
//     precedence over the file that triggered the import）。这样 db.yml 就是数据库
//     配置的权威，不会被 application.yml 里一个忘删的残留字段悄悄压过。
//  2. 带环境后缀的文件整体压过不带的（官方文档：profile-specific files always
//     overriding the non-specific ones）。所以先排完所有无后缀的，再排所有带后缀的 ——
//     否则 db-dev.yml 会被后面声明的 redis.yml 压过，一个环境专属的值被非环境专属的
//     值覆盖，说不通。
//
// 最终顺序（后者覆盖前者）：
//
//	application.yml < db.yml < redis.yml < application-{env}.yml < db-{env}.yml < redis-{env}.yml
func buildLayers(location string, base map[string]any, active string) ([]layer, []string, error) {
	profileBase, profileLocation, err := loadProfileBase(location, active)
	if err != nil {
		return nil, nil, err
	}

	imports, err := collectImports(base, profileBase)
	if err != nil {
		return nil, nil, err
	}

	plain := []layer{{from: location, settings: base}}
	profile := make([]layer, 0, len(imports)+1)
	if profileBase != nil {
		profile = append(profile, layer{from: profileLocation, settings: dropLoadDirectives(profileBase)})
	}

	// 相对路径以主配置文件所在目录为基准，而不是进程工作目录：
	// db.yml 就放在 application.yml 旁边是最自然的布局，
	// 而工作目录在容器里取决于镜像的 WORKDIR，不该影响配置能不能找到
	baseDir := filepath.Dir(location)
	for _, imp := range imports {
		full := imp
		if !filepath.IsAbs(full) {
			full = filepath.Join(baseDir, full)
		}

		// 点名要导入的文件不存在时直接失败：这与 application-{env}.yml 不存在只告警
		// 不同 —— 后者是约定俗成的可选项，而这个是使用者明确要求加载的
		if !xutil.FileExist(full) {
			return nil, nil, xerror.Newf("xconfig", "buildLayers",
				"imported config file not found, %s=[%s], resolved=[%s]", importDisplayKey, imp, full)
		}
		settings, err := decodeFile(full)
		if err != nil {
			return nil, nil, err // decodeFile 已返回 xerror
		}
		if lookupKeyPath(settings, importPath...) != nil {
			xutil.WarnIfEnableDebug("XOne %s in imported config file is ignored, nested import is not supported, location=[%s]",
				importDisplayKey, full)
		}
		plain = append(plain, layer{from: full, settings: dropLoadDirectives(settings)})

		if active == "" {
			continue
		}
		variant := profileVariantPath(full, active)
		if !xutil.FileExist(variant) {
			continue
		}
		variantSettings, err := decodeFile(variant)
		if err != nil {
			return nil, nil, err // decodeFile 已返回 xerror
		}
		profile = append(profile, layer{from: variant, settings: dropLoadDirectives(variantSettings)})
	}

	return append(plain, profile...), imports, nil
}

// loadProfileBase 加载主配置的环境变体 application-{active}.yml，不存在时只告警
func loadProfileBase(location, active string) (map[string]any, string, error) {
	if active == "" {
		return nil, "", nil
	}

	variant := profileVariantPath(location, active)
	if !xutil.FileExist(variant) {
		xutil.WarnIfEnableDebug("XOne profiles active config file not found, ignore, env_config_location=[%s]", variant)
		return nil, "", nil
	}

	settings, err := decodeFile(variant)
	if err != nil {
		return nil, "", err // decodeFile 已返回 xerror
	}
	return settings, variant, nil
}

// collectImports 汇总主配置与环境变体里声明的导入列表
//
// 取并集而非按「列表整体替换」处理：Import 是加载指令而不是配置数据，
// 若按替换处理，application-dev.yml 想多加一个文件就得把整张列表抄一遍。
// 保持声明顺序并去重，主配置在前、环境变体在后。
//
// 此时整份配置尚未展开占位符，单独展开这几个值 —— 路径里写 ${CONFIG_DIR:.}/db.yml
// 是合理需求。
func collectImports(base, profile map[string]any) ([]string, error) {
	raw := importsOf(base)
	raw = append(raw, importsOf(profile)...)

	r := &resolver{}
	expanded := make([]string, 0, len(raw))
	for _, p := range raw {
		if v := r.expand(p); v != "" {
			expanded = append(expanded, v)
		}
	}
	if err := r.err("collectImports"); err != nil {
		return nil, err
	}

	// 扩展名在这里一次性校验：环境变体路径靠它拼接，解析器也靠它选格式。
	// 放到加载时才发现，报出来的是「文件找不到」，看不出问题其实出在导入列表里
	for _, p := range expanded {
		if filepath.Ext(p) == "" {
			return nil, xerror.Newf("xconfig", "collectImports",
				"imported config file has no extension, %s=[%s]", importDisplayKey, p)
		}
	}
	return distinct(expanded), nil
}

// importsOf 读出一层里声明的导入列表
func importsOf(settings map[string]any) []string {
	switch v := lookupKeyPath(settings, importPath...).(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		// 只写了一个文件时不强求写成列表
		if v != "" {
			return []string{v}
		}
	}
	return nil
}

// dropLoadDirectives 剔除一层无权决定的加载指令
//
// 激活环境与导入列表只认主配置/启动参数/环境变量，其余文件里写了也不算数。
// 两者都会在合并之后由权威值回填，所以这里直接删掉即可。
func dropLoadDirectives(settings map[string]any) map[string]any {
	deleteKeyPath(settings, profilesPath...)
	deleteKeyPath(settings, importPath...)
	return settings
}

// mergeLayers 按优先级从低到高逐层深合并
func mergeLayers(layers []layer) map[string]any {
	merged := make(map[string]any)
	for _, l := range layers {
		merged = deepMerge(merged, l.settings)
	}
	return merged
}
