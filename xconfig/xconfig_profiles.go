package xconfig

import (
	"fmt"
	"os"
	"strings"

	"github.com/xiaoshicae/xone/xutil"

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

func toProfilesActiveConfigLocation(configLocation string, pa string) (string, error) {
	items := strings.Split(configLocation, ".")
	if len(items) < 2 {
		return "", fmt.Errorf("config file name is invalid")
	}
	dir := strings.Join(items[:len(items)-1], ".")
	typ := items[len(items)-1]
	return fmt.Sprintf("%s-%s.%s", dir, pa, typ), nil
}
