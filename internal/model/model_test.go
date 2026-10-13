package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// verifyValidConfig verifies that New succeeds and initializes fields matching expectations.
func verifyValidConfig(t *testing.T, cfg Config, wantURL string, wantTimeout time.Duration, wantTokens int) {
	t.Helper()
	c, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	if c.baseURL != wantURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, wantURL)
	}
	if c.timeout != wantTimeout {
		t.Errorf("timeout = %v, want %v", c.timeout, wantTimeout)
	}
	if c.httpClient.Timeout != wantTimeout {
		t.Errorf("httpClient.Timeout = %v, want %v", c.httpClient.Timeout, wantTimeout)
	}
	if c.maxTokens != wantTokens {
		t.Errorf("maxTokens = %d, want %d", c.maxTokens, wantTokens)
	}
}

func TestNew_Positive_ValidConfigurations(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		wantURL     string
		wantTimeout time.Duration
		wantTokens  int
	}{
		{
			name: "defaults applied",
			cfg: Config{
				BaseURL: "http://localhost:8080",
				Alias:   "router/test",
				Policy:  "example-policy",
			},
			wantURL:     "http://localhost:8080",
			wantTimeout: DefaultTimeout,
			wantTokens:  DefaultMaxOutputTokens,
		},
		{
			name: "custom parameters with trailing slash",
			cfg: Config{
				BaseURL:         "https://example.com/api/v1/",
				Alias:           "custom-alias.v1:prod",
				Policy:          "example-policy",
				APIKey:          "secret-key",
				Timeout:         2 * time.Minute,
				MaxOutputTokens: 1024,
			},
			wantURL:     "https://example.com/api/v1",
			wantTimeout: 2 * time.Minute,
			wantTokens:  1024,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verifyValidConfig(t, tc.cfg, tc.wantURL, tc.wantTimeout, tc.wantTokens)
		})
	}
}

type newValidationTestCase struct {
	name    string
	cfg     Config
	wantErr string
}

// newValidationURLCases returns invalid URL configuration test cases.
func newValidationURLCases(secretKey string) []newValidationTestCase {
	return []newValidationTestCase{
		{
			name:    "empty base URL",
			cfg:     Config{BaseURL: "", Alias: "router/test", Policy: "example-policy", APIKey: secretKey},
			wantErr: "unsupported base URL scheme",
		},
		{
			name:    "invalid scheme ftp",
			cfg:     Config{BaseURL: "ftp://localhost:8080", Alias: "router/test", Policy: "example-policy", APIKey: secretKey},
			wantErr: "unsupported base URL scheme",
		},
		{
			name:    "empty host",
			cfg:     Config{BaseURL: "http:///path", Alias: "router/test", Policy: "example-policy", APIKey: secretKey},
			wantErr: "base URL host is empty",
		},
		{
			name:    "contains userinfo",
			cfg:     Config{BaseURL: "http://user:pass@localhost:8080", Alias: "router/test", Policy: "example-policy", APIKey: secretKey},
			wantErr: "userinfo",
		},
		{
			name:    "contains query",
			cfg:     Config{BaseURL: "http://localhost:8080?foo=bar", Alias: "router/test", Policy: "example-policy", APIKey: secretKey},
			wantErr: "query",
		},
		{
			name:    "contains fragment",
			cfg:     Config{BaseURL: "http://localhost:8080#section", Alias: "router/test", Policy: "example-policy", APIKey: secretKey},
			wantErr: "fragment",
		},
	}
}

// newValidationFieldCases returns invalid field configuration test cases.
func newValidationFieldCases(secretKey string) []newValidationTestCase {
	return []newValidationTestCase{
		{
			name:    "empty alias",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "", Policy: "example-policy", APIKey: secretKey},
			wantErr: "invalid model alias",
		},
		{
			name:    "alias with invalid characters",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "bad alias!", Policy: "example-policy", APIKey: secretKey},
			wantErr: "invalid model alias",
		},
		{
			name:    "empty policy",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", Policy: "", APIKey: secretKey},
			wantErr: "invalid model policy",
		},
		{
			name:    "policy with space",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", Policy: "bad policy", APIKey: secretKey},
			wantErr: "invalid model policy",
		},
		{
			name:    "policy with comma",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", Policy: "bad,policy", APIKey: secretKey},
			wantErr: "invalid model policy",
		},
		{
			name:    "negative timeout",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", Policy: "example-policy", Timeout: -1 * time.Second, APIKey: secretKey},
			wantErr: "outside (0,",
		},
		{
			name:    "timeout exceeds maximum",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", Policy: "example-policy", Timeout: MaxTimeout + time.Second, APIKey: secretKey},
			wantErr: "outside (0,",
		},
		{
			name:    "negative max output tokens",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", Policy: "example-policy", MaxOutputTokens: -5, APIKey: secretKey},
			wantErr: "outside 1..4096",
		},
		{
			name:    "max output tokens exceeds limit",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", Policy: "example-policy", MaxOutputTokens: 4097, APIKey: secretKey},
			wantErr: "outside 1..4096",
		},
	}
}

