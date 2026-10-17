//go:build !race

// These tests run the bot's real entry with the real disgo client against a
// fake Discord on 127.0.0.1. disgo v0.19.6 writes gatewayImpl.heartbeatCancel
// in its heartbeat goroutine (gateway/gateway.go:451) and reads it in
// CloseWithCode (gateway/gateway.go:317) without a lock, so the race detector
// reports the library on every shutdown. The race build therefore leaves this
// file out, and the CI step "Gateway health tests" runs it without -race.

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/klauspost/compress/zstd"

	"github.com/tekmora/diallak/internal/config"
)

const (
	testAppID   uint64 = 123456789012345678
	testGuildID uint64 = 987654321098765432
)

func makeFakeToken(id uint64) string {
	seg := base64.RawStdEncoding.EncodeToString([]byte(strconv.FormatUint(id, 10)))
	return seg + ".synthetic-test-token.not-a-real-credential"
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func redirectDiscord(t *testing.T, fakeURL string) {
	t.Helper()
	target, err := url.Parse(fakeURL)
	if err != nil {
		t.Fatalf("parse fake rest url %q: %v", fakeURL, err)
	}
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		path, ok := strings.CutPrefix(r.URL.Path, "/api/v10/")
		if r.URL.Scheme != "https" || r.URL.Host != "discord.com" || !ok {
			return nil, fmt.Errorf("request for %s, want https://discord.com/api/v10", r.URL.Redacted())
		}
		out := r.Clone(r.Context())
		out.URL.Scheme = target.Scheme
		out.URL.Host = target.Host
		out.URL.Path = "/" + path
		out.URL.RawPath = ""
		out.Host = ""
		return orig.RoundTrip(out)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
}

func allocateFreePort(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ephemeral: %v", err)
	}
	addr := ln.Addr().String()
	if closeErr := ln.Close(); closeErr != nil {
		t.Fatalf("close ephemeral: %v", closeErr)
	}
	return addr
}

func createWSSender(conn *websocket.Conn, useZstd bool) (func([]byte) error, func() error, error) {
	// The READY sender and the heartbeat answers write from two goroutines.
	var mu sync.Mutex
	if !useZstd {
		return func(p []byte) error {
			mu.Lock()
			defer mu.Unlock()
			return conn.WriteMessage(websocket.TextMessage, p)
		}, func() error { return nil }, nil
	}

	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf)
	if err != nil {
		return nil, nil, fmt.Errorf("new zstd writer: %w", err)
	}

	send := func(p []byte) error {
		mu.Lock()
		defer mu.Unlock()
		buf.Reset()
		if _, wErr := enc.Write(p); wErr != nil {
			return wErr
		}
		if fErr := enc.Flush(); fErr != nil {
			return fErr
		}
		return conn.WriteMessage(websocket.BinaryMessage, buf.Bytes())
	}
	return send, enc.Close, nil
}

func sendHelloAndReadIdentify(t *testing.T, conn *websocket.Conn, send func([]byte) error) error {
	t.Helper()
	hello := []byte(`{"op":10,"d":{"heartbeat_interval":45000}}`)
	if err := send(hello); err != nil {
		t.Errorf("send hello: %v", err)
		return err
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Errorf("read identify: %v", err)
		return err
	}

	var ident struct {
		Op int `json:"op"`
	}
	if err := json.Unmarshal(msg, &ident); err != nil {
		t.Errorf("unmarshal identify: %v", err)
		return err
	}
	if ident.Op != 2 {
		t.Errorf("expected identify op 2, got %d", ident.Op)
		return fmt.Errorf("expected identify op 2, got %d", ident.Op)
	}
	return nil
}

func sendReadyFrame(send func([]byte) error, appID uint64, wsURL string) error {
	idStr := strconv.FormatUint(appID, 10)
	payload := fmt.Sprintf(
		`{"op":0,"t":"READY","s":1,"d":{"v":10,"user":{"id":"%s","username":"diallak","discriminator":"0","bot":true},"guilds":[],"session_id":"s","resume_gateway_url":"%s","application":{"id":"%s","flags":0}}}`,
		idStr, wsURL, idStr,
	)
	return send([]byte(payload))
}

func handleClientFrames(conn *websocket.Conn, send func([]byte) error, done chan<- struct{}) {
	defer close(done)
	const maxFrames = 5000
	for i := 0; i < maxFrames; i++ {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var frame struct {
			Op int `json:"op"`
		}
		if err := json.Unmarshal(msg, &frame); err == nil && frame.Op == 1 {
			if sErr := send([]byte(`{"op":11}`)); sErr != nil {
				return
			}
		}
	}
}

