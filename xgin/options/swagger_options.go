package options

func WithSwaggerUrlPrefix(urlPrefix string) SwaggerOption {
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
