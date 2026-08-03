package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/744223454/taskpilot-server/internal/svc"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

type healthResponse struct {
	Status string `json:"status"`
	DB     bool   `json:"db"`
	Redis  bool   `json:"redis"`
	Worker bool   `json:"worker"`
}

func HealthHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := dependencyStatus(c.Request.Context(), svcCtx)
		status.Status = "ok"
		response.Success(c, http.StatusOK, status)
	}
}

func ReadinessHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := dependencyStatus(c.Request.Context(), svcCtx)
		if status.DB && status.Redis && status.Worker {
			status.Status = "ready"
			response.Success(c, http.StatusOK, status)
			return
		}
		response.Error(c, http.StatusServiceUnavailable, bizerrors.CodeServiceUnavailable, "service not ready")
	}
}

func dependencyStatus(requestContext context.Context, svcCtx *svc.ServiceContext) healthResponse {
	status := healthResponse{}
	checkContext, cancel := context.WithTimeout(requestContext, 500*time.Millisecond)
	defer cancel()
	if svcCtx.DB != nil {
		if sqlDB, err := svcCtx.DB.DB(); err == nil {
			status.DB = sqlDB.PingContext(checkContext) == nil
		}
	}
	if svcCtx.Redis != nil {
		status.Redis = svcCtx.Redis.Ping(checkContext).Err() == nil
		if status.Redis && svcCtx.ParseJobs != nil {
			status.Worker, _ = svcCtx.ParseJobs.WorkerHealthy(checkContext)
		}
	}
	return status
}
