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
		if err := common.BindJSONStrict(c, &req); err != nil {
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

func ListHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.ProjectListRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		projects, err := projectlogic.NewService(c.Request.Context(), svcCtx).List(principal.UserID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, projects)
	}
}

func GetHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return getHandler(svcCtx, false)
}

func HistoryGetHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return getHandler(svcCtx, true)
}

func HistoryListHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.HistoryProjectListRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		projects, err := projectlogic.NewService(c.Request.Context(), svcCtx).HistoryList(principal.UserID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, projects)
	}
}

func UpdateHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID, err := common.PathID(c, "projectId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		var req types.UpdateProjectRequest
		if err := common.BindJSONStrict(c, &req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		project, err := projectlogic.NewService(c.Request.Context(), svcCtx).Update(principal.UserID, projectID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, project)
	}
}

func ArchiveHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return transitionHandler(svcCtx, true)
}

func UnarchiveHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return transitionHandler(svcCtx, false)
}

func DeleteHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID, err := common.PathID(c, "projectId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		if err := projectlogic.NewService(c.Request.Context(), svcCtx).Delete(principal.UserID, projectID); err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, struct{}{})
	}
}

func getHandler(svcCtx *svc.ServiceContext, history bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID, err := common.PathID(c, "projectId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		service := projectlogic.NewService(c.Request.Context(), svcCtx)
		var project *types.ProjectResponse
		if history {
			project, err = service.HistoryGet(principal.UserID, projectID)
		} else {
			project, err = service.Get(principal.UserID, projectID)
		}
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, project)
	}
}

func transitionHandler(svcCtx *svc.ServiceContext, archive bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID, err := common.PathID(c, "projectId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		service := projectlogic.NewService(c.Request.Context(), svcCtx)
		var project *types.ProjectResponse
		if archive {
			project, err = service.Archive(principal.UserID, projectID)
		} else {
			project, err = service.Unarchive(principal.UserID, projectID)
		}
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, project)
	}
}
