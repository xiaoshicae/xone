package xgin

import (
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"
)

const (
	ginConfigKey        = "XGin"
	ginSwaggerConfigKey = "XGin.Swagger"

	defaultHost = "0.0.0.0"
	defaultPort = 8000
)

var defaultSchemes = []string{"https", "http"}

// Config Gin 相关配置
type Config struct {
	// Host 服务监听的host
	// optional default "0.0.0.0"
	Host string `mapstructure:"Host"`

	// Port 服务端口号
	// optional default 8000
	Port int `mapstructure:"Port"`

	// UseH2C 是否启用 h2c（HTTP/2 Cleartext，非 TLS 下的 HTTP/2）
	// TLS 模式下 HTTP/2 自动启用，无需此配置
	// optional default false
	UseH2C bool `mapstructure:"UseH2C"`

	// CertFile TLS 证书路径
	// optional default ""
	CertFile string `mapstructure:"CertFile"`

	// KeyFile TLS 私钥路径
	// optional default ""
	KeyFile string `mapstructure:"KeyFile"`

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
	if err := xconfig.UnmarshalConfig(ginConfigKey, config); err != nil {
		xutil.WarnIfEnableDebug("XGin GetConfig unmarshal failed, use default, err=[%v]", err)
	}
	return configMergeDefault(config)
}

// GetSwaggerConfig 获取Gin-Swagger相关配置
func GetSwaggerConfig() *SwaggerConfig {
	config := &SwaggerConfig{}
	if err := xconfig.UnmarshalConfig(ginSwaggerConfigKey, config); err != nil {
		xutil.WarnIfEnableDebug("XGin GetSwaggerConfig unmarshal failed, use default, err=[%v]", err)
	}
	return swaggerConfigMergeDefault(config)
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.Host == "" {
		c.Host = defaultHost
	}
	if c.Port <= 0 {
		c.Port = defaultPort
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
		c.Schemes = append([]string{}, defaultSchemes...)
	}
	return c
}
