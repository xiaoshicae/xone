package swagger

import (
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

const SwaggerUrl = "/swagger/*any"

var SwaggerHandler = ginSwagger.WrapHandler(swaggerfiles.Handler)
