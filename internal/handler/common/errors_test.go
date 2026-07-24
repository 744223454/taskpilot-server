package common

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWriteErrorLogsInternalErrorWithoutExposingIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest("POST", "/api/v1/documents/text", nil)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = request

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	WriteError(context, logger, errors.New("database connection reset"))

	if strings.Contains(response.Body.String(), "database connection reset") {
		t.Fatal("internal error leaked to response")
	}
	if !strings.Contains(logs.String(), "database connection reset") {
		t.Fatal("internal error was not written to logs")
	}
}
