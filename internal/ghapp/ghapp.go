// Package ghapp provides GitHub App authentication and API client operations for the news function.
package ghapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	// RequestTimeout is the deadline of one request to GitHub's API.
	// Source: chosen, not measured.
	RequestTimeout = 20 * time.Second
	// MaxResponseBytes bounds the response body taken from GitHub's API (1 MiB).
	// Source: chosen, not measured.
	MaxResponseBytes = 1 << 20
	// MinBackoff is the shortest wait after a server asked to slow down: GitHub asks for "at least
	// one minute" when it names no time (github/docs dae83f7d28f3, "Exceeding the rate limit").
	MinBackoff = time.Minute
	// MaxBackoff caps the wait a server can ask for. Source: chosen, not measured.
	MaxBackoff = 24 * time.Hour
)

// ErrTooLarge reports a response larger than MaxResponseBytes.
var ErrTooLarge = fmt.Errorf("ghapp: response exceeds max %d bytes", MaxResponseBytes)

// StatusError reports an HTTP error response from GitHub's API.
type StatusError struct {
	Code       int
	RetryAfter time.Duration
}

func (e *StatusError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("ghapp: status %d, retry after %v", e.Code, e.RetryAfter)
	}
	return fmt.Sprintf("ghapp: status %d", e.Code)
}

// App authenticates as a GitHub App and sends authenticated requests to GitHub's API.
type App struct {
	ClientID string
	Key      *rsa.PrivateKey
	HTTP     *http.Client
	BaseURL  string
	Now      func() time.Time

	mu             sync.Mutex
	installationID int64
	token          string
	tokenExpiry    time.Time
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	IAT int64  `json:"iat"`
	EXP int64  `json:"exp"`
	ISS string `json:"iss"`
}

// ParseKey parses an RSA private key from PEM-encoded PKCS#1 or PKCS#8 bytes.
func ParseKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("ghapp: no PEM block found")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("ghapp: parse PKCS#1 key: %w", err)
		}
		return key, nil
	case "PRIVATE KEY":
		keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("ghapp: parse PKCS#8 key: %w", err)
		}
		key, ok := keyAny.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("ghapp: PKCS#8 key is not an RSA private key")
		}
		return key, nil
	default:
		return nil, fmt.Errorf("ghapp: unsupported PEM block type %q", block.Type)
	}
}