// newValidationTestCases returns all invalid configuration test cases.
func newValidationTestCases(secretKey string) []newValidationTestCase {
	return append(newValidationURLCases(secretKey), newValidationFieldCases(secretKey)...)
}

func TestNew_Negative_ValidationErrors(t *testing.T) {
	const secretKey = "super-secret-key-12345"
	tests := newValidationTestCases(secretKey)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg, nil)
			if err == nil {
				t.Fatal("New() expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
			if strings.Contains(err.Error(), secretKey) {
				t.Fatalf("error leaks APIKey: %s", err.Error())
			}
		})
	}
}

// verifyNewBoundaryAlias tests alias length boundaries for New.
func verifyNewBoundaryAlias(t *testing.T, baseCfg Config) {
	t.Helper()
	validAlias128 := strings.Repeat("a", 128)
	cfg128 := baseCfg
	cfg128.Alias = validAlias128
	if _, err := New(cfg128, nil); err != nil {
		t.Errorf("New() with 128-char alias failed: %v", err)
	}

	invalidAlias129 := strings.Repeat("a", 129)
	cfg129 := baseCfg
	cfg129.Alias = invalidAlias129
	if _, err := New(cfg129, nil); err == nil {
		t.Error("New() with 129-char alias passed, want error")
	}
}

// verifyNewBoundaryPolicy tests policy length and punctuation boundaries for New.
func verifyNewBoundaryPolicy(t *testing.T, baseCfg Config) {
	t.Helper()
	validPolicy128 := strings.Repeat("p", 128)
	cfgPolicy128 := baseCfg
	cfgPolicy128.Policy = validPolicy128
	if _, err := New(cfgPolicy128, nil); err != nil {
		t.Errorf("New() with 128-char policy failed: %v", err)
	}

	invalidPolicy129 := strings.Repeat("p", 129)
	cfgPolicy129 := baseCfg
	cfgPolicy129.Policy = invalidPolicy129
	if _, err := New(cfgPolicy129, nil); err == nil {
		t.Error("New() with 129-char policy passed, want error")
	}

	punctPolicy := "a.b_c:d/e-f"
	cfgPunct := baseCfg
	cfgPunct.Policy = punctPolicy
	if _, err := New(cfgPunct, nil); err != nil {
		t.Errorf("New() with punctuation policy failed: %v", err)
	}
}

// verifyNewBoundaryTokens tests token limit boundaries for New.
func verifyNewBoundaryTokens(t *testing.T, baseCfg Config) {
	t.Helper()
	cfgTok1 := baseCfg
	cfgTok1.MaxOutputTokens = 1
	if c, err := New(cfgTok1, nil); err != nil || c.maxTokens != 1 {
		t.Errorf("New() with 1 token failed: %v", err)
	}

	cfgTok4096 := baseCfg
	cfgTok4096.MaxOutputTokens = 4096
	if c, err := New(cfgTok4096, nil); err != nil || c.maxTokens != 4096 {
		t.Errorf("New() with 4096 tokens failed: %v", err)
	}
}

// verifyNewBoundaryTimeout tests timeout boundaries for New.
func verifyNewBoundaryTimeout(t *testing.T, baseCfg Config) {
	t.Helper()
	cfgMaxTimeout := baseCfg
	cfgMaxTimeout.Timeout = MaxTimeout
	if c, err := New(cfgMaxTimeout, nil); err != nil || c.timeout != MaxTimeout {
		t.Errorf("New() with MaxTimeout failed: %v", err)
	}
	if c, err := New(cfgMaxTimeout, nil); err != nil || c.httpClient.Timeout != MaxTimeout {
		t.Errorf("New() with MaxTimeout: own client timeout not MaxTimeout (err %v)", err)
	}

	cfgOverTimeout := baseCfg
	cfgOverTimeout.Timeout = MaxTimeout + time.Nanosecond
	if _, err := New(cfgOverTimeout, nil); err == nil {
		t.Error("New() with MaxTimeout+1ns passed, want error")
	}
}

