package xconfig

const (
	ServerConfigKey = "Server"
)

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

type Profiles struct {
	// Active 指定启用的环境
	// required
	Active string `mapstructure:"Active"`
}

func serverConfigMergeDefault(c *Server) *Server {
	if c == nil {
		c = &Server{}
	}
	if c.Version == "" {
		c.Version = "v0.0.1"
	}
	return c
}
