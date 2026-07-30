package task

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	tasklogic "github.com/744223454/taskpilot-server/internal/logic/task"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func ListHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return listHandler(svcCtx, false)
}

func HistoryListHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return listHandler(svcCtx, true)
}

func CreateHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID, err := common.PathID(c, "projectId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		var req types.CreateTaskRequest
		if err := common.BindJSONStrict(c, &req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		task, err := tasklogic.NewService(c.Request.Context(), svcCtx).Create(principal.UserID, projectID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusCreated, task)
	}
}

func UpdateHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID, err := common.PathID(c, "taskId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		var req types.UpdateTaskRequest
		if err := common.BindJSONStrict(c, &req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		task, err := tasklogic.NewService(c.Request.Context(), svcCtx).Update(principal.UserID, taskID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, task)
	}
}

func UpdateStatusHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID, err := common.PathID(c, "taskId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		var req types.UpdateTaskStatusRequest
		if err := common.BindJSONStrict(c, &req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		task, err := tasklogic.NewService(c.Request.Context(), svcCtx).UpdateStatus(principal.UserID, taskID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, task)
	}
}

func DeleteHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		taskID, err := common.PathID(c, "taskId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		if err := tasklogic.NewService(c.Request.Context(), svcCtx).Delete(principal.UserID, taskID); err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, struct{}{})
	}
}

func ReorderHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.ReorderTasksRequest
		if err := common.BindJSONStrict(c, &req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		tasks, err := tasklogic.NewService(c.Request.Context(), svcCtx).Reorder(principal.UserID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, tasks)
	}
}

func listHandler(svcCtx *svc.ServiceContext, history bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectID, err := common.PathID(c, "projectId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		var req types.TaskListRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		service := tasklogic.NewService(c.Request.Context(), svcCtx)
		var tasks *types.TaskListResponse
		if history {
			tasks, err = service.HistoryList(principal.UserID, projectID, &req)
		} else {
			tasks, err = service.List(principal.UserID, projectID, &req)
		}
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, tasks)
	}
}
