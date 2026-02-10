package xgin

import "github.com/xiaoshicae/xone/v2/xconfig"

const (
	ginConfigKey        = "XGin"
	ginSwaggerConfigKey = "XGin.Swagger"
)

// Config Gin 相关配置
type Config struct {
	// Host 服务监听的host
	// optional default "0.0.0.0"
	Host string `mapstructure:"Host"`

	// Port 服务端口号
	// optional default 8000
	Port int `mapstructure:"Port"`

	// UseHttp2 是否使用http2协议
	// optional default false
	UseHttp2 bool `mapstructure:"UseHttp2"`

	// Swagger swagger相关配置
	// optional default nil
	Swagger *SwaggerConfig `mapstructure:"Swagger"`
}

// SwaggerConfig swagger相关配置
type SwaggerConfig struct {
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

// GetConfig 获取Gin相关配置
func GetConfig() *Config {
	config := &Config{}
	_ = xconfig.UnmarshalConfig(ginConfigKey, config)
	return configMergeDefault(config)
}

// GetSwaggerConfig 获取Gin-Swagger相关配置
func GetSwaggerConfig() *SwaggerConfig {
	config := &SwaggerConfig{}
	_ = xconfig.UnmarshalConfig(ginSwaggerConfigKey, config)
	return swaggerConfigMergeDefault(config)
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port <= 0 {
		c.Port = 8000
	}
	if c.Swagger != nil {
		c.Swagger = swaggerConfigMergeDefault(c.Swagger)
	}
	return c
}

func swaggerConfigMergeDefault(c *SwaggerConfig) *SwaggerConfig {
	if c == nil {
		c = &SwaggerConfig{}
	}
	if len(c.Schemes) == 0 {
		c.Schemes = []string{"https", "http"}
	}
	return c
}
