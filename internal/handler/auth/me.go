package auth

import (
	"net/http"

	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func MeHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := bearerToken(c.GetHeader("Authorization"))
		if err != nil {
			writeAuthError(c, err)
			return
		}

		resp, err := authlogic.NewService(c.Request.Context(), svcCtx).CurrentUser(token)
		if err != nil {
			writeAuthError(c, err)
			return
		}

		response.Success(c, http.StatusOK, resp)
	}
}
