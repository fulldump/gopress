package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.deepseek.com/v1"
	defaultModel   = "deepseek-chat"
	promptMessage  = "dado el contenido que te muestro, contestame true o false si el contenido legítimo y publicable (false) o por el contrario es spam, contenido ilegítimo o ilegal (true)."
)

// Config describes how to configure the DeepSeek client.
type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// Client implements a simple DeepSeek API client that can be used as a
// content moderator.
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewClient creates a new DeepSeek client with the given configuration.
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("deepseek: api key is required")
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	model := cfg.Model
	if model == "" {
		model = defaultModel
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		apiKey:     cfg.APIKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		httpClient: httpClient,
	}, nil
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Evaluate sends the provided content to DeepSeek asking whether it should be
// banned. It returns true when the article must be banned.
func (c *Client) Evaluate(ctx context.Context, content string) (bool, error) {
	if c == nil {
		return false, errors.New("deepseek: nil client")
	}

	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{
				Role:    "user",
				Content: fmt.Sprintf("%s\n\nContenido:\n%s", promptMessage, content),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("deepseek: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("deepseek: create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("deepseek: do request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
		return false, fmt.Errorf("deepseek: unexpected status %d: %s", res.StatusCode, string(data))
	}

	var response chatResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return false, fmt.Errorf("deepseek: decode response: %w", err)
	}

	if len(response.Choices) == 0 {
		return false, errors.New("deepseek: empty response choices")
	}

	answer := strings.TrimSpace(response.Choices[0].Message.Content)
	result, err := parseAnswer(answer)
	if err != nil {
		return false, err
	}

	return result, nil
}

func parseAnswer(answer string) (bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(answer))
	switch normalized {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}

	hasTrue := strings.Contains(normalized, "true")
	hasFalse := strings.Contains(normalized, "false")

	switch {
	case hasTrue && !hasFalse:
		return true, nil
	case hasFalse && !hasTrue:
		return false, nil
	default:
		return false, fmt.Errorf("deepseek: unexpected answer %q", answer)
	}
}
