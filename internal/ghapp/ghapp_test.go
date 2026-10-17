package ghapp

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
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
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)

func testRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return key
}

func writeResponse(t *testing.T, w http.ResponseWriter, status int, contentType, body string) {
	t.Helper()
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func verifyJWTHeader(t *testing.T, headB64 string) {
	t.Helper()
	headJSON, err := base64.RawURLEncoding.DecodeString(headB64)
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header jwtHeader
	if err := json.Unmarshal(headJSON, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header.Alg != "RS256" || header.Typ != "JWT" {
		t.Errorf("header = %+v, want RS256/JWT", header)
	}
}

func verifyJWTClaims(t *testing.T, claimB64, wantClientID string, wantNow time.Time) {
	t.Helper()
	claimJSON, err := base64.RawURLEncoding.DecodeString(claimB64)
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimJSON, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	wantIAT := wantNow.Add(-60 * time.Second).Unix()
	wantEXP := wantNow.Add(9 * time.Minute).Unix()
	if claims.IAT != wantIAT || claims.EXP != wantEXP || claims.ISS != wantClientID {
		t.Errorf("claims = %+v, want iat=%d exp=%d iss=%s", claims, wantIAT, wantEXP, wantClientID)
	}
}

func verifyJWTSignature(t *testing.T, pubKey *rsa.PublicKey, payload, sigB64 string) {
	t.Helper()
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	hash := sha256.Sum256([]byte(payload))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sig); err != nil {
		t.Errorf("verify signature: %v", err)
	}
}

func TestJWT_Positive_VerifiesWithPublicKey(t *testing.T) {
	key := testRSAKey(t)
	app := &App{ClientID: "test-client-id", Key: key, Now: func() time.Time { return testNow }}

	jwtStr, err := app.JWT(testNow)
	if err != nil {
		t.Fatalf("JWT() error = %v", err)
	}

	parts := strings.Split(jwtStr, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT parts = %d, want 3", len(parts))
	}

	verifyJWTHeader(t, parts[0])
	verifyJWTClaims(t, parts[1], "test-client-id", testNow)
	verifyJWTSignature(t, &key.PublicKey, parts[0]+"."+parts[1], parts[2])
}

func TestJWT_Negative_InvalidApp(t *testing.T) {
	key := testRSAKey(t)
	cases := []struct {
		name     string
		clientID string
		key      *rsa.PrivateKey
	}{
		{"nil key", "client-id", nil},
		{"empty client id", "", key},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &App{ClientID: tc.clientID, Key: tc.key}
			if _, err := app.JWT(testNow); err == nil {
				t.Errorf("%s: JWT() succeeded, want error", tc.name)
			}
		})
	}
}

func TestParseKey_Positive_PKCS1AndPKCS8(t *testing.T) {
	key := testRSAKey(t)

	pkcs1Bytes := x509.MarshalPKCS1PrivateKey(key)
	pkcs1PEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: pkcs1Bytes})

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS#8: %v", err)
	}
	pkcs8PEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})

	cases := []struct {
		name string
		data []byte
	}{
		{"PKCS1", pkcs1PEM},
		{"PKCS8", pkcs8PEM},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseKey(tc.data)
			if err != nil {
				t.Fatalf("%s: ParseKey() error = %v", tc.name, err)
			}
			if parsed.N.Cmp(key.N) != 0 || parsed.D.Cmp(key.D) != 0 {
				t.Errorf("%s: parsed key does not match original", tc.name)
			}
		})
	}
}

func TestParseKey_Negative_GarbageAndECRefused(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ec key: %v", err)
	}
	ecBytes, err := x509.MarshalPKCS8PrivateKey(ecKey)
	if err != nil {
		t.Fatalf("marshal ec PKCS#8: %v", err)
	}
	ecPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecBytes})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("fake cert")})

	cases := []struct {
		name string
		data []byte
	}{
		{"garbage text", []byte("not a pem file")},
		{"unsupported type", certPEM},
		{"EC key in PKCS8", ecPEM},
		{"corrupt PKCS1", pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("bad")})},
		{"corrupt PKCS8", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad")})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseKey(tc.data); err == nil {
				t.Errorf("%s: ParseKey() succeeded, want error", tc.name)
			}
		})
	}
}

func tokenServerHandler(t *testing.T, postCount *atomic.Int32, clockTime *time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/app/installations" {
			writeResponse(t, w, http.StatusOK, "application/json", `[{"id":42}]`)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/app/installations/42/access_tokens" {
			n := postCount.Add(1)
			expiry := clockTime.Add(time.Hour).Format(time.RFC3339)
			body := fmt.Sprintf(`{"token":"tok-%d","expires_at":%q}`, n, expiry)
			writeResponse(t, w, http.StatusCreated, "application/json", body)
			return
		}
		writeResponse(t, w, http.StatusNotFound, "", "")
	}
}

