package parseresult

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	parseresultlogic "github.com/744223454/taskpilot-server/internal/logic/parseresult"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func GetByJobHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		jobID, err := common.PathID(c, "jobId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		result, err := parseresultlogic.NewService(c.Request.Context(), svcCtx).GetByJob(principal.UserID, jobID)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, result)
	}
}

func GetHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resultID, err := common.PathID(c, "resultId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		result, err := parseresultlogic.NewService(c.Request.Context(), svcCtx).Get(principal.UserID, resultID)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, result)
	}
}

func UpdateHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resultID, err := common.PathID(c, "resultId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		var req types.UpdateParseResultRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		result, err := parseresultlogic.NewService(c.Request.Context(), svcCtx).Update(principal.UserID, resultID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, result)
	}
}

func ConfirmHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resultID, err := common.PathID(c, "resultId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		result, err := parseresultlogic.NewService(c.Request.Context(), svcCtx).Confirm(principal.UserID, resultID)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, result)
	}
}

func HistoryListHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.ParseResultHistoryListRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			common.WriteBindingError(c, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		results, err := parseresultlogic.NewService(c.Request.Context(), svcCtx).HistoryList(principal.UserID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, results)
	}
}