func TestNew_Boundary_Limits(t *testing.T) {
	baseCfg := Config{
		BaseURL: "http://localhost:8080",
		Alias:   "router/test",
		Policy:  "example-policy",
	}

	verifyNewBoundaryAlias(t, baseCfg)
	verifyNewBoundaryPolicy(t, baseCfg)
	verifyNewBoundaryTokens(t, baseCfg)
	verifyNewBoundaryTimeout(t, baseCfg)
}

func TestNew_Negative_CallerClientKeepsItsTimeout(t *testing.T) {
	hc := &http.Client{}
	c, err := New(Config{BaseURL: "http://localhost:8080", Alias: "router/test", Policy: "example-policy"}, hc)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	if c.httpClient != hc || hc.Timeout != 0 {
		t.Errorf("New() replaced or changed the caller's client: timeout %v", hc.Timeout)
	}
}

type capturedCompleteState struct {
	req         chatRequest
	auth        string
	contentType string
	path        string
}

// newCompleteMockServer creates an httptest server recording request details for Complete tests.
func newCompleteMockServer(state *capturedCompleteState) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.path = r.URL.Path
		state.auth = r.Header.Get("Authorization")
		state.contentType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(body, &state.req); err != nil {
			http.Error(w, "unmarshal error", http.StatusBadRequest)
			return
		}

		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: "Simulated answer text."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
	}))
}

// verifyCompleteCapturedRequest verifies headers, payload, and returned content from Complete.
func verifyCompleteCapturedRequest(t *testing.T, content string, state capturedCompleteState) {
	t.Helper()
	if content != "Simulated answer text." {
		t.Errorf("content = %q, want %q", content, "Simulated answer text.")
	}
	if state.path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", state.path)
	}
	if state.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", state.contentType)
	}
	if state.auth != "Bearer test-secret-key" {
		t.Errorf("Authorization = %q, want 'Bearer test-secret-key'", state.auth)
	}
	if state.req.Model != "router/test" {
		t.Errorf("model = %q, want 'router/test'", state.req.Model)
	}
	if state.req.MaxTokens != 256 {
		t.Errorf("max_tokens = %d, want 256", state.req.MaxTokens)
	}
	if state.req.Temperature != 0 {
		t.Errorf("temperature = %v, want 0", state.req.Temperature)
	}
	if state.req.Stream {
		t.Errorf("stream = %v, want false", state.req.Stream)
	}
	if len(state.req.Messages) != 2 ||
		state.req.Messages[0].Content != "System prompt" ||
		state.req.Messages[1].Content != "User prompt" {
		t.Errorf("messages = %+v, unexpected", state.req.Messages)
	}
}

func TestComplete_Positive_RequestShapeAndResponse(t *testing.T) {
	var state capturedCompleteState
	server := newCompleteMockServer(&state)
	defer server.Close()

	client, err := New(Config{
		BaseURL:         server.URL,
		Alias:           "router/test",
		Policy:          "example-policy",
		APIKey:          "test-secret-key",
		MaxOutputTokens: 256,
	}, server.Client())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	content, err := client.Complete(context.Background(), "System prompt", "User prompt")
	if err != nil {
		t.Fatalf("Complete() unexpected error: %v", err)
	}

	verifyCompleteCapturedRequest(t, content, state)
}

func TestComplete_Positive_AuthorizationAbsentWithoutKey(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: "OK"}},
			},
		}
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
		APIKey:  "",
	}, server.Client())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	if _, completeErr := client.Complete(context.Background(), "sys", "usr"); completeErr != nil {
		t.Fatalf("Complete() failed: %v", completeErr)
	}
	if authHeader != "" {
		t.Errorf("Authorization header = %q, want empty", authHeader)
	}
}

