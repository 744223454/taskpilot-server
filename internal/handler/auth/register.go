package auth

import (
	"net/http"

	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func RegisterHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, bizerrors.CodeBadRequest, err.Error())
			return
		}

		resp, err := authlogic.NewService(c.Request.Context(), svcCtx).Register(&req)
		if err != nil {
			writeAuthError(c, err)
			return
		}

		response.Success(c, http.StatusCreated, resp)
	}
}
