package llamacpp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GenerationOptions map[string]any

type GenerateRequest struct {
	Model   string            `json:"model"`
	Prompt  string            `json:"prompt"`
	Stream  bool              `json:"stream"`
	Options GenerationOptions `json:"options,omitempty"`
}

type GenerateResponse struct {
	Model    string `json:"model,omitempty"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string            `json:"model"`
	Messages []ChatMessage     `json:"messages"`
	Stream   bool              `json:"stream"`
	Options  GenerationOptions `json:"options,omitempty"`
}

type ChatResponse struct {
	Model   string      `json:"model,omitempty"`
	Message ChatMessage `json:"message"`
	Done    bool        `json:"done"`
}

type StreamChunk struct {
	Model    string      `json:"model,omitempty"`
	Response string      `json:"response,omitempty"`
	Message  ChatMessage `json:"message,omitempty"`
	Done     bool        `json:"done"`
	Error    string      `json:"error,omitempty"`
}

type LlamaClient interface {
	Generate(ctx context.Context, endpoint string, req GenerateRequest) (*GenerateResponse, error)
	GenerateStream(ctx context.Context, endpoint string, req GenerateRequest) (<-chan StreamChunk, error)
	Chat(ctx context.Context, endpoint string, req ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, endpoint string, req ChatRequest) (<-chan StreamChunk, error)
}

type HTTPClient struct {
	Client *http.Client
}

func NewHTTPClient() HTTPClient {
	return HTTPClient{Client: http.DefaultClient}
}

func (c HTTPClient) Generate(ctx context.Context, endpoint string, req GenerateRequest) (*GenerateResponse, error) {
	req.Stream = false
	var payload map[string]any
	if req.Options != nil {
		payload = map[string]any{}
		for key, value := range req.Options {
			payload[key] = value
		}
	} else {
		payload = map[string]any{}
	}
	payload["prompt"] = req.Prompt
	payload["stream"] = false
	data, err := c.postJSON(ctx, endpoint+"/completion", payload)
	if err != nil {
		return nil, err
	}
	text := extractCompletion(data)
	return &GenerateResponse{Model: req.Model, Response: text, Done: true}, nil
}

func (c HTTPClient) GenerateStream(ctx context.Context, endpoint string, req GenerateRequest) (<-chan StreamChunk, error) {
	req.Stream = true
	payload := map[string]any{}
	for key, value := range req.Options {
		payload[key] = value
	}
	payload["prompt"] = req.Prompt
	payload["stream"] = true
	resp, err := c.postJSONResponse(ctx, endpoint+"/completion", payload)
	if err != nil {
		return nil, err
	}
	return streamChunks(resp.Body, req.Model, false), nil
}

func (c HTTPClient) Chat(ctx context.Context, endpoint string, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	payload := map[string]any{}
	for key, value := range req.Options {
		payload[key] = value
	}
	payload["model"] = req.Model
	payload["messages"] = req.Messages
	payload["stream"] = false
	data, err := c.postJSON(ctx, endpoint+"/v1/chat/completions", payload)
	if err != nil {
		return nil, err
	}
	return &ChatResponse{
		Model:   req.Model,
		Message: ChatMessage{Role: "assistant", Content: extractChatContent(data)},
		Done:    true,
	}, nil
}

func (c HTTPClient) ChatStream(ctx context.Context, endpoint string, req ChatRequest) (<-chan StreamChunk, error) {
	req.Stream = true
	payload := map[string]any{}
	for key, value := range req.Options {
		payload[key] = value
	}
	payload["model"] = req.Model
	payload["messages"] = req.Messages
	payload["stream"] = true
	resp, err := c.postJSONResponse(ctx, endpoint+"/v1/chat/completions", payload)
	if err != nil {
		return nil, err
	}
	return streamChunks(resp.Body, req.Model, true), nil
}

func (c HTTPClient) postJSON(ctx context.Context, url string, payload any) (map[string]any, error) {
	resp, err := c.postJSONResponse(ctx, url, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, ActionableError{
			What:    "llama.cpp response could not be decoded.",
			Reason:  err.Error(),
			Fix:     "Check that the llama.cpp server endpoint is compatible with VinoLlama.",
			Details: fmt.Sprintf("url=%s", url),
		}
	}
	return data, nil
}

func (c HTTPClient) postJSONResponse(ctx context.Context, url string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, ActionableError{
			What:    "llama.cpp request failed.",
			Reason:  err.Error(),
			Fix:     "Check that the llama.cpp server process is running and healthy.",
			Details: fmt.Sprintf("url=%s", url),
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		_ = resp.Body.Close()
		return nil, ActionableError{
			What:    "llama.cpp returned an error.",
			Reason:  fmt.Sprintf("HTTP %d", resp.StatusCode),
			Fix:     "Inspect the llama.cpp runtime log and request adapter compatibility.",
			Details: fmt.Sprintf("url=%s body=%q", url, string(data)),
		}
	}
	return resp, nil
}

func streamChunks(body io.ReadCloser, model string, chat bool) <-chan StreamChunk {
	ch := make(chan StreamChunk)
	go func() {
		defer close(ch)
		defer body.Close()
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			line = strings.TrimPrefix(line, "data:")
			line = strings.TrimSpace(line)
			if line == "[DONE]" {
				ch <- StreamChunk{Model: model, Done: true}
				return
			}
			var data map[string]any
			if err := json.Unmarshal([]byte(line), &data); err != nil {
				ch <- StreamChunk{Model: model, Error: err.Error(), Done: true}
				return
			}
			done := extractDone(data)
			if chat {
				ch <- StreamChunk{Model: model, Message: ChatMessage{Role: "assistant", Content: extractChatContent(data)}, Done: done}
			} else {
				ch <- StreamChunk{Model: model, Response: extractCompletion(data), Done: done}
			}
			if done {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Model: model, Error: err.Error(), Done: true}
			return
		}
		ch <- StreamChunk{Model: model, Done: true}
	}()
	return ch
}

func extractCompletion(data map[string]any) string {
	for _, key := range []string{"content", "response", "text"} {
		if value, ok := data[key].(string); ok {
			return value
		}
	}
	if choices, ok := data["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if text, ok := choice["text"].(string); ok {
				return text
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				if text, ok := delta["content"].(string); ok {
					return text
				}
			}
		}
	}
	return ""
}

func extractChatContent(data map[string]any) string {
	if content, ok := data["content"].(string); ok {
		return content
	}
	if choices, ok := data["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if message, ok := choice["message"].(map[string]any); ok {
				if content, ok := message["content"].(string); ok {
					return content
				}
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				if content, ok := delta["content"].(string); ok {
					return content
				}
			}
		}
	}
	return extractCompletion(data)
}

func extractDone(data map[string]any) bool {
	for _, key := range []string{"done", "stop"} {
		if value, ok := data[key].(bool); ok {
			return value
		}
	}
	return false
}
