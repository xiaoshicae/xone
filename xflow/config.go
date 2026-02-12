package xflow

import (
	"sync"

	"github.com/xiaoshicae/xone/v2/xconfig"
)

// XFlowConfigKey 配置 key
const XFlowConfigKey = "XFlow"

// Config xflow 配置
type Config struct {
	// DisableMonitor 是否禁用监控，默认 false（即默认开启监控）
	DisableMonitor bool `mapstructure:"DisableMonitor"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	return c
}

var (
	cachedConfig     *Config
	cachedConfigOnce sync.Once
)

// GetConfig 获取 xflow 配置（结果会被缓存，仅首次调用时反序列化）
func GetConfig() *Config {
	cachedConfigOnce.Do(func() {
		c := &Config{}
		if err := xconfig.UnmarshalConfig(XFlowConfigKey, c); err != nil {
			cachedConfig = configMergeDefault(nil)
			return
		}
		cachedConfig = configMergeDefault(c)
	})
	return cachedConfig
}
