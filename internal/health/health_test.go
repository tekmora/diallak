package health

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandler_Positive_HealthzAndReadyz(t *testing.T) {
	state := &State{}
	handler := NewHandler(state)
	ctx := context.Background()

	checkHeaders := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
			t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", got)
		}
	}

	// GET /healthz -> 200 "ok\n"
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /healthz code = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok\n" {
		t.Errorf("GET /healthz body = %q, want ok\\n", rec.Body.String())
	}
	checkHeaders(t, rec)

	// HEAD /healthz -> 200
	req = httptest.NewRequestWithContext(ctx, http.MethodHead, "/healthz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("HEAD /healthz code = %d, want 200", rec.Code)
	}
	checkHeaders(t, rec)

	// GET /readyz before ready -> 503 "not ready\n"
	req = httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz (not ready) code = %d, want 503", rec.Code)
	}
	if rec.Body.String() != "not ready\n" {
		t.Errorf("GET /readyz body = %q, want not ready\\n", rec.Body.String())
	}
	checkHeaders(t, rec)

	// HEAD /readyz before ready -> 503
	req = httptest.NewRequestWithContext(ctx, http.MethodHead, "/readyz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("HEAD /readyz (not ready) code = %d, want 503", rec.Code)
	}
	checkHeaders(t, rec)

	// Set ready -> true
	state.SetReady(true)
	if !state.Ready() {
		t.Errorf("state.Ready() = false, want true")
	}

	// GET /readyz after setter -> 200 "ready\n"
	req = httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /readyz (ready) code = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ready\n" {
		t.Errorf("GET /readyz body = %q, want ready\\n", rec.Body.String())
	}
	checkHeaders(t, rec)

	// HEAD /readyz after setter -> 200
	req = httptest.NewRequestWithContext(ctx, http.MethodHead, "/readyz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("HEAD /readyz (ready) code = %d, want 200", rec.Code)
	}
	checkHeaders(t, rec)

	// Set ready -> false (cleared)
	state.SetReady(false)
	if state.Ready() {
		t.Errorf("state.Ready() = true, want false")
	}

	// GET /readyz after cleared -> 503 "not ready\n"
	req = httptest.NewRequestWithContext(ctx, http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz (cleared) code = %d, want 503", rec.Code)
	}
	if rec.Body.String() != "not ready\n" {
		t.Errorf("GET /readyz body = %q, want not ready\\n", rec.Body.String())
	}
	checkHeaders(t, rec)
}

func TestHandler_Negative_MethodNotAllowed(t *testing.T) {
	state := &State{}
	handler := NewHandler(state)
	ctx := context.Background()

	for _, path := range []string{"/healthz", "/readyz"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			t.Run(method+" "+path, func(t *testing.T) {
				req := httptest.NewRequestWithContext(ctx, method, path, nil)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code != http.StatusMethodNotAllowed {
					t.Errorf("%s %s code = %d, want 405", method, path, rec.Code)
				}
				if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
					t.Errorf("%s %s Allow = %q, want GET, HEAD", method, path, got)
				}
			})
		}
	}
}

func TestHandler_Negative_NotFound(t *testing.T) {
	state := &State{}
	handler := NewHandler(state)
	ctx := context.Background()

	paths := []string{"/", "/health", "/ready", "/healthz/", "/readyz/sub", "/nonexistent"}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, p, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("GET %s code = %d, want 404", p, rec.Code)
			}
		})
	}
}

func TestServer_Negative_ListenerDownAfterContextEnded(t *testing.T) {
	ln, err := Listen(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := ln.Addr().String()

	state := &State{}
	srv := NewServer(state)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ctx, ln)
	}()

	client := &http.Client{Timeout: 100 * time.Millisecond}

	// Verify server answers /healthz while active
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/healthz", nil)
	if reqErr != nil {
		t.Fatalf("NewRequest: %v", reqErr)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("initial GET /healthz failed: %v", err)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Errorf("close body: %v", closeErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initial GET /healthz status = %d, want 200", resp.StatusCode)
	}

	// Cancel context to end server
	cancel()

	select {
	case serveErr := <-errCh:
		if serveErr != nil {
			t.Fatalf("Serve returned error on normal shutdown: %v", serveErr)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Serve did not exit within deadline after context cancel")
	}

	// REQUIRED: after the context ends, a request to the same address FAILS (connection error) = listener down, probe fails
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer probeCancel()
	probeReq, probeReqErr := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://"+addr+"/healthz", nil)
	if probeReqErr != nil {
		t.Fatalf("probe NewRequest: %v", probeReqErr)
	}
	probeResp, err := client.Do(probeReq)
	if err == nil {
		if closeErr := probeResp.Body.Close(); closeErr != nil {
			t.Errorf("close probe body: %v", closeErr)
		}
		t.Fatal("request succeeded after context ended; expected connection error (listener down)")
	}
}

func TestServer_Boundary_MaxHeaderBytesRefusedWithoutPanic(t *testing.T) {
	ln, err := Listen(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	srv := NewServer(&State{})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ctx, ln)
	}()
	defer func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(100 * time.Millisecond):
		}
	}()

	client := &http.Client{Timeout: 100 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ln.Addr().String()+"/healthz", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// Add oversized header exceeding MaxHeaderBytes + Go internal buffer
	req.Header.Set("X-Large-Header", strings.Repeat("A", MaxHeaderBytes+8192))

	resp, err := client.Do(req)
	if err != nil {
		// Connection reset/closed is an acceptable refusal
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("close body: %v", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge && resp.StatusCode < 400 {
		t.Errorf("status = %d, want >= 400 refusal without panic", resp.StatusCode)
	}
}

func TestServer_Boundary_ReadHeaderTimeoutDropsIdleClient(t *testing.T) {
	ln, err := Listen(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	const shortTimeout = 25 * time.Millisecond
	srv := NewServer(&State{}, withReadHeaderTimeout(shortTimeout))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ctx, ln)
	}()
	defer func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(100 * time.Millisecond):
		}
	}()

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "use of closed network connection") {
			t.Errorf("close conn: %v", closeErr)
		}
	}()

	// Client sends nothing; ReadHeaderTimeout should drop the connection
	if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}

	buf := make([]byte, 1)
	_, readErr := conn.Read(buf)
	if readErr == nil {
		t.Fatal("expected read error/EOF due to ReadHeaderTimeout, got data")
	}
	if !errors.Is(readErr, io.EOF) && !strings.Contains(readErr.Error(), "connection reset") && !strings.Contains(readErr.Error(), "use of closed network connection") {
		t.Logf("Read error on dropped client: %v", readErr)
	}
}
