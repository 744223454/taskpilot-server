package handler

import (
	"time"

	authhandler "github.com/744223454/taskpilot-server/internal/handler/auth"
	documenthandler "github.com/744223454/taskpilot-server/internal/handler/document"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	parsejobhandler "github.com/744223454/taskpilot-server/internal/handler/parsejob"
	parseresulthandler "github.com/744223454/taskpilot-server/internal/handler/parseresult"
	projecthandler "github.com/744223454/taskpilot-server/internal/handler/project"
	taskhandler "github.com/744223454/taskpilot-server/internal/handler/task"
	"github.com/744223454/taskpilot-server/internal/svc"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	"github.com/gin-gonic/gin"
)

const requestTimeout = 30 * time.Second

// RegisterRoutes wires all HTTP endpoints onto the Gin engine.
func RegisterRoutes(router *gin.Engine, serverCtx *svc.ServiceContext) {
	router.Use(middleware.CORS(serverCtx.Config.CORS.AllowedOrigins))
	router.Use(middleware.Secure(serverCtx.Config.Auth.CookieSecure))
	router.Use(middleware.Timeout(requestTimeout))

	router.GET("/healthz", HealthHandler(serverCtx))
	router.GET("/readyz", ReadinessHandler(serverCtx))

	api := router.Group("/api/v1")
	api.Use(middleware.NoCache())
	api.POST("/auth/register", authhandler.RegisterHandler(serverCtx))
	api.POST("/auth/login", authhandler.LoginHandler(serverCtx))
	api.POST("/auth/refresh", middleware.RequireCookieCSRF(jwtauth.RefreshCookieName), authhandler.RefreshHandler(serverCtx))
	api.POST("/auth/logout", middleware.RequireCookieCSRF(jwtauth.RefreshCookieName), authhandler.LogoutHandler(serverCtx))

	protected := api.Group("")
	protected.Use(middleware.RequireAuth(serverCtx))
	protected.Use(middleware.RequireCSRFForCookieAuth())
	protected.GET("/users/me", authhandler.MeHandler(serverCtx))
	protected.PUT("/users/me", middleware.RequireCookieAccess(), middleware.RequireCookieCSRF(jwtauth.RefreshCookieName), authhandler.UpdateMeHandler(serverCtx))
	protected.POST("/documents/text", middleware.LimitRequestBody(documenthandler.MaxTextDocumentBodyBytes), documenthandler.CreateTextHandler(serverCtx))
	protected.POST("/documents/pdf", middleware.LimitRequestBody(documenthandler.MaxPDFRequestBodyBytes(serverCtx)), documenthandler.CreatePDFHandler(serverCtx))
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
	protected.POST("/projects", projecthandler.CreateHandler(serverCtx))
	protected.GET("/projects", projecthandler.ListHandler(serverCtx))
	protected.GET("/projects/:projectId", projecthandler.GetHandler(serverCtx))
	protected.PUT("/projects/:projectId", projecthandler.UpdateHandler(serverCtx))
	protected.POST("/projects/:projectId/archive", projecthandler.ArchiveHandler(serverCtx))
	protected.POST("/projects/:projectId/unarchive", projecthandler.UnarchiveHandler(serverCtx))
	protected.DELETE("/projects/:projectId", projecthandler.DeleteHandler(serverCtx))
	protected.GET("/projects/:projectId/tasks", taskhandler.ListHandler(serverCtx))
	protected.POST("/projects/:projectId/tasks", taskhandler.CreateHandler(serverCtx))
	protected.PUT("/tasks/:taskId", taskhandler.UpdateHandler(serverCtx))
	protected.PATCH("/tasks/:taskId/status", taskhandler.UpdateStatusHandler(serverCtx))
	protected.DELETE("/tasks/:taskId", taskhandler.DeleteHandler(serverCtx))
	protected.POST("/tasks/reorder", taskhandler.ReorderHandler(serverCtx))
	protected.GET("/history/projects", projecthandler.HistoryListHandler(serverCtx))
	protected.GET("/history/projects/:projectId", projecthandler.HistoryGetHandler(serverCtx))
	protected.GET("/history/projects/:projectId/tasks", taskhandler.HistoryListHandler(serverCtx))
	protected.GET("/history/parse-results", parseresulthandler.HistoryListHandler(serverCtx))
}
