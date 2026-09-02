package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestConsumeResponsesStreamAcceptsDataWithoutSpace(t *testing.T) {
	stream := strings.NewReader("data:{\"type\":\"response.output_text.delta\",\"delta\":\"答案\"}\n\n" +
		"data:{\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	var output strings.Builder
	finishReason, err := consumeResponsesStream(context.Background(), stream, func(delta string) error {
		output.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatalf("consumeResponsesStream() error = %v", err)
	}
	if output.String() != "答案" || finishReason != "completed" {
		t.Fatalf("output = %q, finishReason = %q", output.String(), finishReason)
	}
}

func TestConsumeResponsesStreamRejectsMalformedEvent(t *testing.T) {
	_, err := consumeResponsesStream(context.Background(), strings.NewReader("data: not-json\n\n"), nil)
	if err == nil || !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("consumeResponsesStream() error = %v, want invalid response", err)
	}
}

func TestConsumeResponsesStreamReturnsFailureMessage(t *testing.T) {
	stream := strings.NewReader("data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"upstream failed\"}}}\n\n")
	_, err := consumeResponsesStream(context.Background(), stream, nil)
	if err == nil || !errors.Is(err, ErrInvalidResponse) || !strings.Contains(err.Error(), "upstream failed") {
		t.Fatalf("consumeResponsesStream() error = %v, want upstream failure", err)
	}
}

func TestConsumeResponsesStreamReturnsIncompleteReason(t *testing.T) {
	stream := strings.NewReader("data: {\"type\":\"response.incomplete\",\"response\":{\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n")
	_, err := consumeResponsesStream(context.Background(), stream, nil)
	if err == nil || !errors.Is(err, ErrInvalidResponse) || !strings.Contains(err.Error(), "max_output_tokens") {
		t.Fatalf("consumeResponsesStream() error = %v, want incomplete reason", err)
	}
}

func TestConsumeResponsesStreamRequiresCompletion(t *testing.T) {
	stream := strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
	_, err := consumeResponsesStream(context.Background(), stream, nil)
	if err == nil || !errors.Is(err, ErrInvalidResponse) || !strings.Contains(err.Error(), "before completion") {
		t.Fatalf("consumeResponsesStream() error = %v, want incomplete stream", err)
	}
}

func TestConsumeResponsesStreamLimitsResponseSize(t *testing.T) {
	stream := strings.NewReader("data: " + strings.Repeat("x", maxResponseBytes) + "\n\n")
	_, err := consumeResponsesStream(context.Background(), stream, nil)
	if err == nil || !errors.Is(err, ErrInvalidResponse) || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("consumeResponsesStream() error = %v, want size limit error", err)
	}
}

func TestConsumeResponsesStreamReturnsCallbackError(t *testing.T) {
	wantErr := errors.New("write failed")
	stream := strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\n")
	_, err := consumeResponsesStream(context.Background(), stream, func(string) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("consumeResponsesStream() error = %v, want %v", err, wantErr)
	}
}
