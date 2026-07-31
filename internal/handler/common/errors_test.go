package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
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

func TestWriteErrorMapsPDFErrors(t *testing.T) {
	for _, testCase := range []struct {
		err        error
		statusCode int
		code       int
	}{
		{err: logicerrors.ErrUnsupportedFileType, statusCode: http.StatusUnsupportedMediaType, code: bizerrors.CodeUnsupportedFileType},
		{err: logicerrors.ErrPDFUnprocessable, statusCode: http.StatusUnprocessableEntity, code: bizerrors.CodePDFUnprocessable},
		{err: logicerrors.ErrExtractionBusy, statusCode: http.StatusServiceUnavailable, code: bizerrors.CodeServiceUnavailable},
		{err: logicerrors.ErrPayloadTooLarge, statusCode: http.StatusRequestEntityTooLarge, code: bizerrors.CodePayloadTooLarge},
	} {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/documents/pdf", nil)
		WriteError(context, slog.Default(), testCase.err)
		if recorder.Code != testCase.statusCode {
			t.Fatalf("WriteError(%v) status = %d, want %d", testCase.err, recorder.Code, testCase.statusCode)
		}
		var envelope struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if envelope.Code != testCase.code {
			t.Fatalf("WriteError(%v) code = %d, want %d", testCase.err, envelope.Code, testCase.code)
		}
	}
}
