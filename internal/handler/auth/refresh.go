package auth

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	"github.com/744223454/taskpilot-server/internal/svc"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func RefreshHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		refreshToken, err := c.Cookie(jwtauth.RefreshCookieName)
		if err != nil || refreshToken == "" {
			writeAuthError(c, svcCtx, authlogic.ErrInvalidRefreshToken)
			return
		}

		session, err := authlogic.NewService(c.Request.Context(), svcCtx).Refresh(refreshToken)
		if err != nil {
			writeAuthError(c, svcCtx, err)
			return
		}
		if err := setSessionCookies(c, svcCtx, session); err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, session.Response)
	}
}
