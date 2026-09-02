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
		if payload["stream"] != true {
			t.Fatalf("stream = %#v, want true", payload["stream"])
		}
		textConfig := payload["text"].(map[string]any)
		format := textConfig["format"].(map[string]any)
		if format["type"] != "json_schema" || format["strict"] != true {
			t.Fatalf("format = %#v", format)
		}
		// input 必须是消息数组：网关（如 SoruxGPT）不接受字符串形式。
		input, ok := payload["input"].([]any)
		if !ok || len(input) != 1 {
			t.Fatalf("input = %#v, want a single-element list", payload["input"])
		}
		message, ok := input[0].(map[string]any)
		if !ok || message["role"] != "user" {
			t.Fatalf("input[0] = %#v, want a user message", input[0])
		}
		content, ok := message["content"].([]any)
		if !ok || len(content) != 1 {
			t.Fatalf("input[0].content = %#v, want a single-element list", message["content"])
		}
		part, ok := content[0].(map[string]any)
		if !ok || part["type"] != "input_text" {
			t.Fatalf("input[0].content[0] = %#v, want an input_text part", content[0])
		}
		if !strings.Contains(part["text"].(string), "请于 7 月 30 日提交说明书") {
			t.Fatalf("input text = %#v, want it to contain the document", part["text"])
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

func TestResponsesParserAssemblesMultipleOutputDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeResponsesOutputChunks(t, writer, validStructuredOutput()[:40], validStructuredOutput()[40:])
	}))
	defer server.Close()

	parser, err := NewResponsesParser(server.URL, "test-key", "gpt-5.4", time.Second, 8000)
	if err != nil {
		t.Fatalf("NewResponsesParser() error = %v", err)
	}
	parsed, err := parser.Parse(context.Background(), "document")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.Title != "比赛要求" {
		t.Fatalf("parsed.Title = %q, want 比赛要求", parsed.Title)
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
	writeResponsesOutputChunks(t, writer, output)
}

func writeResponsesOutputChunks(t *testing.T, writer http.ResponseWriter, chunks ...string) {
	t.Helper()
	writer.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := writer.(http.Flusher)
	writeSSEEvent := func(event map[string]any) {
		data, _ := json.Marshal(event)
		writer.Write([]byte("data: "))
		writer.Write(data)
		writer.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	for _, chunk := range chunks {
		writeSSEEvent(map[string]any{"type": "response.output_text.delta", "delta": chunk})
	}
	writeSSEEvent(map[string]any{"type": "response.completed"})
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
