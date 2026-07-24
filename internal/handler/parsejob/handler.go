package parsejob

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	parsejoblogic "github.com/744223454/taskpilot-server/internal/logic/parsejob"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func CreateHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.CreateParseJobRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, bizerrors.CodeBadRequest, err.Error())
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}

		job, err := parsejoblogic.NewService(c.Request.Context(), svcCtx).Create(principal.UserID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusCreated, job)
	}
}

func GetHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
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

		job, err := parsejoblogic.NewService(c.Request.Context(), svcCtx).Get(principal.UserID, jobID)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, job)
	}
}

func LatestHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		documentID, err := common.PathID(c, "documentId")
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}

		job, err := parsejoblogic.NewService(c.Request.Context(), svcCtx).Latest(principal.UserID, documentID)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, job)
	}
}

func RetryHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
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

		job, err := parsejoblogic.NewService(c.Request.Context(), svcCtx).Retry(principal.UserID, jobID)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, job)
	}
}
