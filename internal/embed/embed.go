// Package embed provides an HTTP client for OpenAI-compatible embedding endpoints.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tekmora/diallak/internal/model"
)

const (
	// MinTimeout is the minimum allowed duration to wait for an embedding response.
	MinTimeout = 1 * time.Second
	// DefaultTimeout is the default duration to wait for an embedding response.
	// Source: chosen, not measured.
	DefaultTimeout = 10 * time.Second
	// MaxTimeout is the maximum allowed embedding timeout.
	// Source: chosen, not measured.
	MaxTimeout = 60 * time.Second
	// MaxBatch is the maximum number of texts that can be embedded in one request.
	MaxBatch = 64
	// MaxResponseBytes is the maximum allowed HTTP response size in bytes (4 MiB).
	// Source: chosen, not measured; bounds response body.
	MaxResponseBytes = 4 << 20
)

// Trace records whether the member's question left the bot for an embedding endpoint.
type Trace struct {
	Sent bool
	Host string
}

// StatusError reports an unexpected HTTP status code from the embedding endpoint.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("embedding endpoint returned status %d", e.Code)
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

// Classify returns the class of an embedding error for logs: "timeout", "status", "transport"
// or "error". It never returns error text, which may name the endpoint.
func Classify(err error) string {
	var statusErr *StatusError
	var transportErr *TransportError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.As(err, &statusErr):
		return "status"
	case errors.As(err, &transportErr):
		return "transport"
	default:
		return "error"
	}
}

// Config holds configuration parameters for the embedding client.
type Config struct {
	BaseURL string
	Alias   string
	APIKey  string
	Timeout time.Duration
}

// Client communicates with an OpenAI-compatible embedding endpoint.
type Client struct {
	baseURL    string
	alias      string
	apiKey     string
	host       string
	timeout    time.Duration
	httpClient *http.Client
}

// OnPremises enforces that rawURL resolves to an on-premises host.
// It uses an allow-list, never a deny list. It passes only when the URL host is:
// (a) an IP literal that is loopback, or private (RFC 1918, fc00::/7);
// (b) localhost;
// (c) a single-label host name (no dot);
// (d) a host name ending in one of .svc, .cluster.local, .internal, .home.arpa, .lan.
// Comparison is case-insensitive, with any trailing dot stripped.
// Everything else is refused without DNS lookup (ADR-0022).
func OnPremises(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse base URL: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("base URL host is empty")
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return errors.New("embedding base URL host is empty: a hosted embedding service is never used (ADR-0022)")
	}
	hostLower := strings.ToLower(host)

	if ip := net.ParseIP(hostLower); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() {
			return nil
		}
		return fmt.Errorf("embedding base URL host %q is not on-premises: a hosted embedding service is never used (ADR-0022)", host)
	}

	if hostLower == "localhost" || !strings.Contains(hostLower, ".") {
		return nil
	}

	suffixes := [5]string{".svc", ".cluster.local", ".internal", ".home.arpa", ".lan"}
	for i := 0; i < len(suffixes); i++ {
		if strings.HasSuffix(hostLower, suffixes[i]) {
			return nil
		}
	}

	return fmt.Errorf("embedding base URL host %q is not on-premises: a hosted embedding service is never used (ADR-0022)", host)
}

// New validates cfg and returns a new Client.
// Error text never contains cfg.APIKey.
func New(cfg Config, hc *http.Client) (*Client, error) {
	if err := model.ValidateBaseURL(cfg.BaseURL); err != nil {
		return nil, err
	}
	if err := OnPremises(cfg.BaseURL); err != nil {
		return nil, err
	}
	if !model.ValidAlias(cfg.Alias) {
		return nil, fmt.Errorf("invalid embedding alias %q", cfg.Alias)
	}

	timeout, err := model.ResolveTimeout(cfg.Timeout, DefaultTimeout, MaxTimeout)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}

	client := hc
	if client == nil {
		client = &http.Client{CheckRedirect: model.RefuseRedirect, Timeout: timeout}
	}

	return &Client{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		alias:      cfg.Alias,
		apiKey:     cfg.APIKey,
		host:       u.Host,
		timeout:    timeout,
		httpClient: client,
	}, nil
}

// Host returns the host of the embedding endpoint.
func (c *Client) Host() string {
	return c.host
}

// Trace returns a Trace for a request handed to the embedding client.
func (c *Client) Trace() Trace {
	return Trace{Sent: true, Host: c.host}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedItem struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type embedResponse struct {
	Data []embedItem `json:"data"`
}

// Embed generates embedding vectors for texts using the configured endpoint.
// Request is bounded to MaxBatch texts. Errors never contain the key or response body.
func (c *Client) Embed(ctx context.Context, texts []string) (_ [][]float32, err error) {
	if len(texts) == 0 {
		return nil, errors.New("texts slice is empty")
	}
	if len(texts) > MaxBatch {
		return nil, fmt.Errorf("batch size %d exceeds max %d", len(texts), MaxBatch)
	}

	reqBody := embedRequest{Model: c.alias, Input: texts}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := c.baseURL + "/embeddings"
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
		return nil, drainWith(resp.Body, &StatusError{Code: resp.StatusCode})
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(MaxResponseBytes+1)))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", &TransportError{Err: err})
	}
	if len(data) > MaxResponseBytes {
		return nil, fmt.Errorf("response body exceeds max %d bytes", MaxResponseBytes)
	}

	return parseEmbedResponse(data, len(texts))
}

func drainWith(body io.Reader, cause error) error {
	if _, copyErr := io.Copy(io.Discard, io.LimitReader(body, int64(MaxResponseBytes+1))); copyErr != nil {
		return errors.Join(cause, fmt.Errorf("drain response body: %w", copyErr))
	}
	return cause
}

func parseEmbedResponse(data []byte, wantCount int) ([][]float32, error) {
	var resp embedResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode response json: %w", err)
	}
	if len(resp.Data) != wantCount {
		return nil, fmt.Errorf("expected %d embeddings, got %d", wantCount, len(resp.Data))
	}
	if len(resp.Data[0].Embedding) == 0 {
		return nil, errors.New("embedding vector dimension is zero")
	}

	dim := len(resp.Data[0].Embedding)
	res := make([][]float32, wantCount)
	filled := make([]bool, wantCount)

	for i := 0; i < len(resp.Data); i++ {
		item := resp.Data[i]
		if item.Index < 0 || item.Index >= wantCount {
			return nil, fmt.Errorf("embedding index %d outside 0..%d", item.Index, wantCount-1)
		}
		if filled[item.Index] {
			return nil, fmt.Errorf("duplicate embedding index %d", item.Index)
		}
		if len(item.Embedding) != dim {
			return nil, fmt.Errorf("mixed vector dimensions: expected %d, got %d", dim, len(item.Embedding))
		}
		res[item.Index] = item.Embedding
		filled[item.Index] = true
	}

	for i := 0; i < len(filled); i++ {
		if !filled[i] {
			return nil, fmt.Errorf("missing embedding for index %d", i)
		}
	}
	return res, nil
}
