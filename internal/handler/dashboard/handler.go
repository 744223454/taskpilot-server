package dashboard

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	dashboardlogic "github.com/744223454/taskpilot-server/internal/logic/dashboard"
	"github.com/744223454/taskpilot-server/internal/svc"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func StatsHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}

		stats, err := dashboardlogic.NewService(c.Request.Context(), svcCtx).Stats(principal.UserID)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, stats)
	}
}

func RemindersHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}

		reminders, err := dashboardlogic.NewService(c.Request.Context(), svcCtx).Reminders(principal.UserID)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, reminders)
	}
}
