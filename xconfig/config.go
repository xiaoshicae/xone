package xconfig

const (
	ServerConfigKey = "Server"
)

// Server 框架级服务配置，对应 YAML 中的 Server 块
type Server struct {
	// Name 服务名
	// required
	Name string `mapstructure:"Name"`

	// Version 服务版本号
	// optional default "v0.0.1"
	Version string `mapstructure:"Version"`

	// Profiles 环境相关配置
	// optional default nil
	Profiles *Profiles `mapstructure:"Profiles"`
}

// Profiles 环境配置
type Profiles struct {
	// Active 指定启用的环境，只允许字母、数字、下划线和短横线
	// required
	Active string `mapstructure:"Active"`
}
