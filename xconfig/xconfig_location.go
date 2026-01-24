package xconfig

import (
	"os"

	"github.com/xiaoshicae/xone/xutil"
)

const (
	configLocationArgKey = "server.config.location"
	configLocationEnvKey = "SERVER_CONFIG_LOCATION"
)

// configLocationPaths 配置文件搜索路径列表，按优先级排序
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
	for _, loc := range configLocationPaths {
		if xutil.FileExist(loc) {
			return loc
		}
	}
	return ""
}
