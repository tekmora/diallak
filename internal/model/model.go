// Package model provides an HTTP client for OpenAI-compatible model completion endpoints.
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// DefaultTimeout is the default duration to wait for a completion response.
	DefaultTimeout = 45 * time.Second
	// MaxTimeout is the maximum allowed completion timeout.
	MaxTimeout = 5 * time.Minute
	// DefaultMaxOutputTokens is the default token limit for completion generations.
	DefaultMaxOutputTokens = 512
	// MaxResponseBytes is the maximum allowed HTTP response size in bytes (1 MiB).
	MaxResponseBytes = 1 << 20
)

var aliasRegexp = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,128}$`)

// Config holds configuration parameters for the model client.
type Config struct {
	BaseURL         string
	Alias           string
	APIKey          string
	Timeout         time.Duration
	MaxOutputTokens int
}

// Client communicates with an OpenAI-compatible model completion endpoint.
type Client struct {
	baseURL    string
	alias      string
	apiKey     string
	timeout    time.Duration
	maxTokens  int
	httpClient *http.Client
}

// New validates cfg and returns a new Client.
//
// BaseURL must be a valid http or https URL without userinfo, query, or fragment.
// Alias must match ^[A-Za-z0-9._:/-]{1,128}$.
// Timeout must be positive and at most MaxTimeout (0 uses DefaultTimeout).
// MaxOutputTokens must be between 1 and 4096 (0 uses DefaultMaxOutputTokens).
// If hc is nil, an isolated http.Client without global state is created.
// Error text never contains cfg.APIKey.
func New(cfg Config, hc *http.Client) (*Client, error) {
	if err := validateBaseURL(cfg.BaseURL); err != nil {
		return nil, err
	}
	if !aliasRegexp.MatchString(cfg.Alias) {
		return nil, fmt.Errorf("invalid model alias %q", cfg.Alias)
	}

	timeout, err := validateTimeout(cfg.Timeout)
	if err != nil {
		return nil, err
	}

	maxTokens, err := validateMaxTokens(cfg.MaxOutputTokens)
	if err != nil {
		return nil, err
	}

	client := hc
	if client == nil {
		client = &http.Client{}
	}

	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		alias:      cfg.Alias,
		apiKey:     cfg.APIKey,
		timeout:    timeout,
		maxTokens:  maxTokens,
		httpClient: client,
	}, nil
}

func validateBaseURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported base URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("base URL host is empty")
	}
	if u.User != nil {
		return errors.New("base URL contains userinfo")
	}
	if u.RawQuery != "" {
		return errors.New("base URL contains query parameters")
	}
	if u.Fragment != "" {
		return errors.New("base URL contains fragment")
	}
	return nil
}

func validateTimeout(d time.Duration) (time.Duration, error) {
	if d == 0 {
		return DefaultTimeout, nil
	}
	if d < 0 || d > MaxTimeout {
		return 0, fmt.Errorf("timeout %v outside (0, %v]", d, MaxTimeout)
	}
	return d, nil
}

func validateMaxTokens(n int) (int, error) {
	if n == 0 {
		return DefaultMaxOutputTokens, nil
	}
	if n < 1 || n > 4096 {
		return 0, fmt.Errorf("max output tokens %d outside 1..4096", n)
	}
	return n, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// Complete sends a prompt to the chat completion endpoint and returns the generated content.
func (c *Client) Complete(ctx context.Context, system, user string) (_ string, err error) {
	reqBody := chatRequest{
		Model: c.alias,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens:   c.maxTokens,
		Temperature: 0,
		Stream:      false,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close response body: %w", closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("model endpoint returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(MaxResponseBytes+1)))
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}
	if len(data) > MaxResponseBytes {
		return "", fmt.Errorf("response body exceeds max %d bytes", MaxResponseBytes)
	}

	return parseChatResponse(data)
}

func parseChatResponse(data []byte) (string, error) {
	var chatResp chatResponse
	if err := json.Unmarshal(data, &chatResp); err != nil {
		return "", fmt.Errorf("decode response json: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", errors.New("response contains no choices")
	}
	content := chatResp.Choices[0].Message.Content
	if content == "" {
		return "", errors.New("response choice content is empty")
	}
	return content, nil
}
