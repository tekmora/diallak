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
	// MinTimeout is the minimum allowed duration to wait for a completion response.
	MinTimeout = 1 * time.Second
	// DefaultTimeout is the default duration to wait for a completion response.
	// Source: chosen, not measured.
	DefaultTimeout = 45 * time.Second
	// MaxTimeout is the maximum allowed completion timeout.
	MaxTimeout = 5 * time.Minute
	// DefaultMaxOutputTokens is the default token limit for completion generations.
	// Source: chosen, not measured.
	DefaultMaxOutputTokens = 512
	// MaxResponseBytes is the maximum allowed HTTP response size in bytes (1 MiB).
	MaxResponseBytes = 1 << 20
	// ProbeTimeout is the maximum duration for a startup or health probe.
	// Source: chosen, not measured.
	ProbeTimeout = 10 * time.Second
)

// StatusError reports an unexpected HTTP status code from the model endpoint.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("model endpoint returned status %d", e.Code)
}

// TransportError marks an error from sending the request or reading the response body.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string {
	return e.Err.Error()
}

func (e *TransportError) Unwrap() error {
	return e.Err
}

// Class categorizes errors from the model endpoint.
type Class int

const (
	// ClassOther represents errors other than rejected or unavailable.
	ClassOther Class = iota
	// ClassRejected represents client-side or configuration errors rejected by the endpoint.
	ClassRejected
	// ClassUnavailable represents transient reachability, timeout, or server-side failures.
	ClassUnavailable
)

// Classify categorizes err into a Class.
func Classify(err error) Class {
	if err == nil {
		return ClassOther
	}
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		switch {
		case statusErr.Code == http.StatusRequestTimeout:
			return ClassUnavailable
		case statusErr.Code == http.StatusTooManyRequests:
			return ClassUnavailable
		case statusErr.Code >= 500 && statusErr.Code <= 599:
			return ClassUnavailable
		default:
			return ClassRejected
		}
	}
	var transportErr *TransportError
	if errors.As(err, &transportErr) {
		return ClassUnavailable
	}
	return ClassOther
}

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
// If hc is nil, New creates its own http.Client. That client follows no redirect: a redirecting
// endpoint is an error. A caller that passes hc decides its redirect policy itself.
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
		client = &http.Client{CheckRedirect: refuseRedirect}
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

// refuseRedirect keeps the prompt and the credential at the configured endpoint: the response
// that redirects is returned as it is, and Complete reports its status as an error.
func refuseRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
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

// sendRequest builds the request, sets headers, sends, checks status and reads the limited body.
func (c *Client) sendRequest(ctx context.Context, timeout time.Duration, reqBody chatRequest) (_ []byte, err error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", &TransportError{Err: err})
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close response body: %w", closeErr))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		if _, copyErr := io.Copy(io.Discard, io.LimitReader(resp.Body, int64(MaxResponseBytes+1))); copyErr != nil {
			return nil, errors.Join(&StatusError{Code: resp.StatusCode}, fmt.Errorf("drain response body: %w", copyErr))
		}
		return nil, &StatusError{Code: resp.StatusCode}
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(MaxResponseBytes+1)))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", &TransportError{Err: err})
	}
	if len(data) > MaxResponseBytes {
		return nil, fmt.Errorf("response body exceeds max %d bytes", MaxResponseBytes)
	}

	return data, nil
}

// Complete sends a prompt to the chat completion endpoint and returns the generated content.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
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
	data, err := c.sendRequest(ctx, c.timeout, reqBody)
	if err != nil {
		return "", err
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

// Probe sends a lightweight probe to the model endpoint to verify routing and reachability.
func (c *Client) Probe(ctx context.Context) error {
	reqBody := chatRequest{
		Model: c.alias,
		Messages: []chatMessage{
			{Role: "user", Content: "ping"},
		},
		MaxTokens:   1,
		Temperature: 0,
		Stream:      false,
	}
	_, err := c.sendRequest(ctx, min(ProbeTimeout, c.timeout), reqBody)
	return err
}
