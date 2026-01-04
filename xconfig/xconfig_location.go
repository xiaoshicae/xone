package xconfig

import (
	"os"

	"github.com/xiaoshicae/xone/xutil"
)

const (
	configLocationArgKey = "server.config.location"
	configLocationEnvKey = "SERVER_CONFIG_LOCATION"

	ymlConfigLocationInCurrentDir       = "./application.yml"
	ymlConfigLocationInCurrentConfDir   = "./conf/application.yml"
	ymlConfigLocationInCurrentConfigDir = "./config/application.yml"

	yamlConfigLocationInCurrentDir       = "./application.yaml"
	yamlConfigLocationInCurrentConfDir   = "./conf/application.yaml"
	yamlConfigLocationInCurrentConfigDir = "./config/application.yaml"
)

func detectConfigLocation() string {
	if loc := getLocationFromArg(); loc != "" {
		xutil.InfoIfEnableDebug("XOne detect config location [%s] from arg", loc)
		return loc
	}

	if loc := getLocationFromENV(); loc != "" {
		xutil.InfoIfEnableDebug("XOne detect config location [%s] from env", loc)
		return loc
	}

	if loc := getLocationFromCurrentDir(); loc != "" {
		xutil.InfoIfEnableDebug("XOne detect config location [%s] from current dir", loc)
		return loc
	}

	return ""
}

func getLocationFromArg() string {
	c, _ := xutil.GetConfigFromArgs(configLocationArgKey)
	return c
}

func getLocationFromENV() string {
	return os.Getenv(configLocationEnvKey)
}

func getLocationFromCurrentDir() string {
	if xutil.FileExist(ymlConfigLocationInCurrentDir) {
		return ymlConfigLocationInCurrentDir
	}
	if xutil.FileExist(yamlConfigLocationInCurrentDir) {
		return yamlConfigLocationInCurrentDir
	}

	if xutil.FileExist(ymlConfigLocationInCurrentConfDir) {
		return ymlConfigLocationInCurrentConfDir
	}
	if xutil.FileExist(yamlConfigLocationInCurrentConfDir) {
		return yamlConfigLocationInCurrentConfDir
	}

	if xutil.FileExist(ymlConfigLocationInCurrentConfigDir) {
		return ymlConfigLocationInCurrentConfigDir
	}
	if xutil.FileExist(ymlConfigLocationInCurrentConfigDir) {
		return yamlConfigLocationInCurrentConfigDir
	}

	return ""
}
