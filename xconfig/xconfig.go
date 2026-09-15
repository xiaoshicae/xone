package xconfig

import (
	"reflect"
	"slices"
	"sync"

	"github.com/spf13/viper"

	"github.com/xiaoshicae/xone/v3/xerror"
	"github.com/xiaoshicae/xone/v3/xutil"
)

// 最终配置的只读存储。三个变量在 publish 中一次性写入，读取时共用一把锁。
var (
	storeMu sync.RWMutex

	// store 合并展开后的配置，viper 在这里只承担「大小写不敏感的读取 + 反序列化」
	store *viper.Viper

	// server Server 块，初始化时解析一次，避免每次读取都回到 store 里捞
	server = serverMergeDefault(Server{})

	// sources 实际加载的配置文件，按优先级从低到高
	sources []string
)

// emptyStore 未初始化时返回的空配置，进程内复用一份，避免每次调用都分配
var emptyStore = sync.OnceValue(viper.New)

// UnmarshalConfig 把 key 对应的配置反序列化到 conf，conf 必须是非 nil 指针
func UnmarshalConfig(key string, conf any) error {
	if err := checkParam(key, conf); err != nil {
		return err
	}
	if err := getStore().UnmarshalKey(key, conf); err != nil {
		return xerror.Newf("xconfig", "UnmarshalConfig", "unmarshal failed, key=[%s], err=[%v]", key, err)
	}
	return nil
}

// GetConfig 读取 key 对应的原始配置值，key 不存在时返回 nil
func GetConfig(key string) any {
	return getStore().Get(key)
}

// ContainKey 判断配置中是否存在 key
func ContainKey(key string) bool {
	return getStore().IsSet(key)
}

// GetString 按 string 读取 key 对应的配置值
func GetString(key string) string {
	return getStore().GetString(key)
}

// GetInt 按 int 读取 key 对应的配置值
func GetInt(key string) int {
	return getStore().GetInt(key)
}

// GetBool 按 bool 读取 key 对应的配置值
func GetBool(key string) bool {
	return getStore().GetBool(key)
}

// GetServerName 获取 Server.Name，未配置时返回默认值
func GetServerName() string {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return server.Name
}

// GetServerVersion 获取 Server.Version，未配置时返回默认值 v0.0.1
func GetServerVersion() string {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return server.Version
}

// GetProfilesActive 获取实际激活的环境，未启用任何环境时返回空串
//
// 返回的是最终生效的值：它可能来自启动参数或环境变量，而不只是配置文件里写的那个。
func GetProfilesActive() string {
	storeMu.RLock()
	defer storeMu.RUnlock()
	if server.Profiles == nil {
		return ""
	}
	return server.Profiles.Active
}

// Sources 返回本次实际加载的配置文件，按优先级从低到高（后者覆盖前者）
//
// 排查「这个值到底从哪来」时先看这个列表：它就是配置的全部来源。
func Sources() []string {
	storeMu.RLock()
	defer storeMu.RUnlock()
	return slices.Clone(sources)
}

func getStore() *viper.Viper {
	storeMu.RLock()
	defer storeMu.RUnlock()
	if store == nil {
		xutil.WarnIfEnableDebug("config not found, please init config first")
		return emptyStore()
	}
	return store
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
