package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const strictTransportSecurity = "max-age=31536000; includeSubDomains"

func Secure(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}

		if requestIsHTTPS(c.Request) {
			c.Header("Strict-Transport-Security", strictTransportSecurity)
			c.Next()
			return
		}

		target := "https://" + c.Request.Host + c.Request.URL.RequestURI()
		c.Redirect(http.StatusPermanentRedirect, target)
		c.Abort()
	}
}

func NoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}

func Timeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestContext, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	}
}

func requestIsHTTPS(request *http.Request) bool {
	if request.TLS != nil {
		return true
	}
	forwardedProto := request.Header.Get("X-Forwarded-Proto")
	if separator := strings.IndexByte(forwardedProto, ','); separator >= 0 {
		forwardedProto = forwardedProto[:separator]
	}
	return strings.EqualFold(strings.TrimSpace(forwardedProto), "https")
}
