package auth

import (
	"net/http"

	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	"github.com/744223454/taskpilot-server/internal/svc"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func LogoutHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		refreshToken, err := c.Cookie(jwtauth.RefreshCookieName)
		if err != nil || refreshToken == "" {
			clearSessionCookies(c, svcCtx)
			writeAuthError(c, svcCtx, authlogic.ErrInvalidRefreshToken)
			return
		}

		logoutErr := authlogic.NewService(c.Request.Context(), svcCtx).Logout(refreshToken)
		clearSessionCookies(c, svcCtx)
		if logoutErr != nil {
			writeAuthError(c, svcCtx, logoutErr)
			return
		}
		response.Success(c, http.StatusOK, nil)
	}
}
