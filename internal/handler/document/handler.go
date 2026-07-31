package document

import (
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	documentlogic "github.com/744223454/taskpilot-server/internal/logic/document"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

const MaxTextDocumentBodyBytes int64 = 256 << 10

const multipartOverheadAllowance int64 = 1 << 20

func CreateTextHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.CreateTextDocumentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			common.WriteBindingError(c, err)
			return
		}

		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}

		document, err := documentlogic.NewService(c.Request.Context(), svcCtx).CreateText(principal.UserID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}

		response.Success(c, http.StatusCreated, document)
	}
}

func CreatePDFHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		fileHeader, err := c.FormFile("file")
		if err != nil {
			common.WriteBindingError(c, err)
			return
		}
		file, err := fileHeader.Open()
		if err != nil {
			common.WriteBindingError(c, err)
			return
		}
		defer file.Close()

		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}
		req := types.CreatePDFDocumentRequest{
			Title:    c.PostForm("title"),
			FileName: fileHeader.Filename,
		}
		document, err := documentlogic.NewService(c.Request.Context(), svcCtx).CreatePDF(principal.UserID, &req, file)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusCreated, document)
	}
}

func MaxPDFRequestBodyBytes(svcCtx *svc.ServiceContext) int64 {
	maxFileBytes := svcCtx.Config.Upload.MaxFileBytes
	if maxFileBytes == 0 {
		maxFileBytes = 10 << 20
	}
	return maxFileBytes + multipartOverheadAllowance
}

func GetHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
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

		document, err := documentlogic.NewService(c.Request.Context(), svcCtx).Get(principal.UserID, documentID)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, document)
	}
}

func ListHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.DocumentListRequest
		if err := c.ShouldBindQuery(&req); err != nil {
			response.Error(c, http.StatusBadRequest, bizerrors.CodeBadRequest, err.Error())
			return
		}
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, "invalid access token context")
			return
		}

		documents, err := documentlogic.NewService(c.Request.Context(), svcCtx).List(principal.UserID, &req)
		if err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, documents)
	}
}

func DeleteHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
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

		if err := documentlogic.NewService(c.Request.Context(), svcCtx).Delete(principal.UserID, documentID); err != nil {
			common.WriteError(c, svcCtx.Logger, err)
			return
		}
		response.Success(c, http.StatusOK, struct{}{})
	}
}