func TestComplete_Positive_TrailingSlashInBaseURL(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: "OK"}},
			},
		}
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL + "///",
		Alias:   "router/test",
		Policy:  "example-policy",
	}, server.Client())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	if _, completeErr := client.Complete(context.Background(), "sys", "usr"); completeErr != nil {
		t.Fatalf("Complete() failed: %v", completeErr)
	}
	if requestedPath != "/chat/completions" {
		t.Errorf("Path = %q, want /chat/completions", requestedPath)
	}
}

func TestComplete_Negative_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("sensitive internal server error details")); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
		APIKey:  "secret-key",
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_, completeErr := client.Complete(context.Background(), "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() want error on non-200, got nil")
	}
	if !strings.Contains(completeErr.Error(), "status 500") {
		t.Errorf("error %q should mention status 500", completeErr.Error())
	}
	if strings.Contains(completeErr.Error(), "sensitive") {
		t.Errorf("error %q leaked response body", completeErr.Error())
	}
}

func TestComplete_Negative_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("{invalid-json")); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_, completeErr := client.Complete(context.Background(), "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() want error on malformed JSON, got nil")
	}
	if !strings.Contains(completeErr.Error(), "decode response json") {
		t.Errorf("unexpected error: %v", completeErr)
	}
}

func TestComplete_Negative_NoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"choices":[]}`)); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_, completeErr := client.Complete(context.Background(), "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() want error on empty choices, got nil")
	}
	if !strings.Contains(completeErr.Error(), "response contains no choices") {
		t.Errorf("unexpected error: %v", completeErr)
	}
}

func TestComplete_Negative_EmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`)); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_, completeErr := client.Complete(context.Background(), "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() want error on empty content, got nil")
	}
	if !strings.Contains(completeErr.Error(), "response choice content is empty") {
		t.Errorf("unexpected error: %v", completeErr)
	}
}

func TestComplete_Negative_BodyOverMaxResponseBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		w.WriteHeader(http.StatusOK)
		data := make([]byte, MaxResponseBytes+10)
		if _, err := w.Write(data); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_, completeErr := client.Complete(context.Background(), "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() want error on oversized body, got nil")
	}
	if !strings.Contains(completeErr.Error(), "exceeds max") {
		t.Errorf("unexpected error: %v", completeErr)
	}
}

func TestComplete_Negative_ServerSlowerThanTimeout(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer server.Close()
	defer close(done)

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
		Timeout: 50 * time.Millisecond,
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_, completeErr := client.Complete(context.Background(), "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() want timeout error, got nil")
	}
	if !errors.Is(completeErr, context.DeadlineExceeded) && !strings.Contains(completeErr.Error(), "deadline") {
		t.Errorf("error %v should wrap or describe deadline exceeded", completeErr)
	}
}

