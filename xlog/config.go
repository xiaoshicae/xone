package xlog

import (
	"github.com/xiaoshicae/xone/v2/xutil"
)

const (
	XLogConfigKey = "XLog"
)

type Config struct {
	// Level 日志级别
	// optional default "info"
	Level string `mapstructure:"Level"`

	// EnableFile 是否将日志写入文件
	// 默认关闭，日志仅打印到标准输出，适配 K8s 等由采集器收集容器标准输出的环境
	// 需要日志落盘时设为 true，此时 Name/Path/MaxAge/RotateTime 才生效
	// optional default false
	EnableFile bool `mapstructure:"EnableFile"`

	// Name 日志文件名称(仅 EnableFile 为 true 时生效)
	// optional default "app"
	Name string `mapstructure:"Name"`

	// Path 日志文件夹路径(仅 EnableFile 为 true 时生效)
	// optional default "./log"
	Path string `mapstructure:"Path"`

	// EnableConsole 日志内容是否需要在控制台(标准输出)打印
	// 显式配置为 false 且 EnableFile 也为 false 时，会被强制置为 true，避免日志无处输出
	// optional default true
	EnableConsole *bool `mapstructure:"EnableConsole"`

	// ConsoleFormatIsRaw 在控制台打印的日志是否为原始格式(即底层的json格式)，为false时，打印level+time+filename+func+traceid+内容
	// optional default false
	ConsoleFormatIsRaw bool `mapstructure:"ConsoleFormatIsRaw"`

	// MaxAge 日志保存最大时间(仅 EnableFile 为 true 时生效)
	// optional default "7d"
	MaxAge string `mapstructure:"MaxAge"`

	// RotateTime 日志切割时长(仅 EnableFile 为 true 时生效)
	// optional default "1d"
	RotateTime string `mapstructure:"RotateTime"`

	// Timezone 日志时间的时区
	// optional default "Asia/Shanghai"
	Timezone string `mapstructure:"Timezone"`
}

// configMergeDefault 合并默认配置，入参为 nil 时返回全默认配置
// 该函数幂等，重复调用结果一致
func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.EnableConsole == nil {
		c.EnableConsole = xutil.ToPtr(true)
	}
	// 文件和控制台都关闭时日志无处可去，强制打开控制台输出
	if !c.EnableFile && !*c.EnableConsole {
		c.EnableConsole = xutil.ToPtr(true)
	}
	if c.Name == "" {
		c.Name = "app"
	}
	if c.Level == "" {
		c.Level = "info"
	}
	if c.Path == "" {
		c.Path = "./log"
	}
	if c.MaxAge == "" {
		c.MaxAge = "7d"
	}
	if c.RotateTime == "" {
		c.RotateTime = "1d"
	}
	if c.Timezone == "" {
		c.Timezone = "Asia/Shanghai"
	}
	return c
}
