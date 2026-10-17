package embed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOnPremises(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// Positive: (a) Loopback or private IP
		{"ipv4 loopback", "http://127.0.0.1:11434/v1", false},
		{"ipv4 private 10/8", "http://10.0.0.5:8000/v1", false},
		{"ipv4 private 172.16/12", "http://172.16.1.1:8000/v1", false},
		{"ipv4 private 192.168/16", "http://192.168.1.100/v1", false},
		{"ipv6 loopback", "http://[::1]:11434/v1", false},
		{"ipv6 private fc00", "http://[fd00::1]:8000/v1", false},
		// Positive: (b) localhost
		{"localhost lower", "http://localhost:8000/v1", false},
		{"localhost upper", "HTTP://LOCALHOST/v1", false},
		// Positive: (c) single-label host
		{"single label gateway", "http://gateway/v1", false},
		{"single label router", "http://router:8080/v1", false},
		// Positive: (d) Allowed suffixes
		{"suffix .svc", "http://litellm.cordana.svc:4000/v1", false},
		{"suffix .cluster.local", "http://service.namespace.svc.cluster.local/v1", false},
		{"suffix .internal", "http://ollama.internal:11434/v1", false},
		{"suffix .home.arpa", "http://gateway.home.arpa/v1", false},
		{"suffix .lan", "http://box.lan:8000/v1", false},
		{"trailing dot stripped", "http://gateway.internal./v1", false},

		// Negative: Hosted endpoints
		{"openai", "https://api.openai.com/v1", true},
		{"public domain", "https://gateway.example.org/v1", true},
		{"public ip 8.8.8.8", "http://8.8.8.8/v1", true},
		{"public domain with trailing dot", "https://embeddings.example.com.", true},
		{"suffix trick localhost.example.com", "http://localhost.example.com/v1", true},
		{"suffix trick evil.svc.example.com", "http://evil.svc.example.com/v1", true},
		{"invalid url", ":::invalid:::", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := OnPremises(tc.url)
			if (err != nil) != tc.wantErr {
				t.Fatalf("OnPremises(%q) err = %v, wantErr %v", tc.url, err, tc.wantErr)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "ADR-0022") && !strings.Contains(err.Error(), "parse") {
				t.Errorf("error %q should mention ADR-0022", err.Error())
			}
		})
	}
}

func TestEmbed_Positive_SuccessfulEmbedAndReorder(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer secret-key" {
			t.Errorf("auth header = %q, want Bearer secret-key", auth)
		}
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(req.Input) != 2 {
			t.Errorf("got %d input texts, want 2", len(req.Input))
		}
		// Return in reverse index order to verify sorting
		resp := embedResponse{
			Data: []embedItem{
				{Index: 1, Embedding: []float32{0.3, 0.4}},
				{Index: 0, Embedding: []float32{0.1, 0.2}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client, err := New(Config{
		BaseURL: ts.URL,
		Alias:   "test-alias",
		APIKey:  "secret-key",
	}, ts.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := client.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("got %d vectors, want 2", len(res))
	}
	if res[0][0] != 0.1 || res[0][1] != 0.2 {
		t.Errorf("res[0] = %v, want [0.1, 0.2]", res[0])
	}
	if res[1][0] != 0.3 || res[1][1] != 0.4 {
		t.Errorf("res[1] = %v, want [0.3, 0.4]", res[1])
	}
}

func TestEmbed_Negative_Status500AndKeyNotInError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret sensitive server error body", http.StatusInternalServerError)
	}))
	defer ts.Close()

	key := "sensitive-embed-key-xyz"
	client, err := New(Config{
		BaseURL: ts.URL,
		Alias:   "test-alias",
		APIKey:  key,
	}, ts.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for status 500, got nil")
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.Code != 500 {
		t.Errorf("err = %v, want StatusError 500", err)
	}
	if strings.Contains(err.Error(), key) {
		t.Errorf("error contains API key: %v", err)
	}
	if strings.Contains(err.Error(), "sensitive server error body") {
		t.Errorf("error contains server body: %v", err)
	}
}

func TestEmbed_Negative_RedirectRefused(t *testing.T) {
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusMovedPermanently)
	}))
	defer ts.Close()

	client, err := New(Config{
		BaseURL: ts.URL,
		Alias:   "test-alias",
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error when redirect returned, got nil")
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusMovedPermanently {
		t.Errorf("err = %v, want StatusError 301", err)
	}
}

func TestEmbed_Negative_MalformedResponses(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{
			name: "wrong count",
			handler: func(w http.ResponseWriter, r *http.Request) {
				resp := embedResponse{Data: []embedItem{{Index: 0, Embedding: []float32{0.1}}}}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: "expected 2 embeddings, got 1",
		},
		{
			name: "mixed dimensions",
			handler: func(w http.ResponseWriter, r *http.Request) {
				resp := embedResponse{Data: []embedItem{
					{Index: 0, Embedding: []float32{0.1, 0.2}},
					{Index: 1, Embedding: []float32{0.1}},
				}}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: "mixed vector dimensions",
		},
		{
			name: "zero dimension",
			handler: func(w http.ResponseWriter, r *http.Request) {
				resp := embedResponse{Data: []embedItem{
					{Index: 0, Embedding: []float32{}},
					{Index: 1, Embedding: []float32{}},
				}}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: "vector dimension is zero",
		},
		{
			name: "duplicate index",
			handler: func(w http.ResponseWriter, r *http.Request) {
				resp := embedResponse{Data: []embedItem{
					{Index: 0, Embedding: []float32{0.1}},
					{Index: 0, Embedding: []float32{0.2}},
				}}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: "duplicate embedding index",
		},
		{
			name: "out of bounds index",
			handler: func(w http.ResponseWriter, r *http.Request) {
				resp := embedResponse{Data: []embedItem{
					{Index: 0, Embedding: []float32{0.1}},
					{Index: 5, Embedding: []float32{0.2}},
				}}
				_ = json.NewEncoder(w).Encode(resp)
			},
			wantErr: "outside 0..1",
		},
		{
			name: "oversized body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// Write more than MaxResponseBytes
				w.Write(make([]byte, MaxResponseBytes+10))
			},
			wantErr: "exceeds max",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			defer ts.Close()

			client, err := New(Config{
				BaseURL: ts.URL,
				Alias:   "test-alias",
			}, ts.Client())
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			_, err = client.Embed(context.Background(), []string{"one", "two"})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Embed error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestEmbed_Boundary_BatchAndTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client, err := New(Config{
		BaseURL: ts.URL,
		Alias:   "test-alias",
		Timeout: 20 * time.Millisecond,
	}, ts.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = client.Embed(ctx, []string{"test"})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Empty texts
	if _, err := client.Embed(context.Background(), nil); err == nil {
		t.Error("Embed with nil texts returned nil error")
	}

	// MaxBatch boundary
	oversized := make([]string, MaxBatch+1)
	if _, err := client.Embed(context.Background(), oversized); err == nil {
		t.Error("Embed with 65 texts returned nil error")
	}
}