func TestComplete_Negative_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`)); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, completeErr := client.Complete(ctx, "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() with cancelled ctx want error, got nil")
	}
	if !errors.Is(completeErr, context.Canceled) && !strings.Contains(completeErr.Error(), "context canceled") {
		t.Errorf("unexpected error: %v", completeErr)
	}
}

func TestComplete_Negative_NeverLeaksAPIKey(t *testing.T) {
	const secretKey = "super-secret-api-key-xyz-987"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if _, err := w.Write([]byte("unauthorized")); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
		APIKey:  secretKey,
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	_, completeErr := client.Complete(context.Background(), "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() want error, got nil")
	}
	if strings.Contains(completeErr.Error(), secretKey) {
		t.Fatalf("Complete() error leaked APIKey: %s", completeErr.Error())
	}
}

func TestComplete_Negative_DefaultClientRefusesRedirects(t *testing.T) {
	var followed bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		followed = true
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		if _, err := w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"redirected"}}]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(target.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/chat/completions", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)

	// The prompt and the credential go to the configured endpoint only. A redirect is an error.
	client, err := New(Config{BaseURL: redirector.URL, Alias: "router/test", Policy: "example-policy", APIKey: "key-for-redirect-test"}, nil)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	out, err := client.Complete(context.Background(), "system", "user")
	if err == nil {
		t.Fatalf("Complete() = %q, want an error for a redirecting endpoint", out)
	}
	if followed {
		t.Fatalf("the client followed the redirect and sent the request to another URL")
	}
	if !strings.Contains(err.Error(), "307") {
		t.Fatalf("error = %v, want it to name status 307", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestComplete_Positive_ConfiguredTimeoutSeenByServer(t *testing.T) {
	var observedDeadline time.Time
	var hasDeadline bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedDeadline, hasDeadline = r.Context().Deadline()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		if _, err := w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	})

	testClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
	}

	configuredTimeout := 5 * time.Second
	client, err := New(Config{
		BaseURL: "http://diallak.test",
		Alias:   "router/test",
		Policy:  "example-policy",
		Timeout: configuredTimeout,
	}, testClient)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	start := time.Now()
	_, err = client.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Complete() failed: %v", err)
	}
	if !hasDeadline {
		t.Fatalf("request context seen by handler had no deadline")
	}
	duration := observedDeadline.Sub(start)
	if duration < 4*time.Second || duration > 6*time.Second {
		t.Errorf("observed deadline duration = %v, want approx %v", duration, configuredTimeout)
	}
}

func TestClassify_Boundary_Table(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Class
	}{
		{name: "nil", err: nil, want: ClassOther},
		{name: "policy error", err: &PolicyError{Applied: []string{"other"}}, want: ClassPolicy},
		{name: "status 399", err: &StatusError{Code: 399}, want: ClassRejected},
		{name: "status 400", err: &StatusError{Code: 400}, want: ClassRejected},
		{name: "status 407", err: &StatusError{Code: 407}, want: ClassRejected},
		{name: "status 408", err: &StatusError{Code: 408}, want: ClassUnavailable},
		{name: "status 409", err: &StatusError{Code: 409}, want: ClassRejected},
		{name: "status 428", err: &StatusError{Code: 428}, want: ClassRejected},
		{name: "status 429", err: &StatusError{Code: 429}, want: ClassUnavailable},
		{name: "status 430", err: &StatusError{Code: 430}, want: ClassRejected},
		{name: "status 499", err: &StatusError{Code: 499}, want: ClassRejected},
		{name: "status 500", err: &StatusError{Code: 500}, want: ClassUnavailable},
		{name: "status 599", err: &StatusError{Code: 599}, want: ClassUnavailable},
		{name: "status 600", err: &StatusError{Code: 600}, want: ClassRejected},
		{name: "status 200", err: &StatusError{Code: 200}, want: ClassRejected},
		{name: "status 201", err: &StatusError{Code: 201}, want: ClassRejected},
		{name: "status 302", err: &StatusError{Code: 302}, want: ClassRejected},
		{name: "transport connection refused", err: &TransportError{Err: errors.New("connection refused")}, want: ClassUnavailable},
		{name: "transport deadline exceeded", err: &TransportError{Err: context.DeadlineExceeded}, want: ClassUnavailable},
		{name: "wrapped transport", err: fmt.Errorf("do request: %w", &TransportError{Err: errors.New("reset")}), want: ClassUnavailable},
		{name: "decode error", err: fmt.Errorf("decode response json: %w", errors.New("syntax error")), want: ClassOther},
		{name: "generic error", err: errors.New("other failure"), want: ClassOther},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.err)
			if got != tc.want {
				t.Errorf("Classify(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

type capturedProbeState struct {
	path        string
	method      string
	auth        string
	contentType string
	req         chatRequest
}

// newProbeMockServer creates an httptest server recording request details for Probe tests.
func newProbeMockServer(t *testing.T, state *capturedProbeState) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.path = r.URL.Path
		state.method = r.Method
		state.auth = r.Header.Get("Authorization")
		state.contentType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(body, &state.req); err != nil {
			http.Error(w, "unmarshal error", http.StatusBadRequest)
			return
		}
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		w.WriteHeader(http.StatusOK)
		if _, writeErr := w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`)); writeErr != nil {
			t.Errorf("write response: %v", writeErr)
		}
	}))
}

// verifyProbeCapturedHeaders verifies path, method, content type, and authorization headers for Probe.
func verifyProbeCapturedHeaders(t *testing.T, state capturedProbeState, withKey bool) {
	t.Helper()
	if state.path != "/chat/completions" || state.method != http.MethodPost {
		t.Errorf("path/method = %s %s, want POST /chat/completions", state.method, state.path)
	}
	if state.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", state.contentType)
	}
	if withKey && state.auth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", state.auth)
	}
	if !withKey && state.auth != "" {
		t.Errorf("Authorization = %q, want empty", state.auth)
	}
}

