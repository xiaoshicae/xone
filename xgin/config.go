package xgin

import (
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"
)

const (
	// XGinConfigKey xgin 配置根 key
	XGinConfigKey = "XGin"
	// XGinSwaggerConfigKey xgin swagger 配置 key
	XGinSwaggerConfigKey = "XGin.Swagger"

	defaultHost = "0.0.0.0"
	// defaultPort 与 gin 自身的默认端口一致（gin 的 resolveAddress 在未指定时返回 :8080）
	defaultPort = 8080

	// defaultReadHeaderTimeout 读取请求头的超时，slowloris 类攻击的主要防线
	defaultReadHeaderTimeout = "10s"
	// defaultIdleTimeout keep-alive 连接的空闲超时
	defaultIdleTimeout = "60s"
	// defaultGracefulStopTimeout 优雅退出超时
	//
	// 取值小于 K8s terminationGracePeriodSeconds 的默认值（30s）：
	// 两者相等意味着 Shutdown 还没走完 pod 就被 SIGKILL，等于没有优雅退出
	defaultGracefulStopTimeout = "25s"
)

var defaultSchemes = []string{"https", "http"}

// Config Gin 相关配置
type Config struct {
	// Host 服务监听的host
	// optional default "0.0.0.0"
	Host string `mapstructure:"Host"`

	// Port 服务端口号
	// optional default 8080
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

	// ReadHeaderTimeout 读取请求头的超时时间
	// 不设置则慢客户端可以一直占着连接不放，连接数打满后服务不可用
	// optional default "10s"
	ReadHeaderTimeout string `mapstructure:"ReadHeaderTimeout"`

	// ReadTimeout 读取整个请求（含 body）的超时时间
	// 默认不限制：限制它会打断大文件上传等长时间请求，需要时按业务上限配置
	// optional default "" (不限制)
	ReadTimeout string `mapstructure:"ReadTimeout"`

	// WriteTimeout 写响应的超时时间
	// 默认不限制：限制它会打断 SSE、长轮询、大文件下载，需要时按业务上限配置
	// optional default "" (不限制)
	WriteTimeout string `mapstructure:"WriteTimeout"`

	// IdleTimeout keep-alive 连接的空闲超时时间
	// optional default "60s"
	IdleTimeout string `mapstructure:"IdleTimeout"`

	// GracefulStopTimeout 优雅退出超时，超时后未处理完的连接被强制关闭
	// 建议小于部署环境的进程终止宽限期（如 K8s terminationGracePeriodSeconds）
	// optional default "25s"
	GracefulStopTimeout string `mapstructure:"GracefulStopTimeout"`

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
//
// 解析失败时回退到默认值，供业务在非启动路径上查询使用。
// 启动路径请用 getConfig()，配置写错必须让服务起不来，而不是安静地用默认端口
func GetConfig() *Config {
	c, err := getConfig()
	if err != nil {
		xutil.WarnIfEnableDebug("XGin GetConfig unmarshal failed, use default, err=[%v]", err)
		return configMergeDefault(nil)
	}
	return c
}

// getConfig 获取Gin相关配置，解析失败返回 error
func getConfig() (*Config, error) {
	config := &Config{}
	if err := xconfig.UnmarshalConfig(XGinConfigKey, config); err != nil {
		return nil, err
	}
	return configMergeDefault(config), nil
}

// GetSwaggerConfig 获取Gin-Swagger相关配置
func GetSwaggerConfig() *SwaggerConfig {
	config := &SwaggerConfig{}
	if err := xconfig.UnmarshalConfig(XGinSwaggerConfigKey, config); err != nil {
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
	if c.ReadHeaderTimeout == "" {
		c.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if c.IdleTimeout == "" {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.GracefulStopTimeout == "" {
		c.GracefulStopTimeout = defaultGracefulStopTimeout
	}
	// ReadTimeout / WriteTimeout 默认不限制，空串即为默认值，无需设置
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
