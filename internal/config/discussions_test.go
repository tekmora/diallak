package config

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func testRSAPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}))
}

func testECPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ec key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal ec key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestLoad_Positive_DiscussionsSettings(t *testing.T) {
	base := validBaseEnv(t)
	base[EnvNewsStateFile] = filepath.Join(t.TempDir(), "news.json")
	pemText := testRSAPEM(t)
	base[EnvDiscussionsAppClientID] = "123456.app"
	base[EnvDiscussionsAppKeyFile] = writeSecret(t, "appkey.pem", pemText)

	cfg, err := Load(env(base))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.HasDiscussions() {
		t.Fatal("HasDiscussions() = false, want true")
	}
	if cfg.DiscussionsAppClientID != "123456.app" {
		t.Errorf("DiscussionsAppClientID = %q, want 123456.app", cfg.DiscussionsAppClientID)
	}
	if cfg.DiscussionsAppKey != strings.TrimSpace(pemText) {
		t.Errorf("DiscussionsAppKey mismatch")
	}
}

func TestLoad_Positive_DiscussionsKeyRedacted(t *testing.T) {
	base := validBaseEnv(t)
	base[EnvNewsStateFile] = filepath.Join(t.TempDir(), "news.json")
	pemText := testRSAPEM(t)
	base[EnvDiscussionsAppClientID] = "app.client.1"
	base[EnvDiscussionsAppKeyFile] = writeSecret(t, "appkey.pem", pemText)

	cfg, err := Load(env(base))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	secretFragment := pemText[30:60]
	if strings.Contains(cfg.String(), secretFragment) || !strings.Contains(cfg.String(), `DiscussionsAppKey:"[redacted]"`) {
		t.Errorf("String() leaked key or did not redact: %s", cfg.String())
	}
	if strings.Contains(cfg.GoString(), secretFragment) || !strings.Contains(cfg.GoString(), `DiscussionsAppKey:"[redacted]"`) {
		t.Errorf("GoString() leaked key or did not redact: %s", cfg.GoString())
	}

	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("dump", slog.Any("cfg", cfg))
	if strings.Contains(buf.String(), secretFragment) || !strings.Contains(buf.String(), `"DiscussionsAppKey":"[redacted]"`) {
		t.Errorf("LogValue() leaked key or did not redact: %s", buf.String())
	}

	plusV := fmt.Sprintf("%+v", cfg)
	if strings.Contains(plusV, secretFragment) || !strings.Contains(plusV, `DiscussionsAppKey:"[redacted]"`) {
		t.Errorf("%%+v leaked key or did not redact: %s", plusV)
	}
}

func TestLoad_Negative_DiscussionsSettingsRefused(t *testing.T) {
	pemText := testRSAPEM(t)
	ecPEM := testECPEM(t)

	cases := []struct {
		name, state, clientID, key, wantVar string
	}{
		{"client id without key file", "/var/lib/news.json", "client.1", "", EnvDiscussionsAppKeyFile},
		{"key file without client id", "/var/lib/news.json", "", "valid_key", EnvDiscussionsAppClientID},
		{"discussions without news state file", "", "client.1", "valid_key", EnvNewsStateFile},
		{"invalid client id characters", "/var/lib/news.json", "bad/client", "valid_key", EnvDiscussionsAppClientID},
		{"client id exceeds 64 chars", "/var/lib/news.json", strings.Repeat("a", 65), "valid_key", EnvDiscussionsAppClientID},
		{"key file missing", "/var/lib/news.json", "client.1", "/nonexistent/key.pem", EnvDiscussionsAppKeyFile},
		{"key file empty", "/var/lib/news.json", "client.1", "empty", EnvDiscussionsAppKeyFile},
		{"key file not RSA", "/var/lib/news.json", "client.1", "ec_key", EnvDiscussionsAppKeyFile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := validBaseEnv(t)
			base[EnvNewsStateFile] = tc.state
			base[EnvDiscussionsAppClientID] = tc.clientID

			switch tc.key {
			case "valid_key":
				base[EnvDiscussionsAppKeyFile] = writeSecret(t, "key.pem", pemText)
			case "empty":
				base[EnvDiscussionsAppKeyFile] = writeSecret(t, "key.pem", "  \n")
			case "ec_key":
				base[EnvDiscussionsAppKeyFile] = writeSecret(t, "key.pem", ecPEM)
			default:
				base[EnvDiscussionsAppKeyFile] = tc.key
			}

			if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), tc.wantVar) {
				t.Errorf("%s: Load = %v, want error naming %s", tc.name, err, tc.wantVar)
			}
		})
	}
}

func TestLoad_Boundary_DiscussionsKeySize(t *testing.T) {
	base := validBaseEnv(t)
	base[EnvNewsStateFile] = filepath.Join(t.TempDir(), "news.json")
	base[EnvDiscussionsAppClientID] = "client1"

	pemText := testRSAPEM(t)
	padding := strings.Repeat("# comment\n", (MaxKeyFileBytes-len(pemText))/10)
	paddedPEM := pemText + padding
	if len(paddedPEM) < MaxKeyFileBytes {
		paddedPEM += strings.Repeat(" ", MaxKeyFileBytes-len(paddedPEM))
	}
	base[EnvDiscussionsAppKeyFile] = writeSecret(t, "key.pem", paddedPEM)
	if _, err := Load(env(base)); err != nil {
		t.Fatalf("Load with key of %d bytes: %v", len(paddedPEM), err)
	}

	base[EnvDiscussionsAppKeyFile] = writeSecret(t, "key_large.pem", paddedPEM+" ")
	if _, err := Load(env(base)); err == nil {
		t.Fatalf("Load with key larger than %d bytes succeeded", MaxKeyFileBytes)
	}
}