// JWT generates an RS256 JWT for authenticating as the GitHub App.
func (a *App) JWT(now time.Time) (string, error) {
	if a.Key == nil {
		return "", errors.New("ghapp: private key is nil")
	}
	if a.ClientID == "" {
		return "", errors.New("ghapp: client ID is empty")
	}
	headBytes, err := json.Marshal(jwtHeader{Alg: "RS256", Typ: "JWT"})
	if err != nil {
		return "", fmt.Errorf("ghapp: marshal jwt header: %w", err)
	}
	claimsBytes, err := json.Marshal(jwtClaims{
		IAT: now.Add(-60 * time.Second).Unix(),
		EXP: now.Add(9 * time.Minute).Unix(),
		ISS: a.ClientID,
	})
	if err != nil {
		return "", fmt.Errorf("ghapp: marshal jwt claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(headBytes) + "." + base64.RawURLEncoding.EncodeToString(claimsBytes)
	hash := sha256.Sum256([]byte(payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.Key, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("ghapp: sign jwt: %w", err)
	}
	return payload + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Token returns a cached installation access token, requesting a new one when expired or
// when less than 5 minutes remain.
func (a *App) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if a.token != "" && a.tokenExpiry.Sub(now) >= 5*time.Minute {
		return a.token, nil
	}
	jwt, err := a.JWT(now)
	if err != nil {
		return "", fmt.Errorf("ghapp: mint jwt: %w", err)
	}
	if a.installationID == 0 {
		id, err := a.discoverInstallation(ctx, jwt)
		if err != nil {
			return "", err
		}
		a.installationID = id
	}
	return a.mintToken(ctx, jwt)
}

func (a *App) discoverInstallation(ctx context.Context, jwt string) (int64, error) {
	var list []struct {
		ID int64 `json:"id"`
	}
	if err := a.doWithAuth(ctx, http.MethodGet, "/app/installations", "Bearer "+jwt, nil, &list); err != nil {
		return 0, fmt.Errorf("ghapp: get installations: %w", err)
	}
	if len(list) != 1 {
		return 0, fmt.Errorf("ghapp: want exactly 1 installation, got %d", len(list))
	}
	return list[0].ID, nil
}

func (a *App) mintToken(ctx context.Context, jwt string) (string, error) {
	reqBody := struct {
		Permissions map[string]string `json:"permissions"`
	}{
		Permissions: map[string]string{"discussions": "read", "metadata": "read"},
	}
	var resp struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	path := fmt.Sprintf("/app/installations/%d/access_tokens", a.installationID)
	if err := a.doWithAuth(ctx, http.MethodPost, path, "Bearer "+jwt, reqBody, &resp); err != nil {
		return "", fmt.Errorf("ghapp: create access token: %w", err)
	}
	exp, err := time.Parse(time.RFC3339, resp.ExpiresAt)
	if err != nil {
		return "", fmt.Errorf("ghapp: parse token expires_at: %w", err)
	}
	a.token = resp.Token
	a.tokenExpiry = exp
	return a.token, nil
}

// Do sends an HTTP request with method, path, and optional JSON body to BaseURL with the
// installation bearer token, and decodes the JSON response into out when out is non-nil.
func (a *App) Do(ctx context.Context, method, path string, body any, out any) error {
	tok, err := a.Token(ctx)
	if err != nil {
		return fmt.Errorf("ghapp: get token: %w", err)
	}
	return a.doWithAuth(ctx, method, path, "Bearer "+tok, body, out)
}

func (a *App) buildRequest(ctx context.Context, method, path, authHeader string, body any) (*http.Request, error) {
	fullURL, err := a.resolveURL(path)
	if err != nil {
		return nil, err
	}
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("ghapp: encode request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("ghapp: create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func parseResponse(resp *http.Response, now time.Time, out any) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("ghapp: read response: %w", err)
	}
	if len(data) > MaxResponseBytes {
		return ErrTooLarge
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &StatusError{Code: resp.StatusCode, RetryAfter: SlowDown(resp.StatusCode, resp.Header, now)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("ghapp: decode response: %w", err)
		}
	}
	return nil
}

func (a *App) doWithAuth(ctx context.Context, method, path, authHeader string, body any, out any) (err error) {
	reqCtx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	req, err := a.buildRequest(reqCtx, method, path, authHeader, body)
	if err != nil {
		return err
	}
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("ghapp: do request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("ghapp: close response body: %w", closeErr))
		}
	}()
	return parseResponse(resp, a.now(), out)
}

func (a *App) resolveURL(path string) (string, error) {
	base := a.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	baseU, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("ghapp: parse base URL: %w", err)
	}
	targetU, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("ghapp: parse target URL: %w", err)
	}
	// Resolve first, so a scheme-relative "//host/path" is checked like an absolute address.
	resolved := baseU.ResolveReference(targetU)
	if resolved.Host != baseU.Host || resolved.Scheme != baseU.Scheme {
		return "", fmt.Errorf("ghapp: target host %q does not match base host %q", resolved.Host, baseU.Host)
	}
	return resolved.String(), nil
}

func (a *App) httpClient() *http.Client {
	if a.HTTP == nil {
		return &http.Client{CheckRedirect: refuseRedirect, Timeout: RequestTimeout}
	}
	c := *a.HTTP
	c.CheckRedirect = refuseRedirect
	return &c
}

func refuseRedirect(*http.Request, []*http.Request) error {
	return errors.New("ghapp: redirects refused")
}

func (a *App) now() time.Time {
	if a.Now == nil {
		return time.Now()
	}
	return a.Now()
}

// SlowDown returns how long to wait after an answer with status code and headers h, or zero when
// the answer does not ask to slow down. It follows GitHub's rule for 403 and 429 (github/docs
// dae83f7d28f3, "Exceeding the rate limit") and RFC 9110, section 10.2.3, for 503. The news
// reader uses it for feed servers too.
func SlowDown(code int, h http.Header, now time.Time) time.Duration {
	if code != http.StatusTooManyRequests && code != http.StatusServiceUnavailable && code != http.StatusForbidden {
		return 0
	}
	if d, ok := retryAfter(h.Get("Retry-After"), now); ok {
		return clampBackoff(d)
	}
	if h.Get("X-Ratelimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(h.Get("X-Ratelimit-Reset"), 10, 64); err == nil {
			return clampBackoff(time.Unix(reset, 0).Sub(now))
		}
		return MinBackoff
	}
	if code == http.StatusForbidden {
		return 0
	}
	return MinBackoff
}

// retryAfter reads a Retry-After value: delay-seconds or an HTTP date (RFC 9110, section 10.2.3).
func retryAfter(v string, now time.Time) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseUint(v, 10, 32); err == nil {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return t.Sub(now), true
	}
	return 0, false
}

func clampBackoff(d time.Duration) time.Duration {
	return min(max(d, MinBackoff), MaxBackoff)
}
