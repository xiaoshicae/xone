package xlog

const (
	XLogConfigKey = "XLog"
)

type Config struct {
	// Level 日志级别
	// optional default "info"
	Level string `mapstructure:"Level"`

	// Name 日志文件名称
	// optional default "app"
	Name string `mapstructure:"Name"`

	// Path 日志文件夹路径
	// optional default "./log"
	Path string `mapstructure:"Path"`

	// Console 日志内容是否需要在控制台打印
	// optional default false
	Console bool `mapstructure:"Console"`

	// ConsoleFormatIsRaw 在控制台打印的日志是否为原始格式(即底层的json格式)，为false时，打印level+time+filename+func+traceid+内容
	// optional default false
	ConsoleFormatIsRaw bool `mapstructure:"ConsoleFormatIsRaw"`

	// MaxAge 日志保存最大时间
	// optional default "7d"
	MaxAge string `mapstructure:"MaxAge"`

	// RotateTime 日志切割时长
	// optional default "1d"
	RotateTime string `mapstructure:"RotateTime"`

	// Timezone 日志时间的时区
	// optional default "Asia/Shanghai"
	Timezone string `mapstructure:"Timezone"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
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
