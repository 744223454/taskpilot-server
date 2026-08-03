package middleware

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/744223454/taskpilot-server/internal/svc"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

const principalKey = "auth.principal"
const authSourceKey = "auth.source"

var errMissingBearerToken = errors.New("missing bearer token")

type Principal struct {
	UserID   int64
	Email    string
	Nickname string
}

type AuthSource string

const (
	AuthSourceBearer AuthSource = "bearer"
	AuthSourceCookie AuthSource = "cookie"
)

func RequireAuth(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		source := AuthSourceCookie
		var token string
		var err error
		if authorization != "" {
			source = AuthSourceBearer
			token, err = bearerToken(authorization)
		} else {
			token, err = c.Cookie(jwtauth.AccessCookieName)
		}
		if err != nil || strings.TrimSpace(token) == "" {
			writeUnauthorized(c)
			return
		}

		if svcCtx.JWT == nil {
			writeUnauthorized(c)
			return
		}
		claims, err := svcCtx.JWT.ParseToken(token)
		if err != nil || claims.UserID <= 0 {
			writeUnauthorized(c)
			return
		}

		c.Set(principalKey, Principal{
			UserID:   claims.UserID,
			Email:    claims.Email,
			Nickname: claims.Nickname,
		})
		c.Set(authSourceKey, source)
		c.Next()
	}
}

func AuthSourceFrom(c *gin.Context) (AuthSource, bool) {
	value, exists := c.Get(authSourceKey)
	if !exists {
		return "", false
	}
	source, ok := value.(AuthSource)
	return source, ok
}

func RequireCookieAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		source, ok := AuthSourceFrom(c)
		if !ok || source != AuthSourceCookie {
			writeUnauthorizedMessage(c, "access cookie required")
			return
		}
		c.Next()
	}
}

func RequireCSRFForCookieAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		source, ok := AuthSourceFrom(c)
		if !ok || source != AuthSourceCookie || csrfSafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		if !validCSRF(c) {
			writeCSRFForbidden(c)
			return
		}
		c.Next()
	}
}

func RequireCookieCSRF(cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := c.Cookie(cookieName); err != nil {
			writeUnauthorizedMessage(c, "invalid or missing refresh token")
			return
		}
		if !csrfSafeMethod(c.Request.Method) && !validCSRF(c) {
			writeCSRFForbidden(c)
			return
		}
		c.Next()
	}
}

func csrfSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func validCSRF(c *gin.Context) bool {
	cookieToken, err := c.Cookie(jwtauth.CSRFCookieName)
	if err != nil || cookieToken == "" {
		return false
	}
	headerToken := c.GetHeader(jwtauth.CSRFHeaderName)
	return headerToken != "" && len(cookieToken) == len(headerToken) && subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) == 1
}

func PrincipalFrom(c *gin.Context) (Principal, bool) {
	value, exists := c.Get(principalKey)
	if !exists {
		return Principal{}, false
	}
	principal, ok := value.(Principal)
	return principal, ok && principal.UserID > 0
}

func bearerToken(header string) (string, error) {
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return "", errMissingBearerToken
	}
	return fields[1], nil
}

func writeUnauthorized(c *gin.Context) {
	writeUnauthorizedMessage(c, "invalid or missing access token")
}

func writeUnauthorizedMessage(c *gin.Context, message string) {
	response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, message)
	c.Abort()
}

func writeCSRFForbidden(c *gin.Context) {
	response.Error(c, http.StatusForbidden, bizerrors.CodeForbidden, "invalid or missing csrf token")
	c.Abort()
}