func handleWSConnection(
	t *testing.T,
	conn *websocket.Conn,
	useZstd bool,
	readyGate <-chan struct{},
	appID uint64,
	wsURL string,
) {
	defer func() {
		if cErr := conn.Close(); cErr != nil {
			t.Logf("close ws connection: %v", cErr)
		}
	}()

	send, closeEnc, err := createWSSender(conn, useZstd)
	if err != nil {
		t.Errorf("create ws sender: %v", err)
		return
	}
	defer func() {
		if encErr := closeEnc(); encErr != nil {
			t.Logf("close zstd encoder: %v", encErr)
		}
	}()

	if err := sendHelloAndReadIdentify(t, conn, send); err != nil {
		return
	}

	clientDone := make(chan struct{})
	go handleClientFrames(conn, send, clientDone)

	select {
	case <-readyGate:
		if err := sendReadyFrame(send, appID, wsURL); err != nil {
			t.Errorf("send ready: %v", err)
			return
		}
	case <-clientDone:
		return
	}

	<-clientDone
}

func startFakeGateway(t *testing.T, readyGate <-chan struct{}, appID uint64) (*httptest.Server, string) {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake ws: %v", err)
	}
	wsURL := "ws://" + ln.Addr().String()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		useZstd := r.URL.Query().Get("compress") == "zstd-stream"
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("ws upgrade: %v", err)
			return
		}
		handleWSConnection(t, conn, useZstd, readyGate, appID, wsURL)
	})

	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, wsURL
}

func startFakeREST(t *testing.T, appID uint64, wsURL string) *httptest.Server {
	t.Helper()
	idStr := strconv.FormatUint(appID, 10)
	// Each path the bot reads at start, with the body the fake answers. The areas command reads
	// the server's roles once before the gateway opens; the moderation commands read the bot user,
	// the roles and the channels.
	bodies := map[string]string{
		"/applications/@me": fmt.Sprintf(`{"id":"%s","name":"diallak","bot":{"id":"%s","username":"diallak"}}`, idStr, idStr),
		"/guilds/" + strconv.FormatUint(testGuildID, 10) + "/channels": `[]`,
		"/gateway":     fmt.Sprintf(`{"url":"%s","shards":1}`, wsURL),
		"/gateway/bot": fmt.Sprintf(`{"url":"%s","shards":1}`, wsURL),
		"/guilds/" + strconv.FormatUint(testGuildID, 10) + "/roles": fmt.Sprintf(
			`[{"id":"%d","name":"@everyone","position":0}]`, testGuildID),
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			t.Errorf("unexpected REST path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s for %s", r.Method, r.URL.Path)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, body); err != nil {
			t.Errorf("write %s: %v", r.URL.Path, err)
		}
	})

	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake rest: %v", err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func setupFakeDiscord(t *testing.T, readyGate <-chan struct{}, appID uint64) {
	t.Helper()
	_, wsURL := startFakeGateway(t, readyGate, appID)
	restSrv := startFakeREST(t, appID, wsURL)
	redirectDiscord(t, restSrv.URL)
}

func setupBotEnv(t *testing.T, appID, guildID uint64, healthAddr string) func(string) string {
	t.Helper()
	token := makeFakeToken(appID)
	tokenPath := writeSecret(t, "token", token)
	corpusDir := setupTestCorpus(t)
	envMap := map[string]string{
		config.EnvTokenFile:  tokenPath,
		config.EnvGuildID:    strconv.FormatUint(guildID, 10),
		config.EnvCorpusDir:  corpusDir,
		config.EnvHealthAddr: healthAddr,
	}
	return func(k string) string { return envMap[k] }
}

func newHealthClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
		Timeout: 1 * time.Second,
	}
}

func pollHealthz(ctx context.Context, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := newHealthClient()
	const maxTries = 1000
	for i := 0; i < maxTries; i++ {
		if time.Now().After(deadline) {
			return errors.New("timeout waiting for /healthz")
		}
		reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://"+addr+"/healthz", nil)
		if err != nil {
			cancel()
			return err
		}
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			body, rErr := io.ReadAll(resp.Body)
			if closeErr := resp.Body.Close(); closeErr != nil {
				return closeErr
			}
			if rErr == nil && resp.StatusCode == http.StatusOK && string(body) == "ok\n" {
				return nil
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("max tries exceeded waiting for /healthz")
}

func pollReadyz(ctx context.Context, addr string, wantStatus int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := newHealthClient()
	const maxTries = 1000
	for i := 0; i < maxTries; i++ {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for /readyz status %d", wantStatus)
		}
		reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://"+addr+"/readyz", nil)
		if err != nil {
			cancel()
			return err
		}
		resp, err := client.Do(req)
		cancel()
		if err == nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				return closeErr
			}
			if resp.StatusCode == wantStatus {
				return nil
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("max tries exceeded waiting for /readyz status %d", wantStatus)
}

func probeReadyzSingle(ctx context.Context, addr string) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://"+addr+"/readyz", nil)
	if err != nil {
		return 0, err
	}
	client := newHealthClient()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		return resp.StatusCode, closeErr
	}
	return resp.StatusCode, nil
}

