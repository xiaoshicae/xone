package xconfig

import (
	"path/filepath"

	"github.com/spf13/viper"

	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xutil"
)

// importConfigKey 额外配置文件列表的配置 key
//
// 放在 Server.Config 下，与启动参数 server.config.location 同一脉络：
// 都是「配置文件从哪来」这件事。
const importConfigKey = "Server.Config.Import"

// collectImportPaths 汇总主配置与环境配置里声明的导入列表
//
// 取并集而非按「列表整体替换」处理：Import 是加载指令而不是配置数据，
// 若按替换处理，application-dev.yml 想多加一个文件就得把整张列表抄一遍。
// 保持声明顺序并去重，base 在前、env 在后。
//
// 此时整份配置尚未展开占位符，单独展开这几个值即可——路径里写
// ${CONFIG_DIR:.}/db.yml 是合理需求。
func collectImportPaths(baseVp, envVp *viper.Viper) ([]string, error) {
	raw := make([]string, 0, 8)
	raw = append(raw, importPathsOf(baseVp)...)
	raw = append(raw, importPathsOf(envVp)...)

	var issues placeholderIssues
	expanded := make([]string, 0, len(raw))
	for _, p := range raw {
		v, pathIssues := expandEnvPlaceholder(p)
		issues.merge(pathIssues)
		if v != "" {
			expanded = append(expanded, v)
		}
	}
	if !issues.empty() {
		return nil, issues.err("collectImportPaths")
	}

	// 扩展名在这里一次性校验：viper 靠它判断文件格式，环境变体路径也靠它拼接。
	// 放到加载时才发现，报出来的会是 viper 的 "Unsupported Config Type"，
	// 看不出问题其实出在导入列表里
	for _, p := range expanded {
		if filepath.Ext(p) == "" {
			return nil, xerror.Newf("xconfig", "collectImportPaths",
				"imported config file has no extension, %s=[%s]", importConfigKey, p)
		}
	}
	return distinct(expanded), nil
}

// importPathsOf 读取单个配置里的导入列表
func importPathsOf(vp *viper.Viper) []string {
	if vp == nil {
		return nil
	}
	return vp.GetStringSlice(importConfigKey)
}

// loadImportedSettings 加载导入的配置文件，分成「无环境后缀」与「带环境后缀」两份
//
// 分两份而不是合成一份，是为了让带环境后缀的文件整体压过不带的，与 Spring 一致
// （官方文档：profile-specific files always overriding the non-specific ones）。
// 合成一份会让 db-dev.yml 被后面声明的 redis.yml 压过——一个环境专属的值
// 被一个非环境专属的值覆盖，说不通。
//
// 每一份内部按声明顺序，后声明的覆盖先声明的（同 Spring：later imports taking precedence）。
func loadImportedSettings(paths []string, baseDir, profilesActive string) (plain, profile map[string]any, err error) {
	plain = make(map[string]any)
	profile = make(map[string]any)

	for _, p := range paths {
		full := p
		if !filepath.IsAbs(full) {
			full = filepath.Join(baseDir, full)
		}

		// 显式声明要导入的文件不存在时直接失败：这与 application-{env}.yml
		// 不存在只告警不同——后者是约定俗成的可选项，而这个是使用者点名要的
		if !xutil.FileExist(full) {
			return nil, nil, xerror.Newf("xconfig", "loadImportedSettings",
				"imported config file not found, %s=[%s], resolved=[%s]", importConfigKey, p, full)
		}

		vp, err := loadLocalConfig(full)
		if err != nil {
			return nil, nil, xerror.Newf("xconfig", "loadImportedSettings",
				"load imported config file failed, location=[%s], err=[%v]", full, err)
		}
		plain = deepMerge(plain, importedSettingsOf(vp, full))

		if profilesActive == "" {
			continue
		}
		// 扩展名已由 collectImportPaths 校验，环境名已在构造主配置的环境变体时校验，
		// 此处不会再有构造失败的可能，用不返回错误的版本避免留下不可达分支
		envFull := profileVariantPath(full, profilesActive)
		if !xutil.FileExist(envFull) {
			continue
		}
		envVp, err := loadLocalConfig(envFull)
		if err != nil {
			return nil, nil, xerror.Newf("xconfig", "loadImportedSettings",
				"load imported env config file failed, location=[%s], err=[%v]", envFull, err)
		}
		profile = deepMerge(profile, importedSettingsOf(envVp, envFull))
	}

	return plain, profile, nil
}

// importedSettingsOf 取出被导入文件的设置，并剔除它无权决定的部分
//
// 被导入文件不能改写激活环境（同 application-{env}.yml 的理由：激活环境由
// 主配置/启动参数/环境变量决定），也不能再声明 Import——只认主配置的导入列表，
// 免去环检测，且嵌套导入会让「这个值到底从哪来」难以追查。忽略时打一条告警，
// 不静默丢弃。
func importedSettingsOf(vp *viper.Viper, location string) map[string]any {
	settings := vp.AllSettings()
	dropProfilesActive(settings)

	if len(vp.GetStringSlice(importConfigKey)) > 0 {
		xutil.WarnIfEnableDebug("XOne %s in imported config file is ignored, nested import is not supported, location=[%s]",
			importConfigKey, location)
	}
	dropImport(settings)
	return settings
}

// dropImport 从配置中移除 Server.Config.Import
func dropImport(settings map[string]any) {
	server, ok := settings[lowerKey(ServerConfigKey)].(map[string]any)
	if !ok {
		return
	}
	cfg, ok := server[configSubKey].(map[string]any)
	if !ok {
		return
	}
	delete(cfg, importSubKey)
	if len(cfg) == 0 {
		delete(server, configSubKey)
	}
	if len(server) == 0 {
		delete(settings, lowerKey(ServerConfigKey))
	}
}
