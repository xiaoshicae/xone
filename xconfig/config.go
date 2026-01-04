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

	// Gin gin相关配置
	// optional default nil
	Gin *Gin `mapstructure:"Gin"`
}

type Profiles struct {
	// Active 指定启用的环境
	// required
	Active string `mapstructure:"Active"`
}

type Gin struct {
	// Host 服务监听的host
	// optional default "0.0.0.0"
	Host string `mapstructure:"Host"`

	// Port 服务端口号
	// optional default 8080
	Port int `mapstructure:"Port"`

	// UseHttp2 是否使用http2协议
	// optional default false
	UseHttp2 bool `mapstructure:"UseHttp2"`

	// GinSwagger swagger相关配置
	// optional default nil
	GinSwagger *GinSwagger `mapstructure:"GinSwagger"`
}

type GinSwagger struct {
	// Host 提供api服务的host
	// optional default ""
	Host string `mapstructure:"Host"`

	// BasePath api公共前缀
	// optional default ""
	BasePath string `mapstructure:"BasePath"`

	// Title api管理后台的title
	// optional default ""
	Title string `mapstructure:"Title"`

	// Description api管理后台的描述信息
	// optional default ""
	Description string `mapstructure:"Description"`

	// Schemes api支持的协议
	// optional default ["https", "http"]
	Schemes []string `mapstructure:"Schemes"`
}

func serverConfigMergeDefault(c *Server) *Server {
	if c == nil {
		c = &Server{}
	}
	if c.Version == "" {
		c.Version = "v0.0.1"
	}
	if c.Gin != nil {
		c.Gin = ginConfigMergeDefault(c.Gin)
	}
	return c
}

func ginConfigMergeDefault(c *Gin) *Gin {
	if c == nil {
		c = &Gin{}
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port <= 0 {
		c.Port = 8000
	}
	if c.GinSwagger != nil {
		c.GinSwagger = ginSwaggerConfigMergeDefault(c.GinSwagger)
	}
	return c
}

func ginSwaggerConfigMergeDefault(c *GinSwagger) *GinSwagger {
	if c == nil {
		c = &GinSwagger{}
	}
	if len(c.Schemes) == 0 {
		c.Schemes = []string{"https", "http"}
	}
	return c
}
