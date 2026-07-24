package auth

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	"github.com/744223454/taskpilot-server/internal/svc"
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
