package xflow

import (
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

// GetConfig 获取 xflow 配置
func GetConfig() *Config {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XFlowConfigKey, c); err != nil {
		return configMergeDefault(nil)
	}
	return configMergeDefault(c)
}
