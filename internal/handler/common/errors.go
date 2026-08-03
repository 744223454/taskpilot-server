package common

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/744223454/taskpilot-server/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

func WriteError(c *gin.Context, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, logicerrors.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, bizerrors.CodeBadRequest, err.Error())
	case errors.Is(err, logicerrors.ErrNotFound):
		response.Error(c, http.StatusNotFound, bizerrors.CodeNotFound, err.Error())
	case errors.Is(err, logicerrors.ErrConflict):
		response.Error(c, http.StatusConflict, bizerrors.CodeConflict, err.Error())
	case errors.Is(err, logicerrors.ErrInvalidState):
		response.Error(c, http.StatusUnprocessableEntity, bizerrors.CodeInvalidState, err.Error())
	case errors.Is(err, logicerrors.ErrPayloadTooLarge):
		response.Error(c, http.StatusRequestEntityTooLarge, bizerrors.CodePayloadTooLarge, err.Error())
	case errors.Is(err, logicerrors.ErrUnsupportedFileType):
		response.Error(c, http.StatusUnsupportedMediaType, bizerrors.CodeUnsupportedFileType, err.Error())
	case errors.Is(err, logicerrors.ErrPDFUnprocessable):
		response.Error(c, http.StatusUnprocessableEntity, bizerrors.CodePDFUnprocessable, err.Error())
	case errors.Is(err, logicerrors.ErrExtractionBusy):
		response.Error(c, http.StatusServiceUnavailable, bizerrors.CodeServiceUnavailable, err.Error())
	case errors.Is(err, logicerrors.ErrRateLimited):
		response.Error(c, http.StatusTooManyRequests, bizerrors.CodeTooManyRequests, err.Error())
	case errors.Is(err, logicerrors.ErrDatabaseUnavailable):
		response.Error(c, http.StatusServiceUnavailable, bizerrors.CodeDatabaseUnavailable, err.Error())
	case errors.Is(err, logicerrors.ErrCacheUnavailable):
		logUnexpectedError(c, logger, err)
		response.Error(c, http.StatusServiceUnavailable, bizerrors.CodeServiceUnavailable, logicerrors.ErrCacheUnavailable.Error())
	default:
		logUnexpectedError(c, logger, err)
		response.Error(c, http.StatusInternalServerError, bizerrors.CodeInternalError, "internal server error")
	}
}

func WriteBindingError(c *gin.Context, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		response.Error(c, http.StatusRequestEntityTooLarge, bizerrors.CodePayloadTooLarge, "request body too large")
		return
	}
	response.Error(c, http.StatusBadRequest, bizerrors.CodeBadRequest, err.Error())
}

func BindJSONStrict(c *gin.Context, value any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain a single JSON object")
		}
		return err
	}
	return binding.Validator.ValidateStruct(value)
}

func PathID(c *gin.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, logicerrors.ErrInvalidInput
	}
	return id, nil
}

func logUnexpectedError(c *gin.Context, logger *slog.Logger, err error) {
	if logger == nil {
		logger = slog.Default()
	}

	args := []any{
		"error", err,
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"route", c.FullPath(),
	}
	if principal, ok := middleware.PrincipalFrom(c); ok {
		args = append(args, "user_id", principal.UserID)
	}
	logger.ErrorContext(c.Request.Context(), "request failed", args...)
}
