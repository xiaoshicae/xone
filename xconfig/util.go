package xconfig

import (
	"reflect"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/spf13/viper"
)

const (
	serverNameConfigKey    = ServerConfigKey + ".Name"
	serverVersionConfigKey = ServerConfigKey + ".Version"

	defaultServerName    = "unknown.unknown.unknown"
	defaultServerVersion = "v0.0.1"

	dotEnvFileName = ".env"
)

// UnmarshalConfig 把 key 对应的配置反序列化到 conf，conf 必须是非 nil 指针
func UnmarshalConfig(key string, conf any) error {
	if err := checkParam(key, conf); err != nil {
		return err
	}
	if err := getViperConfig().UnmarshalKey(key, conf); err != nil {
		return xerror.Newf("xconfig", "UnmarshalConfig", "unmarshal failed, key=[%s], err=[%v]", key, err)
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

// GetServerVersion 获取Server的Version，如果没有配置则为默认值 v0.0.1
func GetServerVersion() string {
	return xutil.GetOrDefault(getViperConfig().GetString(serverVersionConfigKey), defaultServerVersion)
}

// emptyViperConfig 未初始化时返回的空配置，进程内复用一份，避免每次调用都分配
var emptyViperConfig = sync.OnceValue(viper.New)

func getViperConfig() *viper.Viper {
	vipMu.RLock()
	v := vip
	vipMu.RUnlock()
	if v == nil {
		xutil.WarnIfEnableDebug("config not found，please init config first")
		return emptyViperConfig()
	}
	return v
}

func checkParam(key string, conf any) error {
	if key == "" {
		return xerror.Newf("xconfig", "checkParam", "param key is empty")
	}
	if conf == nil {
		return xerror.Newf("xconfig", "checkParam", "param conf is nil")
	}
	v := reflect.ValueOf(conf)
	if v.Kind() != reflect.Ptr {
		return xerror.Newf("xconfig", "checkParam", "param conf is not ptr")
	}
	// 类型化的 nil 指针能通过 Kind 检查，但反序列化时无处可写
	if v.IsNil() {
		return xerror.Newf("xconfig", "checkParam", "param conf is a nil pointer")
	}
	return nil
}