func TestToken_Positive_OneInstallationCachedAndRenewed(t *testing.T) {
	key := testRSAKey(t)
	var postCount atomic.Int32
	var clockTime = testNow

	srv := httptest.NewServer(tokenServerHandler(t, &postCount, &clockTime))
	defer srv.Close()

	app := &App{
		ClientID: "test-client",
		Key:      key,
		BaseURL:  srv.URL,
		HTTP:     srv.Client(),
		Now:      func() time.Time { return clockTime },
	}

	ctx := context.Background()
	tok1, err := app.Token(ctx)
	if err != nil || tok1 != "tok-1" {
		t.Fatalf("first Token() = %q, %v; want tok-1", tok1, err)
	}

	tok2, err := app.Token(ctx)
	if err != nil || tok2 != "tok-1" {
		t.Fatalf("cached Token() = %q, %v; want tok-1", tok2, err)
	}
	if got := postCount.Load(); got != 1 {
		t.Fatalf("POST count = %d, want 1 (cached)", got)
	}

	clockTime = clockTime.Add(56 * time.Minute)
	tok3, err := app.Token(ctx)
	if err != nil || tok3 != "tok-2" {
		t.Fatalf("renewed Token() = %q, %v; want tok-2", tok3, err)
	}
	if got := postCount.Load(); got != 2 {
		t.Fatalf("POST count = %d, want 2 (renewed)", got)
	}
}

