package handler

import (
	authhandler "github.com/744223454/taskpilot-server/internal/handler/auth"
	documenthandler "github.com/744223454/taskpilot-server/internal/handler/document"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	parsejobhandler "github.com/744223454/taskpilot-server/internal/handler/parsejob"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes wires all HTTP endpoints onto the Gin engine.
func RegisterRoutes(router *gin.Engine, serverCtx *svc.ServiceContext) {
	router.Use(middleware.CORS(serverCtx.Config.CORS.AllowedOrigins))

	router.GET("/healthz", HealthHandler(serverCtx))
	router.GET("/from/:name", TaskpilotHandler(serverCtx))

	api := router.Group("/api/v1")
	api.POST("/auth/register", authhandler.RegisterHandler(serverCtx))
	api.POST("/auth/login", authhandler.LoginHandler(serverCtx))

	protected := api.Group("")
	protected.Use(middleware.RequireAuth(serverCtx))
	protected.GET("/users/me", authhandler.MeHandler(serverCtx))
	protected.POST("/documents/text", middleware.LimitRequestBody(documenthandler.MaxTextDocumentBodyBytes), documenthandler.CreateTextHandler(serverCtx))
	protected.GET("/documents", documenthandler.ListHandler(serverCtx))
	protected.GET("/documents/:documentId", documenthandler.GetHandler(serverCtx))
	protected.DELETE("/documents/:documentId", documenthandler.DeleteHandler(serverCtx))
	protected.POST("/parse-jobs", parsejobhandler.CreateHandler(serverCtx))
	protected.GET("/parse-jobs/:jobId", parsejobhandler.GetHandler(serverCtx))
	protected.POST("/parse-jobs/:jobId/retry", parsejobhandler.RetryHandler(serverCtx))
	protected.GET("/documents/:documentId/latest-job", parsejobhandler.LatestHandler(serverCtx))
}
