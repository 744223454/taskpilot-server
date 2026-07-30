package common

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type strictJSONRequest struct {
	Name string `json:"name" binding:"required"`
}

func TestBindJSONStrictRejectsUnknownAndTrailingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"name":"task","status":"done"}`,
		`{"name":"task"}{"name":"other"}`,
	} {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		var req strictJSONRequest
		if err := BindJSONStrict(context, &req); err == nil {
			t.Fatalf("BindJSONStrict(%q) error = nil, want error", body)
		}
	}
}

func TestBindJSONStrictValidatesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"name":""}`))
	var req strictJSONRequest
	if err := BindJSONStrict(context, &req); err == nil {
		t.Fatal("BindJSONStrict() error = nil, want validation error")
	}
}

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
