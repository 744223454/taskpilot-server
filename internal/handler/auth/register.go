package auth

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
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
		retryAfter, err := enforceRegisterRateLimit(c.Request.Context(), svcCtx, c.ClientIP())
		if err != nil {
			setRetryAfterHeader(c, retryAfter)
			writeAuthError(c, svcCtx, err)
			return
		}

		resp, err := authlogic.NewService(c.Request.Context(), svcCtx).Register(&req)
		if err != nil {
			writeAuthError(c, svcCtx, err)
			return
		}
		if err := setSessionCookies(c, svcCtx, resp); err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}

		response.Success(c, http.StatusCreated, resp.Response)
	}
}
