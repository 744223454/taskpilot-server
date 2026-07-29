package project

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	projectlogic "github.com/744223454/taskpilot-server/internal/logic/project"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func CreateHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.CreateProjectRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			common.WriteBindingError(c, err)
			return
		}

		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}

		project, created, err := projectlogic.NewService(c.Request.Context(), svcCtx).Create(principal.UserID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		response.Success(c, status, project)
	}
}
