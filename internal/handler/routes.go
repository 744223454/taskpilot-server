package handler

import (
	authhandler "github.com/744223454/taskpilot-server/internal/handler/auth"
	documenthandler "github.com/744223454/taskpilot-server/internal/handler/document"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	parsejobhandler "github.com/744223454/taskpilot-server/internal/handler/parsejob"
	parseresulthandler "github.com/744223454/taskpilot-server/internal/handler/parseresult"
	"github.com/744223454/taskpilot-server/internal/svc"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
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
	api.POST("/auth/refresh", middleware.RequireCookieCSRF(jwtauth.RefreshCookieName), authhandler.RefreshHandler(serverCtx))
	api.POST("/auth/logout", middleware.RequireCookieCSRF(jwtauth.RefreshCookieName), authhandler.LogoutHandler(serverCtx))

	protected := api.Group("")
	protected.Use(middleware.RequireAuth(serverCtx))
	protected.Use(middleware.RequireCSRFForCookieAuth())
	protected.GET("/users/me", authhandler.MeHandler(serverCtx))
	protected.POST("/documents/text", middleware.LimitRequestBody(documenthandler.MaxTextDocumentBodyBytes), documenthandler.CreateTextHandler(serverCtx))
	protected.GET("/documents", documenthandler.ListHandler(serverCtx))
	protected.GET("/documents/:documentId", documenthandler.GetHandler(serverCtx))
	protected.DELETE("/documents/:documentId", documenthandler.DeleteHandler(serverCtx))
	protected.POST("/parse-jobs", parsejobhandler.CreateHandler(serverCtx))
	protected.GET("/parse-jobs/:jobId", parsejobhandler.GetHandler(serverCtx))
	protected.POST("/parse-jobs/:jobId/retry", parsejobhandler.RetryHandler(serverCtx))
	protected.GET("/documents/:documentId/latest-job", parsejobhandler.LatestHandler(serverCtx))
	protected.GET("/parse-jobs/:jobId/result", parseresulthandler.GetByJobHandler(serverCtx))
	protected.GET("/parse-results/:resultId", parseresulthandler.GetHandler(serverCtx))
	protected.PUT("/parse-results/:resultId", parseresulthandler.UpdateHandler(serverCtx))
	protected.POST("/parse-results/:resultId/confirm", parseresulthandler.ConfirmHandler(serverCtx))
}
