package auth

import (
	"errors"
	"net/http"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	"github.com/744223454/taskpilot-server/internal/svc"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

func writeAuthError(c *gin.Context, svcCtx *svc.ServiceContext, err error) {
	switch {
	case errors.Is(err, authlogic.ErrEmailRegistered):
		response.Error(c, http.StatusConflict, bizerrors.CodeEmailRegistered, err.Error())
	case errors.Is(err, authlogic.ErrInvalidCredentials), errors.Is(err, authlogic.ErrInvalidAccessToken):
		response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, err.Error())
	default:
		common.WriteError(c, svcCtx.Logger, err)
	}
}