func TestToken_Negative_ZeroOrMultipleInstallations(t *testing.T) {
	key := testRSAKey(t)
	cases := []struct {
		name string
		json string
	}{
		{"zero installations", `[]`},
		{"two installations", `[{"id":1},{"id":2}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeResponse(t, w, http.StatusOK, "application/json", tc.json)
			}))
			defer srv.Close()

			app := &App{
				ClientID: "client",
				Key:      key,
				BaseURL:  srv.URL,
				HTTP:     srv.Client(),
				Now:      func() time.Time { return testNow },
			}
			if _, err := app.Token(context.Background()); err == nil {
				t.Errorf("%s: Token() succeeded, want error", tc.name)
			}
		})
	}
}

func fakeAuthServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations":
			writeResponse(t, w, http.StatusOK, "application/json", `[{"id":100}]`)
		case "/app/installations/100/access_tokens":
			body := fmt.Sprintf(`{"token":"inst-tok","expires_at":%q}`, testNow.Add(time.Hour).Format(time.RFC3339))
			writeResponse(t, w, http.StatusCreated, "application/json", body)
		default:
			handler(w, r)
		}
	}))
}

func TestDo_Positive_RequestSuccess(t *testing.T) {
	key := testRSAKey(t)
	srv := fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/test/resource" {
			writeResponse(t, w, http.StatusNotFound, "", "")
			return
		}
		if r.Header.Get("Authorization") != "Bearer inst-tok" {
			writeResponse(t, w, http.StatusUnauthorized, "", "")
			return
		}
		writeResponse(t, w, http.StatusOK, "application/json", `{"message":"hello"}`)
	})
	defer srv.Close()

	app := &App{
		ClientID: "client",
		Key:      key,
		BaseURL:  srv.URL,
		HTTP:     srv.Client(),
		Now:      func() time.Time { return testNow },
	}

	var out struct {
		Message string `json:"message"`
	}
	if err := app.Do(context.Background(), http.MethodGet, "/test/resource", nil, &out); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if out.Message != "hello" {
		t.Errorf("out.Message = %q, want hello", out.Message)
	}
}

func TestDo_Negative_RedirectRefused(t *testing.T) {
	key := testRSAKey(t)
	srv := fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		writeResponse(t, w, http.StatusOK, "", "")
	})
	defer srv.Close()

	app := &App{
		ClientID: "client",
		Key:      key,
		BaseURL:  srv.URL,
		HTTP:     srv.Client(),
		Now:      func() time.Time { return testNow },
	}

	if err := app.Do(context.Background(), http.MethodGet, "/redirect", nil, nil); err == nil {
		t.Fatal("Do() followed redirect, want error")
	}
}

func TestDo_Negative_BackoffOn403RetryAfter(t *testing.T) {
	key := testRSAKey(t)
	srv := fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rate-limited" {
			w.Header().Set("Retry-After", "120")
			writeResponse(t, w, http.StatusForbidden, "", "")
			return
		}
		writeResponse(t, w, http.StatusNotFound, "", "")
	})
	defer srv.Close()

	app := &App{
		ClientID: "client",
		Key:      key,
		BaseURL:  srv.URL,
		HTTP:     srv.Client(),
		Now:      func() time.Time { return testNow },
	}

	err := app.Do(context.Background(), http.MethodGet, "/rate-limited", nil, nil)
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("Do() error = %v, want *StatusError", err)
	}
	if se.Code != http.StatusForbidden || se.RetryAfter != 120*time.Second {
		t.Errorf("StatusError = %+v, want 403 / 120s", se)
	}
}

func TestDo_Boundary_OversizeBodyRefused(t *testing.T) {
	key := testRSAKey(t)
	srv := fakeAuthServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oversize" {
			writeResponse(t, w, http.StatusOK, "", strings.Repeat("a", MaxResponseBytes+1))
			return
		}
		writeResponse(t, w, http.StatusNotFound, "", "")
	})
	defer srv.Close()

	app := &App{
		ClientID: "client",
		Key:      key,
		BaseURL:  srv.URL,
		HTTP:     srv.Client(),
		Now:      func() time.Time { return testNow },
	}

	err := app.Do(context.Background(), http.MethodGet, "/oversize", nil, nil)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Do() error = %v, want ErrTooLarge", err)
	}
}

func TestDo_Negative_AuthorizationNeverSentToOtherHost(t *testing.T) {
	key := testRSAKey(t)
	srv := fakeAuthServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeResponse(t, w, http.StatusOK, "", "")
	})
	defer srv.Close()

	app := &App{
		ClientID: "client",
		Key:      key,
		BaseURL:  srv.URL,
		HTTP:     srv.Client(),
		Now:      func() time.Time { return testNow },
	}

	// The test server speaks http, so the same host under another scheme must be refused too.
	host := strings.TrimPrefix(srv.URL, "http://")
	for _, target := range []string{
		"https://evil.example.com/steal",
		"//evil.example.com/steal",
		"https://" + host + "/graphql",
	} {
		err := app.Do(context.Background(), http.MethodGet, target, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "does not match base host") {
			t.Errorf("Do(%q) error = %v, want error naming host mismatch", target, err)
		}
	}
}

func TestSlowDown_Boundary_ServerRules(t *testing.T) {
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	date := func(d time.Duration) string { return now.Add(d).Format(http.TimeFormat) }
	unix := func(d time.Duration) string { return strconv.FormatInt(now.Add(d).Unix(), 10) }
	for _, tc := range []struct {
		name string
		code int
		h    map[string]string
		want time.Duration
	}{
		{"429 delay-seconds", 429, map[string]string{"Retry-After": "120"}, 2 * time.Minute},
		{"503 http date", 503, map[string]string{"Retry-After": date(10 * time.Minute)}, 10 * time.Minute},
		{"403 primary limit until reset", 403, map[string]string{"X-Ratelimit-Remaining": "0", "X-Ratelimit-Reset": unix(30 * time.Minute)}, 30 * time.Minute},
		{"Retry-After wins over the reset", 429, map[string]string{"Retry-After": "300", "X-Ratelimit-Remaining": "0", "X-Ratelimit-Reset": unix(time.Hour)}, 5 * time.Minute},
		{"429 without a time waits the minimum", 429, nil, MinBackoff},
		{"429 unreadable Retry-After waits the minimum", 429, map[string]string{"Retry-After": "soon"}, MinBackoff},
		{"remaining 0 without a reset waits the minimum", 403, map[string]string{"X-Ratelimit-Remaining": "0"}, MinBackoff},
		{"403 without limit headers is no slow-down", 403, nil, 0},
		{"404 ignores Retry-After", 404, map[string]string{"Retry-After": "120"}, 0},
		{"500 is no slow-down", 500, nil, 0},
		{"Retry-After 0 waits the minimum", 429, map[string]string{"Retry-After": "0"}, MinBackoff},
		{"a date in the past waits the minimum", 503, map[string]string{"Retry-After": date(-time.Hour)}, MinBackoff},
		{"a reset in the past waits the minimum", 429, map[string]string{"X-Ratelimit-Remaining": "0", "X-Ratelimit-Reset": unix(-time.Hour)}, MinBackoff},
		{"MaxBackoff exactly", 429, map[string]string{"Retry-After": "86400"}, MaxBackoff},
		{"more than MaxBackoff is capped", 429, map[string]string{"Retry-After": "86401"}, MaxBackoff},
		{"a delay beyond 32 bits is unreadable", 429, map[string]string{"Retry-After": "99999999999"}, MinBackoff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tc.h {
				h.Set(k, v)
			}
			if got := SlowDown(tc.code, h, now); got != tc.want {
				t.Fatalf("SlowDown(%d, %v) = %v, want %v", tc.code, tc.h, got, tc.want)
			}
		})
	}
}