// verifyProbeCapturedBody verifies the chat request payload structure for Probe.
func verifyProbeCapturedBody(t *testing.T, req chatRequest) {
	t.Helper()
	if req.Model != "test-alias" || req.MaxTokens != 1 || req.Stream {
		t.Errorf("req = %+v, unexpected fields", req)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content != "ping" {
		t.Errorf("messages = %+v, want single user ping message", req.Messages)
	}
}

// verifyProbeCapturedRequest verifies headers, method, and payload for Probe requests.
func verifyProbeCapturedRequest(t *testing.T, state capturedProbeState, withKey bool) {
	t.Helper()
	verifyProbeCapturedHeaders(t, state, withKey)
	verifyProbeCapturedBody(t, state.req)
}

// verifyProbeRequestShapeCase runs the Probe request shape verification for a key configuration.
func verifyProbeRequestShapeCase(t *testing.T, withKey bool, key string) {
	t.Helper()
	var state capturedProbeState
	server := newProbeMockServer(t, &state)
	defer server.Close()

	c, err := New(Config{
		BaseURL: server.URL,
		Alias:   "test-alias",
		Policy:  "example-policy",
		APIKey:  key,
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := c.Probe(context.Background()); err != nil {
		t.Fatalf("Probe() unexpected error: %v", err)
	}

	verifyProbeCapturedRequest(t, state, withKey)
}

func TestProbe_Positive_RequestShape(t *testing.T) {
	for _, withKey := range []bool{true, false} {
		name := "without key"
		key := ""
		if withKey {
			name = "with key"
			key = "test-key"
		}
		t.Run(name, func(t *testing.T) {
			verifyProbeRequestShapeCase(t, withKey, key)
		})
	}
}

func TestProbe_Positive_Status200WithNonJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		w.WriteHeader(http.StatusOK)
		if _, writeErr := w.Write([]byte("x")); writeErr != nil {
			t.Errorf("write response: %v", writeErr)
		}
	}))
	defer server.Close()

	c, err := New(Config{BaseURL: server.URL, Alias: "test-alias", Policy: "example-policy"}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if err := c.Probe(context.Background()); err != nil {
		t.Fatalf("Probe() with non-json body returned error: %v, want nil", err)
	}
}

func TestProbe_Boundary_DeadlineDerivedFromContext(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer server.Close()
	defer close(done)

	c, err := New(Config{
		BaseURL: server.URL,
		Alias:   "test-alias",
		Policy:  "example-policy",
		Timeout: 50 * time.Millisecond,
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	start := time.Now()
	err = c.Probe(context.Background())
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Probe() on hanging server want error, got nil")
	}
	if duration > 1*time.Second {
		t.Errorf("Probe took %v, want inside 1s", duration)
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("error %v should wrap or describe deadline exceeded", err)
	}
}

func TestClient_Negative_KeyAbsentFromEveryErrorText(t *testing.T) {
	const secretKey = "test-secret-key-probe-401"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if _, writeErr := w.Write([]byte("unauthorized: Bearer test-secret-key-probe-401 echoed back")); writeErr != nil {
			t.Errorf("write response: %v", writeErr)
		}
	}))
	defer server.Close()

	c, err := New(Config{
		BaseURL: server.URL,
		Alias:   "test-alias",
		Policy:  "example-policy",
		APIKey:  secretKey,
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	probeErr := c.Probe(context.Background())
	if probeErr == nil {
		t.Fatal("Probe() want error on 401, got nil")
	}
	if strings.Contains(probeErr.Error(), secretKey) {
		t.Fatalf("Probe error leaked APIKey: %s", probeErr.Error())
	}

	_, completeErr := c.Complete(context.Background(), "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() want error on 401, got nil")
	}
	if strings.Contains(completeErr.Error(), secretKey) {
		t.Fatalf("Complete error leaked APIKey: %s", completeErr.Error())
	}
}

