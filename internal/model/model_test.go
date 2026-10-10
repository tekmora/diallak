package model

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
			c, err := New(tc.cfg, nil)
			if err != nil {
				t.Fatalf("New() unexpected error: %v", err)
			}
			if c.baseURL != tc.wantURL {
				t.Errorf("baseURL = %q, want %q", c.baseURL, tc.wantURL)
			}
			if c.timeout != tc.wantTimeout {
				t.Errorf("timeout = %v, want %v", c.timeout, tc.wantTimeout)
			}
			if c.maxTokens != tc.wantTokens {
				t.Errorf("maxTokens = %d, want %d", c.maxTokens, tc.wantTokens)
			}
		})
	}
}

func TestNew_Negative_ValidationErrors(t *testing.T) {
	const secretKey = "super-secret-key-12345"
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "empty base URL",
			cfg:     Config{BaseURL: "", Alias: "router/test", APIKey: secretKey},
			wantErr: "unsupported base URL scheme",
		},
		{
			name:    "invalid scheme ftp",
			cfg:     Config{BaseURL: "ftp://localhost:8080", Alias: "router/test", APIKey: secretKey},
			wantErr: "unsupported base URL scheme",
		},
		{
			name:    "empty host",
			cfg:     Config{BaseURL: "http:///path", Alias: "router/test", APIKey: secretKey},
			wantErr: "base URL host is empty",
		},
		{
			name:    "contains userinfo",
			cfg:     Config{BaseURL: "http://user:pass@localhost:8080", Alias: "router/test", APIKey: secretKey},
			wantErr: "userinfo",
		},
		{
			name:    "contains query",
			cfg:     Config{BaseURL: "http://localhost:8080?foo=bar", Alias: "router/test", APIKey: secretKey},
			wantErr: "query",
		},
		{
			name:    "contains fragment",
			cfg:     Config{BaseURL: "http://localhost:8080#section", Alias: "router/test", APIKey: secretKey},
			wantErr: "fragment",
		},
		{
			name:    "empty alias",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "", APIKey: secretKey},
			wantErr: "invalid model alias",
		},
		{
			name:    "alias with invalid characters",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "bad alias!", APIKey: secretKey},
			wantErr: "invalid model alias",
		},
		{
			name:    "negative timeout",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", Timeout: -1 * time.Second, APIKey: secretKey},
			wantErr: "outside (0,",
		},
		{
			name:    "timeout exceeds maximum",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", Timeout: MaxTimeout + time.Second, APIKey: secretKey},
			wantErr: "outside (0,",
		},
		{
			name:    "negative max output tokens",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", MaxOutputTokens: -5, APIKey: secretKey},
			wantErr: "outside 1..4096",
		},
		{
			name:    "max output tokens exceeds limit",
			cfg:     Config{BaseURL: "http://localhost:8080", Alias: "router/test", MaxOutputTokens: 4097, APIKey: secretKey},
			wantErr: "outside 1..4096",
		},
	}

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

func TestNew_Boundary_Limits(t *testing.T) {
	baseCfg := Config{
		BaseURL: "http://localhost:8080",
		Alias:   "router/test",
	}

	// Alias boundary: exactly 128 chars vs 129 chars
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

	// MaxOutputTokens boundary: 1, 4096 vs 0, 4097
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

	// Timeout boundary: MaxTimeout vs MaxTimeout + 1ns
	cfgMaxTimeout := baseCfg
	cfgMaxTimeout.Timeout = MaxTimeout
	if c, err := New(cfgMaxTimeout, nil); err != nil || c.timeout != MaxTimeout {
		t.Errorf("New() with MaxTimeout failed: %v", err)
	}

	cfgOverTimeout := baseCfg
	cfgOverTimeout.Timeout = MaxTimeout + time.Nanosecond
	if _, err := New(cfgOverTimeout, nil); err == nil {
		t.Error("New() with MaxTimeout+1ns passed, want error")
	}
}

func TestComplete_Positive_RequestShapeAndResponse(t *testing.T) {
	var capturedReq chatRequest
	var capturedAuth string
	var capturedContentType string
	var capturedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuth = r.Header.Get("Authorization")
		capturedContentType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(body, &capturedReq); err != nil {
			http.Error(w, "unmarshal error", http.StatusBadRequest)
			return
		}

		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Role: "assistant", Content: "Simulated answer text."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL:         server.URL,
		Alias:           "router/test",
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

	if content != "Simulated answer text." {
		t.Errorf("content = %q, want %q", content, "Simulated answer text.")
	}
	if capturedPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", capturedPath)
	}
	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", capturedContentType)
	}
	if capturedAuth != "Bearer test-secret-key" {
		t.Errorf("Authorization = %q, want 'Bearer test-secret-key'", capturedAuth)
	}
	if capturedReq.Model != "router/test" {
		t.Errorf("model = %q, want 'router/test'", capturedReq.Model)
	}
	if capturedReq.MaxTokens != 256 {
		t.Errorf("max_tokens = %d, want 256", capturedReq.MaxTokens)
	}
	if capturedReq.Temperature != 0 {
		t.Errorf("temperature = %v, want 0", capturedReq.Temperature)
	}
	if capturedReq.Stream {
		t.Errorf("stream = %v, want false", capturedReq.Stream)
	}
	if len(capturedReq.Messages) != 2 ||
		capturedReq.Messages[0].Content != "System prompt" ||
		capturedReq.Messages[1].Content != "User prompt" {
		t.Errorf("messages = %+v, unexpected", capturedReq.Messages)
	}
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
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL,
		Alias:   "router/test",
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
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client, err := New(Config{
		BaseURL: server.URL + "///",
		Alias:   "router/test",
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
	client, err := New(Config{BaseURL: redirector.URL, Alias: "router/test", APIKey: "key-for-redirect-test"}, nil)
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
