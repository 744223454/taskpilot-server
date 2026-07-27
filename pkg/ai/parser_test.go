package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestResponsesParserUsesStrictSchemaAndParsesOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			t.Fatalf("request path = %q, want /responses", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		textConfig := payload["text"].(map[string]any)
		format := textConfig["format"].(map[string]any)
		if format["type"] != "json_schema" || format["strict"] != true {
			t.Fatalf("format = %#v", format)
		}
		writeResponsesOutput(t, writer, validStructuredOutput())
	}))
	defer server.Close()

	parser, err := NewResponsesParser(server.URL, "test-key", "gpt-5.4", time.Second, 8000)
	if err != nil {
		t.Fatalf("NewResponsesParser() error = %v", err)
	}
	parser.now = func() time.Time {
		return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	}
	parsed, err := parser.Parse(context.Background(), "请于 7 月 30 日提交说明书")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.Title != "比赛要求" || parsed.Model != "gpt-5.4" || len(parsed.GeneratedTasks) != 1 {
		t.Fatalf("parsed = %#v", parsed)
	}
	if parsed.Deadline == nil || parsed.Deadline.Format(time.RFC3339) != "2026-07-30T23:59:59+08:00" {
		t.Fatalf("deadline = %v", parsed.Deadline)
	}
}

func TestResponsesParserRetriesInvalidStructuredOutput(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writeResponsesOutput(t, writer, `{}`)
			return
		}
		writeResponsesOutput(t, writer, validStructuredOutput())
	}))
	defer server.Close()

	parser, err := NewResponsesParser(server.URL, "test-key", "gpt-5.4", time.Second, 8000)
	if err != nil {
		t.Fatalf("NewResponsesParser() error = %v", err)
	}
	parser.retryDelay = time.Millisecond
	if _, err := parser.Parse(context.Background(), "document"); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestResponsesParserDoesNotRetryAuthenticationFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	parser, err := NewResponsesParser(server.URL, "test-key", "gpt-5.4", time.Second, 8000)
	if err != nil {
		t.Fatalf("NewResponsesParser() error = %v", err)
	}
	_, err = parser.Parse(context.Background(), "document")
	if err == nil {
		t.Fatal("Parse() error = nil, want authentication error")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	if PublicErrorMessage(err) != "AI service authentication failed" {
		t.Fatalf("PublicErrorMessage() = %q", PublicErrorMessage(err))
	}
}

func TestResponsesParserTimeoutCoversAllAttempts(t *testing.T) {
	var calls atomic.Int32
	parser, err := NewResponsesParser("https://example.com/v1", "test-key", "gpt-5.4", 20*time.Millisecond, 8000)
	if err != nil {
		t.Fatalf("NewResponsesParser() error = %v", err)
	}
	parser.retryDelay = 0
	parser.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}
		<-request.Context().Done()
		return nil, request.Context().Err()
	})

	_, err = parser.Parse(context.Background(), "document")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Parse() error = %v, want deadline exceeded", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func writeResponsesOutput(t *testing.T, writer http.ResponseWriter, output string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"output": []any{map[string]any{
			"type": "message",
			"content": []any{map[string]any{
				"type": "output_text",
				"text": output,
			}},
		}},
	}); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func validStructuredOutput() string {
	return `{
		"title":"比赛要求",
		"summary":"提交比赛材料",
		"deadline":"2026-07-30T23:59:59+08:00",
		"deliverables":["说明书","说明书"],
		"key_requirements":["不超过五人"],
		"risk_warnings":[],
		"generated_tasks":[{
			"title":"完成说明书",
			"description":null,
			"priority":"high",
			"deadline":"2026-07-29T23:59:59+08:00"
		}]
	}`
}
