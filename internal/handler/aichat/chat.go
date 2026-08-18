package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/744223454/taskpilot-server/internal/handler/common"
	"github.com/744223454/taskpilot-server/internal/handler/middleware"
	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	logicaichat "github.com/744223454/taskpilot-server/internal/logic/aichat"
	"github.com/744223454/taskpilot-server/internal/svc"
	"github.com/744223454/taskpilot-server/internal/types"
	"github.com/744223454/taskpilot-server/pkg/ai"
	bizerrors "github.com/744223454/taskpilot-server/pkg/errors"
	"github.com/gin-gonic/gin"
)

const MaxRequestBodyBytes = 64 << 10

var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func ChatHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := middleware.PrincipalFrom(c)
		if !ok {
			common.WriteError(c, svcCtx.Logger, errors.New("missing authenticated principal"))
			return
		}
		requestID := c.GetHeader("X-Request-ID")
		if !requestIDPattern.MatchString(requestID) {
			common.WriteError(c, svcCtx.Logger, logicerrors.ErrInvalidInput)
			return
		}
		var request types.AIChatRequest
		if err := common.BindJSONStrict(c, &request); err != nil {
			common.WriteBindingError(c, err)
			return
		}

		stream, err := logicaichat.NewService(c.Request.Context(), svcCtx).Stream(principal.UserID, requestID, &request)
		if err != nil {
			var limitError *logicaichat.LimitError
			if errors.As(err, &limitError) && limitError.RetryAfter > 0 {
				seconds := int64((limitError.RetryAfter + time.Second - 1) / time.Second)
				c.Header("Retry-After", strconv.FormatInt(seconds, 10))
			}
			common.WriteError(c, svcCtx.Logger, err)
			return
		}

		c.Header("Content-Type", "text/event-stream; charset=utf-8")
		c.Header("Cache-Control", "no-cache, no-transform")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Header("Content-Encoding", "identity")
		c.Status(http.StatusOK)
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			writeSSE(c, "error", map[string]any{"code": bizerrors.CodeInternalError, "message": "streaming is unsupported"})
			return
		}
		flusher.Flush()

		startedAt := time.Now()
		outputChars := 0
		finishReason := "client_disconnected"
		defer func() {
			if svcCtx.Logger != nil {
				svcCtx.Logger.Info("AI chat finished", "request_id", requestID, "user_id", principal.UserID, "parse_result_id", request.ParseResultID, "duration_ms", time.Since(startedAt).Milliseconds(), "output_chars", outputChars, "finish_reason", finishReason)
			}
		}()

		for event := range stream {
			switch event.Type {
			case "meta":
				writeSSE(c, "meta", map[string]any{"request_id": event.RequestID, "model": event.Model})
			case "delta":
				outputChars += len([]rune(event.Content))
				writeSSE(c, "delta", map[string]any{"content": event.Content})
			case "done":
				finishReason = event.FinishReason
				writeSSE(c, "done", map[string]any{"finish_reason": event.FinishReason})
			case "error":
				if errors.Is(event.Error, context.Canceled) {
					finishReason = "canceled"
					return
				}
				finishReason = "error"
				if svcCtx.Logger != nil {
					svcCtx.Logger.ErrorContext(c.Request.Context(), "AI chat stream failed", "request_id", requestID, "user_id", principal.UserID, "parse_result_id", request.ParseResultID, "error", event.Error)
				}
				writeSSE(c, "error", map[string]any{"code": bizerrors.CodeServiceUnavailable, "message": ai.PublicChatErrorMessage(event.Error)})
			case "ping":
				_, _ = fmt.Fprint(c.Writer, ": ping\n\n")
			}
			flusher.Flush()
		}
	}
}

func writeSSE(c *gin.Context, event string, data any) {
	encoded, err := json.Marshal(data)
	if err != nil {
		encoded = []byte(`{"code":10005,"message":"internal server error"}`)
	}
	_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, encoded)
}
