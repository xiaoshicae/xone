package xpipeline

import (
	"sync"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"
)

// XPipelineConfigKey 配置 key
const XPipelineConfigKey = "XPipeline"

const defaultBufferSize = 64

// Config xpipeline 配置
type Config struct {
	// BufferSize channel 缓冲大小，默认 64
	BufferSize int `mapstructure:"BufferSize"`
	// DisableMonitor 是否禁用监控，默认 false（即默认开启监控）
	DisableMonitor bool `mapstructure:"DisableMonitor"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	c.BufferSize = xutil.GetOrDefault(c.BufferSize, defaultBufferSize)
	return c
}

var (
	cachedConfig     *Config
	cachedConfigOnce sync.Once
)

// GetConfig 获取 xpipeline 配置（结果会被缓存，仅首次调用时反序列化）
func GetConfig() *Config {
	cachedConfigOnce.Do(func() {
		c := &Config{}
		if err := xconfig.UnmarshalConfig(XPipelineConfigKey, c); err != nil {
			cachedConfig = configMergeDefault(nil)
			return
		}
		cachedConfig = configMergeDefault(c)
	})
	return cachedConfig
}
