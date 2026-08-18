package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Instructions string
	Messages     []ChatMessage
}

type ChatStreamer interface {
	Model() string
	Stream(ctx context.Context, request ChatRequest, onDelta func(string) error) (string, error)
}

type ResponsesChatClient struct {
	baseURL         string
	apiKey          string
	model           string
	requestTimeout  time.Duration
	maxOutputTokens int64
	client          *http.Client
}

func NewResponsesChatClient(baseURL, apiKey, model string, requestTimeout time.Duration, maxOutputTokens int64) (*ResponsesChatClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid AI base URL: %w", err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("AI API key is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("AI model is required")
	}
	if requestTimeout <= 0 || maxOutputTokens <= 0 {
		return nil, errors.New("AI chat limits must be positive")
	}
	return &ResponsesChatClient{
		baseURL:         baseURL,
		apiKey:          apiKey,
		model:           strings.TrimSpace(model),
		requestTimeout:  requestTimeout,
		maxOutputTokens: maxOutputTokens,
		client:          &http.Client{},
	}, nil
}

func (client *ResponsesChatClient) Model() string {
	if client == nil {
		return ""
	}
	return client.model
}

func (client *ResponsesChatClient) Stream(ctx context.Context, chatRequest ChatRequest, onDelta func(string) error) (string, error) {
	streamContext, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()

	payload := struct {
		Model           string        `json:"model"`
		Instructions    string        `json:"instructions"`
		Input           []ChatMessage `json:"input"`
		MaxOutputTokens int64         `json:"max_output_tokens"`
		Stream          bool          `json:"stream"`
	}{
		Model:           client.model,
		Instructions:    chatRequest.Instructions,
		Input:           chatRequest.Messages,
		MaxOutputTokens: client.maxOutputTokens,
		Stream:          true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode AI chat request: %w", err)
	}

	request, err := http.NewRequestWithContext(streamContext, http.MethodPost, client.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create AI chat request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")

	response, err := client.client.Do(request)
	if err != nil {
		if streamContext.Err() != nil {
			return "", streamContext.Err()
		}
		return "", fmt.Errorf("call AI chat API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return "", &ResponseError{StatusCode: response.StatusCode}
	}

	return consumeResponsesStream(streamContext, response.Body, onDelta)
}

func consumeResponsesStream(ctx context.Context, body io.Reader, onDelta func(string) error) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), maxResponseBytes)
	finishReason := "stop"
	completed := false
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Message  string `json:"message"`
			Response *struct {
				Status string `json:"status"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return "", fmt.Errorf("%w: decode streaming event", ErrInvalidResponse)
		}
		switch event.Type {
		case "response.output_text.delta", "output_text.delta":
			if event.Delta != "" && onDelta != nil {
				if err := onDelta(event.Delta); err != nil {
					return "", err
				}
			}
		case "response.completed", "response.done":
			completed = true
			if event.Response != nil && event.Response.Status != "" {
				finishReason = event.Response.Status
			}
		case "response.failed", "response.incomplete", "error":
			message := event.Message
			if event.Response != nil && event.Response.Error != nil && event.Response.Error.Message != "" {
				message = event.Response.Error.Message
			}
			if message == "" {
				message = "AI streaming response failed"
			}
			return "", fmt.Errorf("%w: %s", ErrInvalidResponse, message)
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("read AI chat stream: %w", err)
	}
	if !completed {
		return "", fmt.Errorf("%w: streaming response ended before completion", ErrInvalidResponse)
	}
	return finishReason, nil
}

func PublicChatErrorMessage(err error) string {
	var responseErr *ResponseError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "AI 助手响应超时"
	case errors.Is(err, ErrInvalidResponse):
		return "AI 助手返回了无效响应"
	case errors.As(err, &responseErr):
		if responseErr.StatusCode == http.StatusTooManyRequests || responseErr.StatusCode >= http.StatusInternalServerError {
			return "AI 助手暂时不可用"
		}
		return "AI 助手拒绝了本次请求"
	default:
		return "AI 助手请求失败"
	}
}
