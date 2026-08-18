package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResponsesChatClientStreamsDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", request.URL.Path)
		}
		var payload struct {
			Stream bool          `json:"stream"`
			Input  []ChatMessage `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !payload.Stream || len(payload.Input) != 1 || payload.Input[0].Content != "问题" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"答案\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"内容\"}\n\n")
		_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer server.Close()

	client, err := NewResponsesChatClient(server.URL, "test-key", "test-model", time.Second, 2000)
	if err != nil {
		t.Fatalf("NewResponsesChatClient() error = %v", err)
	}
	var output strings.Builder
	finishReason, err := client.Stream(context.Background(), ChatRequest{
		Instructions: "规则",
		Messages:     []ChatMessage{{Role: "user", Content: "问题"}},
	}, func(delta string) error {
		output.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if output.String() != "答案内容" || finishReason != "completed" {
		t.Fatalf("output = %q, finishReason = %q", output.String(), finishReason)
	}
}

func TestResponsesChatClientRejectsInvalidEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(writer, "data: not-json\n\n")
	}))
	defer server.Close()

	client, err := NewResponsesChatClient(server.URL, "test-key", "test-model", time.Second, 2000)
	if err != nil {
		t.Fatalf("NewResponsesChatClient() error = %v", err)
	}
	_, err = client.Stream(context.Background(), ChatRequest{}, nil)
	if err == nil || !strings.Contains(err.Error(), ErrInvalidResponse.Error()) {
		t.Fatalf("Stream() error = %v, want invalid response", err)
	}
}
