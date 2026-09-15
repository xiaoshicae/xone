package xconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/spf13/viper"
)

const (
	profilesActiveArgKey    = "server.profiles.active"
	profilesActiveConfigKey = "Server.Profiles.Active"
	profilesActiveEnvKey    = "SERVER_PROFILES_ACTIVE"
)

// profilesActivePattern 激活环境名的合法字符集
//
// 环境名会被拼进配置文件路径，必须限制字符集：否则 SERVER_PROFILES_ACTIVE=../../etc/x
// 会让进程去加载任意路径下的 YAML。
var profilesActivePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func detectProfilesActive(vip *viper.Viper) string {
	if pa := getProfilesActiveFromArg(); pa != "" {
		xutil.InfoIfEnableDebug("XOne detect profiles active [%s] from arg", pa)
		return pa
	}

	if pa := getProfilesActiveFromENV(); pa != "" {
		xutil.InfoIfEnableDebug("XOne detect profiles active [%s] from env", pa)
		return pa
	}

	if pa := getProfilesActiveFromViperConfig(vip); pa != "" {
		xutil.InfoIfEnableDebug("XOne detect profiles active [%s] from base config file", pa)
		return pa
	}

	xutil.InfoIfEnableDebug("XOne config profiles active not found")
	return ""
}

func getProfilesActiveFromArg() string {
	c, _ := xutil.GetConfigFromArgs(profilesActiveArgKey)
	return c
}

func getProfilesActiveFromENV() string {
	return os.Getenv(profilesActiveEnvKey)
}

func getProfilesActiveFromViperConfig(vp *viper.Viper) string {
	if vp == nil {
		return ""
	}
	// 此时整份配置尚未展开占位符，单独展开这一个值即可；
	// 环境变量缺失时返回原样，随后的字符集校验会拦下它
	expanded, _ := expandEnvPlaceholder(vp.GetString(profilesActiveConfigKey))
	return expanded
}

// toProfilesActiveConfigLocation 根据基础配置文件路径和激活的环境，构建环境配置文件路径
// 例如: ./conf/application.yml + dev -> ./conf/application-dev.yml
func toProfilesActiveConfigLocation(configLocation string, pa string) (string, error) {
	if !profilesActivePattern.MatchString(pa) {
		return "", xerror.Newf("xconfig", "init",
			"profiles active is invalid, only letters, digits, '_' and '-' are allowed, profiles_active=[%s]", pa)
	}

	if filepath.Ext(configLocation) == "" {
		return "", xerror.Newf("xconfig", "init", "config file name is invalid, no extension found")
	}
	return profileVariantPath(configLocation, pa), nil
}

// profileVariantPath 构造环境变体路径，要求调用方已校验扩展名与环境名
//
// 例如: ./conf/db.yml + dev -> ./conf/db-dev.yml
func profileVariantPath(configLocation, pa string) string {
	ext := filepath.Ext(configLocation)
	return fmt.Sprintf("%s-%s%s", strings.TrimSuffix(configLocation, ext), pa, ext)
}
