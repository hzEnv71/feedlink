package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	allowOriginHeader      = "Access-Control-Allow-Origin"
	allowMethodsHeader     = "Access-Control-Allow-Methods"
	allowHeadersHeader     = "Access-Control-Allow-Headers"
	exposeHeadersHeader    = "Access-Control-Expose-Headers"
	allowCredentialsHeader = "Access-Control-Allow-Credentials"
)

// CorsMiddleware 跨域中间件
func CorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		setCORSHeaders(c)
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func setCORSHeaders(c *gin.Context) {
	c.Header(allowOriginHeader, "*")
	c.Header(allowMethodsHeader, "GET, POST, PUT, DELETE, OPTIONS")
	c.Header(allowHeadersHeader, "Origin, Content-Type, Authorization, Accept")
	c.Header(exposeHeadersHeader, "Content-Length, Content-Type")
	c.Header(allowCredentialsHeader, "true")
}
