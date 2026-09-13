package options

// SwaggerUrlPrefix 设置 Swagger 文档路由的前缀，如 "/api/v1"
func SwaggerUrlPrefix(urlPrefix string) SwaggerOption {
	return func(o *SwaggerOptions) {
		o.UrlPrefix = urlPrefix
	}
}

type SwaggerOption func(*SwaggerOptions)

type SwaggerOptions struct {
	UrlPrefix string
}

func DefaultSwaggerOptions() *SwaggerOptions {
	return &SwaggerOptions{
		UrlPrefix: "",
	}
}
