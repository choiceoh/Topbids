package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/choiceoh/topbid/backend/internal/config"
)

// Client talks to a local vLLM (OpenAI-compatible) server.
type Client struct {
	baseURL    string
	model      string // explicit override via AI_MODEL env; empty = auto-detect
	httpClient *http.Client

	mu            sync.Mutex
	cachedModel   string
	cachedModelAt time.Time
}

// modelCacheTTL controls how long the auto-detected model name is cached.
const modelCacheTTL = 30 * time.Second

// healthTimeout is the timeout for the lightweight health probe.
const healthTimeout = 3 * time.Second

func NewClient(cfg config.AIConfig) *Client {
	return &Client{
		baseURL: cfg.BaseURL,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// NewClientWith creates a Client with explicit base URL, model, and HTTP client.
// Useful for testing with a mock vLLM server.
func NewClientWith(baseURL, model string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    baseURL,
		model:      model,
		httpClient: httpClient,
	}
}

// modelsResponse is the response from /v1/models.
type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// resolveModel returns the model to use. If AI_MODEL is set, use that.
// Otherwise query the vLLM /v1/models endpoint and cache the result.
func (c *Client) resolveModel(ctx context.Context) (string, error) {
	if c.model != "" {
		return c.model, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cachedModel != "" && time.Since(c.cachedModelAt) < modelCacheTTL {
		return c.cachedModel, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return "", fmt.Errorf("create models request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.cachedModel != "" {
			slog.Warn("failed to refresh model list, using cached model", "model", c.cachedModel, "error", err)
			return c.cachedModel, nil
		}
		return "", fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		if c.cachedModel != "" {
			slog.Warn("failed to refresh model list, using cached model", "model", c.cachedModel, "status", resp.StatusCode)
			return c.cachedModel, nil
		}
		return "", fmt.Errorf("models endpoint returned %d: %s", resp.StatusCode, string(b))
	}

	var models modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return "", fmt.Errorf("decode models response: %w", err)
	}
	if len(models.Data) == 0 {
		return "", fmt.Errorf("no models available on vLLM server")
	}

	detected := models.Data[0].ID
	if c.cachedModel != detected {
		slog.Info("AI model detected", "model", detected)
	}
	c.cachedModel = detected
	c.cachedModelAt = time.Now()
	return detected, nil
}

// Healthy returns true when the vLLM server is reachable and serving at least one model.
func (c *Client) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var models modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return false
	}
	return len(models.Data) > 0
}

// chatRequest is the OpenAI-compatible chat completion request.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Tools       []Tool        `json:"tools,omitempty"`
}

// contentPart is a single part of a multimodal message (OpenAI vision format).
type contentPart struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for type=text
	ImageURL *imageURL `json:"image_url,omitempty"` // for type=image_url
}

type imageURL struct {
	URL string `json:"url"`
}

// chatMessage supports both plain text and multimodal content.
// When Images is non-empty, Content is serialized as an array of content parts.
type chatMessage struct {
	Role       string     `json:"-"`
	Content    string     `json:"-"`
	Images     []string   `json:"-"` // image URLs (data: or http:)
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

func (m chatMessage) MarshalJSON() ([]byte, error) {
	type alias struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
	}

	a := alias{
		Role:       m.Role,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
	}

	if len(m.Images) > 0 {
		parts := []contentPart{{Type: "text", Text: m.Content}}
		for _, img := range m.Images {
			parts = append(parts, contentPart{
				Type:     "image_url",
				ImageURL: &imageURL{URL: img},
			})
		}
		b, err := json.Marshal(parts)
		if err != nil {
			return nil, err
		}
		a.Content = b
	} else {
		b, err := json.Marshal(m.Content)
		if err != nil {
			return nil, err
		}
		a.Content = b
	}

	return json.Marshal(a)
}

type chatResponse struct {
	Choices []struct {
		Message      chatResponseMessage `json:"message"`
		FinishReason string              `json:"finish_reason"`
	} `json:"choices"`
}

type chatResponseMessage struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Tool describes an OpenAI-compatible function tool.
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction is the function definition inside a Tool.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is a tool invocation returned by the model.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ToolResolver is called for each tool invocation. It receives the function
// name and raw JSON arguments and returns the result string to feed back.
type ToolResolver func(name string, arguments string) (string, error)

// Complete sends a chat completion request and returns the assistant message.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	model, err := c.resolveModel(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve model: %w", err)
	}

	body := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.3,
		MaxTokens:   4096,
	}

	return c.doRequest(ctx, body)
}

// ChatMessage is the exported type for multi-turn conversation history.
type ChatMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"` // image URLs for multimodal messages
}

// CompleteChat sends a multi-turn chat request with history and returns the assistant reply.
func (c *Client) CompleteChat(ctx context.Context, system string, history []ChatMessage, userMsg string) (string, error) {
	model, err := c.resolveModel(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve model: %w", err)
	}

	msgs := []chatMessage{
		{Role: "system", Content: system},
	}
	for _, h := range history {
		msgs = append(msgs, chatMessage{Role: h.Role, Content: h.Content, Images: h.Images})
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: userMsg})

	body := chatRequest{
		Model:       model,
		Messages:    msgs,
		Temperature: 0.5,
		MaxTokens:   2048,
	}
	return c.doRequest(ctx, body)
}

// CompleteWithTools sends a chat request with tool definitions and automatically
// resolves tool calls using the provided resolver (max 3 rounds).
func (c *Client) CompleteWithTools(ctx context.Context, system string, history []ChatMessage, userMsg string, tools []Tool, resolve ToolResolver, images ...string) (string, error) {
	model, err := c.resolveModel(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve model: %w", err)
	}

	msgs := []chatMessage{
		{Role: "system", Content: system},
	}
	for _, h := range history {
		msgs = append(msgs, chatMessage{Role: h.Role, Content: h.Content, Images: h.Images})
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: userMsg, Images: images})

	const maxRounds = 3
	for range maxRounds {
		body := chatRequest{
			Model:       model,
			Messages:    msgs,
			Temperature: 0.5,
			MaxTokens:   2048,
			Tools:       tools,
		}

		resp, err := c.doRequestFull(ctx, body)
		if err != nil {
			return "", err
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("ai returned no choices")
		}
		choice := resp.Choices[0]

		// No tool calls — return text.
		if len(choice.Message.ToolCalls) == 0 || choice.FinishReason != "tool_calls" {
			return choice.Message.Content, nil
		}

		// Append assistant message with tool calls.
		msgs = append(msgs, chatMessage{
			Role:      "assistant",
			ToolCalls: choice.Message.ToolCalls,
		})

		// Resolve each tool call and append results.
		for _, tc := range choice.Message.ToolCalls {
			result, err := resolve(tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				result = fmt.Sprintf("error: %s", err.Error())
			}
			msgs = append(msgs, chatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("tool call loop exceeded %d rounds", maxRounds)
}

// doRequestFull is like doRequest but returns the full parsed response.
func (c *Client) doRequestFull(ctx context.Context, body chatRequest) (*chatResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ai server returned %d: %s", resp.StatusCode, string(b))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// doRequest is the shared HTTP call logic.
func (c *Client) doRequest(ctx context.Context, body chatRequest) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai server returned %d: %s", resp.StatusCode, string(b))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("ai returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}
