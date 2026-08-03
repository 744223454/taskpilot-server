package auth

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func MeHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}

		resp, err := authlogic.NewService(c.Request.Context(), svcCtx).CurrentUserByID(principal.UserID)
		if err != nil {
			writeAuthError(c, svcCtx, err)
			return
		}

		response.Success(c, http.StatusOK, resp)
	}
}

func UpdateMeHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		refreshToken, err := c.Cookie(jwtauth.RefreshCookieName)
		if err != nil || refreshToken == "" {
			writeAuthError(c, svcCtx, authlogic.ErrInvalidRefreshToken)
			return
		}

		var req types.UpdateUserRequest
		if err := common.BindJSONStrict(c, &req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		session, err := authlogic.NewService(c.Request.Context(), svcCtx).UpdateCurrentUser(principal.UserID, refreshToken, &req)
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
