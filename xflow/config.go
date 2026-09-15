package xflow

import (
	"time"

	"github.com/xiaoshicae/xone/v3/xutil"
)

const (
	// XFlowConfigKey 配置 key
	XFlowConfigKey = "XFlow"

	// defaultRollbackTimeoutStr 回滚总超时默认值
	defaultRollbackTimeoutStr = "30s"
)

// Config xflow 配置
type Config struct {
	// EnableMonitor 是否开启监控
	// optional default true
	EnableMonitor *bool `mapstructure:"EnableMonitor"`

	// RollbackTimeout 回滚全部处理器的总超时
	//
	// 回滚不沿用调用方的 context（否则请求超时后补偿必然失败），
	// 改由本项单独限时，避免补偿逻辑无限期挂住退出流程。
	// optional default "30s"
	RollbackTimeout string `mapstructure:"RollbackTimeout"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.EnableMonitor == nil {
		c.EnableMonitor = xutil.ToPtr(true)
	}
	if xutil.ToDuration(c.RollbackTimeout) <= 0 {
		if c.RollbackTimeout != "" {
			xutil.WarnIfEnableDebug("XOne xflow RollbackTimeout is invalid, fallback to %s, got=[%s]",
				defaultRollbackTimeoutStr, c.RollbackTimeout)
		}
		c.RollbackTimeout = defaultRollbackTimeoutStr
	}
	return c
}

// rollbackTimeout 返回当前生效的回滚总超时
func rollbackTimeout() time.Duration {
	return time.Duration(rollbackTimeoutNanos.Load())
}