func TestClient_Boundary_BodySlowerThanTimeoutIsUnavailable(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-litellm-applied-policies", "example-policy")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer server.Close()
	defer close(done)

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
		Policy:  "example-policy",
		Timeout: 50 * time.Millisecond,
	}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	probeErr := client.Probe(context.Background())
	if probeErr == nil {
		t.Fatal("Probe() want an error for a body that never ends, got nil")
	}
	if got := Classify(probeErr); got != ClassUnavailable {
		t.Errorf("Classify(Probe error %v) = %v, want ClassUnavailable", probeErr, got)
	}

	_, completeErr := client.Complete(context.Background(), "sys", "usr")
	if completeErr == nil {
		t.Fatal("Complete() want an error for a body that never ends, got nil")
	}
	if got := Classify(completeErr); got != ClassUnavailable {
		t.Errorf("Classify(Complete error %v) = %v, want ClassUnavailable", completeErr, got)
	}
}

func TestProbe_Boundary_DeadlineNeverAboveProbeTimeout(t *testing.T) {
	var left time.Duration
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			return nil, errors.New("request carries no deadline")
		}
		left = time.Until(deadline)
		return nil, errors.New("stop after reading the deadline")
	})}

	c, err := New(Config{
		BaseURL: "http://127.0.0.1:1",
		Alias:   "test-alias",
		Policy:  "example-policy",
		Timeout: MaxTimeout,
	}, httpClient)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if MaxTimeout <= ProbeTimeout {
		t.Fatalf("MaxTimeout %v must be above ProbeTimeout %v for this case", MaxTimeout, ProbeTimeout)
	}

	if probeErr := c.Probe(context.Background()); probeErr == nil {
		t.Fatal("Probe() want the transport error, got nil")
	}
	if left <= 0 || left > ProbeTimeout {
		t.Errorf("probe deadline lies %v ahead, want above 0 and at most %v", left, ProbeTimeout)
	}
}

func runPolicyServer(headers http.Header, status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, vv := range headers {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			http.Error(w, "write error", http.StatusInternalServerError)
		}
	}))
}