func assertRunReturns(t *testing.T, runErrCh <-chan error, timeout time.Duration) {
	t.Helper()
	select {
	case <-runErrCh:
	case <-time.After(timeout):
		t.Fatalf("run did not return within %v", timeout)
	}
}

func assertRunReturnsNil(t *testing.T, runErrCh <-chan error, timeout time.Duration) {
	t.Helper()
	select {
	case err := <-runErrCh:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(timeout):
		t.Fatalf("run did not return within %v", timeout)
	}
}

func assertPortRefused(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	dialer := &net.Dialer{Timeout: 50 * time.Millisecond}
	const maxTries = 100
	for i := 0; i < maxTries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		cancel()
		if err != nil {
			return
		}
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("close probe conn: %v", closeErr)
		}
		if time.Now().After(deadline) {
			t.Fatalf("health listener port %s still accepts connections", addr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRun_Positive_ReadyzFollowsTheGatewayOverTheRealEntry(t *testing.T) {
	healthAddr := allocateFreePort(t)
	readyGate := make(chan struct{})
	setupFakeDiscord(t, readyGate, testAppID)
	getenv := setupBotEnv(t, testAppID, testGuildID, healthAddr)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- run(runCtx, nil, getenv, io.Discard, defaultDependencies())
	}()

	if err := pollHealthz(runCtx, healthAddr, 10*time.Second); err != nil {
		t.Fatalf("poll /healthz: %v", err)
	}

	code, err := probeReadyzSingle(runCtx, healthAddr)
	if err != nil {
		t.Fatalf("probe /readyz before ready release: %v", err)
	}
	if code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz before ready release = %d, want 503", code)
	}

	close(readyGate)

	if err := pollReadyz(runCtx, healthAddr, http.StatusOK, 10*time.Second); err != nil {
		t.Fatalf("poll /readyz ready: %v", err)
	}

	runCancel()
	assertRunReturnsNil(t, runErrCh, 10*time.Second)
}

func TestRun_Negative_ReadyzStaysUnreadyWhenTheGatewayNeverSendsReady(t *testing.T) {
	healthAddr := allocateFreePort(t)
	readyGate := make(chan struct{})
	setupFakeDiscord(t, readyGate, testAppID)
	getenv := setupBotEnv(t, testAppID, testGuildID, healthAddr)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- run(runCtx, nil, getenv, io.Discard, defaultDependencies())
	}()

	if err := pollHealthz(runCtx, healthAddr, 10*time.Second); err != nil {
		t.Fatalf("poll /healthz: %v", err)
	}

	const checks = 5
	for i := 0; i < checks; i++ {
		code, err := probeReadyzSingle(runCtx, healthAddr)
		if err != nil {
			t.Fatalf("probe /readyz check %d: %v", i, err)
		}
		if code != http.StatusServiceUnavailable {
			t.Fatalf("/readyz check %d = %d, want 503", i, code)
		}
		time.Sleep(10 * time.Millisecond)
	}

	runCancel()
	assertRunReturns(t, runErrCh, 10*time.Second)
	assertPortRefused(t, healthAddr, 2*time.Second)
}

func TestRun_Boundary_ReadyzTurnsUnreadyAtShutdown(t *testing.T) {
	healthAddr := allocateFreePort(t)
	readyGate := make(chan struct{})
	setupFakeDiscord(t, readyGate, testAppID)
	getenv := setupBotEnv(t, testAppID, testGuildID, healthAddr)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- run(runCtx, nil, getenv, io.Discard, defaultDependencies())
	}()

	if err := pollHealthz(runCtx, healthAddr, 10*time.Second); err != nil {
		t.Fatalf("poll /healthz: %v", err)
	}

	close(readyGate)

	if err := pollReadyz(runCtx, healthAddr, http.StatusOK, 10*time.Second); err != nil {
		t.Fatalf("poll /readyz ready: %v", err)
	}

	runCancel()

	// The listener closes within microseconds of the cancel, so a probe here
	// mostly sees a refused connection; any answer must be 503.
	const maxProbes = 20
	for i := 0; i < maxProbes; i++ {
		code, pErr := probeReadyzSingle(context.Background(), healthAddr)
		if pErr == nil && code != http.StatusServiceUnavailable {
			t.Fatalf("probe /readyz returned %d during shutdown, want 503 or refused", code)
		}
		time.Sleep(5 * time.Millisecond)
	}

	assertRunReturnsNil(t, runErrCh, 10*time.Second)

	for i := 0; i < maxProbes; i++ {
		code, pErr := probeReadyzSingle(context.Background(), healthAddr)
		if pErr == nil && code == http.StatusOK {
			t.Fatalf("probe /readyz returned 200 after run returned")
		}
		time.Sleep(5 * time.Millisecond)
	}

	assertPortRefused(t, healthAddr, 2*time.Second)
}
