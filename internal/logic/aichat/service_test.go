package aichat

import (
	"errors"
	"strings"
	"testing"

	logicerrors "github.com/744223454/taskpilot-server/internal/logic"
	"github.com/744223454/taskpilot-server/internal/types"
)

func TestNormalizeMessagesRejectsBrokenSequence(t *testing.T) {
	_, err := normalizeMessages([]types.AIChatMessage{
		{Role: "user", Content: "first"},
		{Role: "user", Content: "second"},
	})
	if !errors.Is(err, logicerrors.ErrInvalidInput) {
		t.Fatalf("normalizeMessages() error = %v", err)
	}
}

func TestNormalizeMessagesTrimsOldestRounds(t *testing.T) {
	long := strings.Repeat("a", 4000)
	messages, err := normalizeMessages([]types.AIChatMessage{
		{Role: "user", Content: long},
		{Role: "assistant", Content: long},
		{Role: "user", Content: long},
		{Role: "assistant", Content: "answer"},
		{Role: "user", Content: "latest"},
	})
	if err != nil {
		t.Fatalf("normalizeMessages() error = %v", err)
	}
	if len(messages) != 3 || messages[0].Content != long || messages[2].Content != "latest" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestTruncateRunesPreservesBothEnds(t *testing.T) {
	result := truncateRunes("1234567890", 6)
	if !strings.HasPrefix(result, "123") || !strings.HasSuffix(result, "890") {
		t.Fatalf("truncateRunes() = %q", result)
	}
}
