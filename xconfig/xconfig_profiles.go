package xconfig

import (
	"fmt"
	"os"
	"path/filepath"
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
	return vp.GetString(profilesActiveConfigKey)
}

// toProfilesActiveConfigLocation 根据基础配置文件路径和激活的环境，构建环境配置文件路径
// 例如: ./conf/application.yml + dev -> ./conf/application-dev.yml
func toProfilesActiveConfigLocation(configLocation string, pa string) (string, error) {
	ext := filepath.Ext(configLocation)
	if ext == "" {
		return "", xerror.Newf("xconfig", "init", "config file name is invalid, no extension found")
	}

	// 去掉扩展名，添加环境后缀，再加回扩展名
	nameWithoutExt := strings.TrimSuffix(configLocation, ext)
	return fmt.Sprintf("%s-%s%s", nameWithoutExt, pa, ext), nil
}
