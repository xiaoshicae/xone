package xconfig

import (
	"fmt"
	"reflect"
	"time"

	"github.com/xiaoshicae/xone/xutil"

	"github.com/spf13/viper"
)

const (
	serverNameConfigKey    = ServerConfigKey + ".Name"
	serverVersionConfigKey = ServerConfigKey + ".Version"

	defaultServerName    = "unknown.unknown.unknown"
	defaultServerVersion = "v0.0.1"

	dotEnvFileName = ".env"
)

func UnmarshalConfig(key string, conf interface{}) error {
	if err := checkParam(key, conf); err != nil {
		return err
	}
	if err := getViperConfig().UnmarshalKey(key, conf); err != nil {
		return err
	}
	return nil
}

func GetConfig(key string) any {
	return getViperConfig().Get(key)
}

func ContainKey(key string) bool {
	return getViperConfig().IsSet(key)
}

func GetString(key string) string {
	return getViperConfig().GetString(key)
}

func GetBool(key string) bool {
	return getViperConfig().GetBool(key)
}

func GetInt(key string) int {
	return getViperConfig().GetInt(key)
}

func GetInt32(key string) int32 {
	return getViperConfig().GetInt32(key)
}

func GetInt64(key string) int64 {
	return getViperConfig().GetInt64(key)
}

func GetFloat64(key string) float64 {
	return getViperConfig().GetFloat64(key)
}

func GetDuration(key string) time.Duration {
	return getViperConfig().GetDuration(key)
}

func GetStringSlice(key string) []string {
	return getViperConfig().GetStringSlice(key)
}

func GetIntSlice(key string) []int {
	return getViperConfig().GetIntSlice(key)
}

// ************ server 相关配置获取 ************

// GetServerName 获取Server的Name，如果没有配置则为默认值
func GetServerName() string {
	return xutil.GetOrDefault(GetRawServerName(), defaultServerName)
}

// GetRawServerName 获取Server的Name，如果没有配置则为空
func GetRawServerName() string {
	return getViperConfig().GetString(serverNameConfigKey)
}

// GetServerVersion 获取Server的Version，如果没有配置则为空
func GetServerVersion() string {
	return xutil.GetOrDefault(getViperConfig().GetString(serverVersionConfigKey), defaultServerVersion)
}

func getViperConfig() *viper.Viper {
	if vip == nil {
		xutil.WarnIfEnableDebug("config not found，please init config first")
		return viper.New()
	}
	return vip
}

func checkParam(key string, conf interface{}) error {
	if key == "" {
		return fmt.Errorf("param key is empty")
	}
	if conf == nil {
		return fmt.Errorf("param conf is nil")
	}
	if reflect.TypeOf(conf).Kind() != reflect.Ptr {
		return fmt.Errorf("param conf is not ptr")
	}
	return nil
}
