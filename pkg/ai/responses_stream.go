package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type responsesStreamEvent struct {
	Type    string `json:"type"`
	Delta   string `json:"delta"`
	Message string `json:"message"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
	Response *struct {
		Status string `json:"status"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	} `json:"response"`
}

func consumeResponsesStream(ctx context.Context, body io.Reader, onDelta func(string) error) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), maxResponseBytes+64*1024)
	dataLines := make([]string, 0, 1)
	totalBytes := 0

	dispatch := func() (string, bool, error) {
		if len(dataLines) == 0 {
			return "", false, nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if strings.TrimSpace(data) == "[DONE]" {
			return "", false, nil
		}

		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return "", false, fmt.Errorf("%w: decode streaming event", ErrInvalidResponse)
		}
		switch event.Type {
		case "response.output_text.delta", "output_text.delta":
			if event.Delta != "" && onDelta != nil {
				if err := onDelta(event.Delta); err != nil {
					return "", false, err
				}
			}
		case "response.completed", "response.done":
			finishReason := "completed"
			if event.Response != nil && strings.TrimSpace(event.Response.Status) != "" {
				finishReason = event.Response.Status
			}
			return finishReason, true, nil
		case "response.failed", "response.incomplete", "response.error", "error":
			return "", false, fmt.Errorf("%w: %s", ErrInvalidResponse, responsesStreamFailureMessage(event))
		case "response.refusal.delta", "response.refusal.done":
			message := strings.TrimSpace(event.Delta)
			if message == "" {
				message = "AI response was refused"
			}
			return "", false, fmt.Errorf("%w: %s", ErrInvalidResponse, message)
		}
		return "", false, nil
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		line := scanner.Text()
		totalBytes += len(line) + 1
		if totalBytes > maxResponseBytes {
			return "", fmt.Errorf("%w: streaming response exceeds size limit", ErrInvalidResponse)
		}
		if line == "" {
			finishReason, completed, err := dispatch()
			if err != nil {
				return "", err
			}
			if completed {
				return finishReason, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, found := strings.Cut(line, ":")
		if !found || field != "data" {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		dataLines = append(dataLines, value)
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("read AI response stream: %w", err)
	}
	finishReason, completed, err := dispatch()
	if err != nil {
		return "", err
	}
	if completed {
		return finishReason, nil
	}
	return "", fmt.Errorf("%w: streaming response ended before completion", ErrInvalidResponse)
}

func responsesStreamFailureMessage(event responsesStreamEvent) string {
	if event.Response != nil {
		if event.Response.Error != nil {
			if message := strings.TrimSpace(event.Response.Error.Message); message != "" {
				return message
			}
		}
		if event.Response.IncompleteDetails != nil {
			if reason := strings.TrimSpace(event.Response.IncompleteDetails.Reason); reason != "" {
				return reason
			}
		}
	}
	if event.Error != nil {
		if message := strings.TrimSpace(event.Error.Message); message != "" {
			return message
		}
	}
	if message := strings.TrimSpace(event.Message); message != "" {
		return message
	}
	return "AI streaming response failed"
}