func TestComplete_Positive_PolicyValidationTable(t *testing.T) {
	const validBody = `{"choices":[{"message":{"content":"ok"}}]}`
	tests := []struct {
		name    string
		headers http.Header
	}{
		{"name alone", http.Header{PolicyHeader: []string{"example-policy"}}},
		{"name first of three", http.Header{PolicyHeader: []string{"example-policy,other1,other2"}}},
		{"name middle of three", http.Header{PolicyHeader: []string{"other1,example-policy,other2"}}},
		{"name last of three", http.Header{PolicyHeader: []string{"other1,other2,example-policy"}}},
		{"spaces around names", http.Header{PolicyHeader: []string{"  other1  ,  example-policy  ,  other2  "}}},
		{"two header lines name in second", http.Header{PolicyHeader: []string{"other1", "example-policy"}}},
		{"empty entries", http.Header{PolicyHeader: []string{",,example-policy,,other,,"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := runPolicyServer(tc.headers, http.StatusOK, validBody)
			defer server.Close()

			c, err := New(Config{BaseURL: server.URL, Alias: "test-alias", Policy: "example-policy"}, server.Client())
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			out, err := c.Complete(context.Background(), "sys", "usr")
			if err != nil {
				t.Fatalf("Complete() failed: %v", err)
			}
			if out != "ok" {
				t.Errorf("out = %q, want %q", out, "ok")
			}
		})
	}
}

func build65NamesHeader() http.Header {
	names := make([]string, 64)
	for i := range names {
		names[i] = fmt.Sprintf("other-%d", i)
	}
	all := append(names, "example-policy")
	return http.Header{PolicyHeader: []string{strings.Join(all, ",")}}
}

// verifyCompleteNegativePolicyCase checks that Complete returns a sanitized PolicyError for missing/invalid policy headers.
func verifyCompleteNegativePolicyCase(t *testing.T, headers http.Header, secretBody string) {
	t.Helper()
	server := runPolicyServer(headers, http.StatusOK, secretBody)
	defer server.Close()

	c, err := New(Config{BaseURL: server.URL, Alias: "test-alias", Policy: "example-policy"}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	out, err := c.Complete(context.Background(), "sys", "usr")
	if err == nil {
		t.Fatal("Complete() want PolicyError, got nil")
	}
	if out != "" {
		t.Errorf("Complete() out = %q, want empty", out)
	}
	var polErr *PolicyError
	if !errors.As(err, &polErr) {
		t.Fatalf("err type %T, want *PolicyError", err)
	}
	if Classify(err) != ClassPolicy {
		t.Errorf("Classify(%v) = %v, want ClassPolicy", err, Classify(err))
	}
	if strings.Contains(err.Error(), "secret-model-reply") {
		t.Errorf("error %q leaked body", err.Error())
	}
	if strings.Contains(err.Error(), "example-policy") || strings.Contains(err.Error(), "other1") {
		t.Errorf("error %q leaked policy header names", err.Error())
	}
}

func TestComplete_Negative_PolicyValidationTable(t *testing.T) {
	const secretBody = `{"choices":[{"message":{"content":"secret-model-reply"}}]}`
	tests := []struct {
		name    string
		headers http.Header
	}{
		{"header absent", http.Header{}},
		{"header without name", http.Header{PolicyHeader: []string{"other1,other2"}}},
		{"name differs only in case", http.Header{PolicyHeader: []string{"Example-Policy"}}},
		{"name prefix", http.Header{PolicyHeader: []string{"example"}}},
		{"name suffix", http.Header{PolicyHeader: []string{"policy"}}},
		{"65 names with wanted at 65", build65NamesHeader()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verifyCompleteNegativePolicyCase(t, tc.headers, secretBody)
		})
	}
}

func TestComplete_Negative_Status500WithHeaderReturnsStatusError(t *testing.T) {
	server := runPolicyServer(http.Header{PolicyHeader: []string{"example-policy"}}, http.StatusInternalServerError, "error")
	defer server.Close()

	c, err := New(Config{BaseURL: server.URL, Alias: "test-alias", Policy: "example-policy"}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	_, err = c.Complete(context.Background(), "sys", "usr")
	if err == nil {
		t.Fatal("Complete() want error, got nil")
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("err %T, want *StatusError", err)
	}
	if statusErr.Code != 500 {
		t.Errorf("status code = %d, want 500", statusErr.Code)
	}
	if Classify(err) != ClassUnavailable {
		t.Errorf("Classify = %v, want ClassUnavailable", Classify(err))
	}
}

func TestProbe_Positive_PolicyValidationTable(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
	}{
		{"name alone", http.Header{PolicyHeader: []string{"example-policy"}}},
		{"two lines name in second", http.Header{PolicyHeader: []string{"other", "example-policy"}}},
		{"spaces around name", http.Header{PolicyHeader: []string{"  example-policy  , other"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := runPolicyServer(tc.headers, http.StatusOK, "pong")
			defer server.Close()

			c, err := New(Config{BaseURL: server.URL, Alias: "test-alias", Policy: "example-policy"}, server.Client())
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			if err := c.Probe(context.Background()); err != nil {
				t.Fatalf("Probe() unexpected error: %v", err)
			}
		})
	}
}

// verifyProbeNegativePolicyCase checks that Probe returns a sanitized PolicyError for missing/invalid policy headers.
func verifyProbeNegativePolicyCase(t *testing.T, headers http.Header, secretBody string) {
	t.Helper()
	server := runPolicyServer(headers, http.StatusOK, secretBody)
	defer server.Close()

	c, err := New(Config{BaseURL: server.URL, Alias: "test-alias", Policy: "example-policy"}, server.Client())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	err = c.Probe(context.Background())
	if err == nil {
		t.Fatal("Probe() want PolicyError, got nil")
	}
	var polErr *PolicyError
	if !errors.As(err, &polErr) {
		t.Fatalf("err type %T, want *PolicyError", err)
	}
	if Classify(err) != ClassPolicy {
		t.Errorf("Classify(%v) = %v, want ClassPolicy", err, Classify(err))
	}
	if strings.Contains(err.Error(), secretBody) {
		t.Errorf("error %q leaked body", err.Error())
	}
	if strings.Contains(err.Error(), "example-policy") || strings.Contains(err.Error(), "other") {
		t.Errorf("error %q leaked policy names", err.Error())
	}
}

func TestProbe_Negative_PolicyValidationTable(t *testing.T) {
	const secretBody = "secret-probe-reply"
	tests := []struct {
		name    string
		headers http.Header
	}{
		{"header absent", http.Header{}},
		{"header without name", http.Header{PolicyHeader: []string{"other"}}},
		{"case mismatch", http.Header{PolicyHeader: []string{"Example-Policy"}}},
		{"65 names with wanted at 65", build65NamesHeader()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			verifyProbeNegativePolicyCase(t, tc.headers, secretBody)
		})
	}
}
