package handler

import (
	authhandler "github.com/744223454/taskpilot-server/internal/handler/auth"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes wires all HTTP endpoints onto the Gin engine.
func RegisterRoutes(router *gin.Engine, serverCtx *svc.ServiceContext) {
	router.GET("/healthz", HealthHandler(serverCtx))
	router.GET("/from/:name", TaskpilotHandler(serverCtx))

	api := router.Group("/api/v1")
	api.POST("/auth/register", authhandler.RegisterHandler(serverCtx))
	api.POST("/auth/login", authhandler.LoginHandler(serverCtx))
	api.GET("/users/me", authhandler.MeHandler(serverCtx))
}
