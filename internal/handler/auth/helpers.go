package auth

import (
	"errors"
	"net/http"
	"strings"

	authlogic "github.com/744223454/taskpilot-server/internal/logic/auth"
	jwtauth "github.com/744223454/taskpilot-server/pkg/auth"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
)

var errMissingBearerToken = errors.New("missing bearer token")

func writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errMissingBearerToken):
		response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, err.Error())
	case errors.Is(err, authlogic.ErrDatabaseNotConnected):
		response.Error(c, http.StatusServiceUnavailable, bizerrors.CodeDatabaseUnavailable, err.Error())
	case errors.Is(err, authlogic.ErrEmailRegistered):
		response.Error(c, http.StatusConflict, bizerrors.CodeEmailRegistered, err.Error())
	case errors.Is(err, authlogic.ErrInvalidCredentials),
		errors.Is(err, authlogic.ErrInvalidAccessToken),
		errors.Is(err, jwtauth.ErrInvalidToken),
		errors.Is(err, jwtauth.ErrExpiredToken):
		response.Error(c, http.StatusUnauthorized, bizerrors.CodeUnauthorized, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, bizerrors.CodeInternalError, "internal server error")
	}
}

func bearerToken(header string) (string, error) {
	fields := strings.Fields(header)
	if len(fields) != 2 {
		return "", errMissingBearerToken
	}
	if !strings.EqualFold(fields[0], "Bearer") {
		return "", errMissingBearerToken
	}
	return fields[1], nil
}
