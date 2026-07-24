package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/744223454/taskpilot-server/internal/svc"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

const principalKey = "auth.principal"

var errMissingBearerToken = errors.New("missing bearer token")

type Principal struct {
	UserID   int64
	Email    string
	Nickname string
}

func RequireAuth(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := bearerToken(c.GetHeader("Authorization"))
		if err != nil {
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
		c.Next()
	}
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
	response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid or missing access token")
	c.Abort()
}
