package handler

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

type healthResponse struct {
	Status string `json:"status"`
	DB     bool   `json:"db"`
}

func TaskpilotHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &types.Request{
			Name: c.Param("name"),
		}

		l := logic.NewTaskpilotLogic(c.Request.Context(), svcCtx)
		resp, err := l.Taskpilot(req)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, bizerrors.CodeInternalError, err.Error())
			return
		}

		response.Success(c, http.StatusOK, resp)
	}
}

func HealthHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		response.Success(c, http.StatusOK, healthResponse{
			Status: "ok",
			DB:     svcCtx.DB != nil,
		})
	}
}
