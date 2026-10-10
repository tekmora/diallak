package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfig_Negative_JSONEncodingOmitsSecrets(t *testing.T) {
	cfg := Config{Token: "token-value-4711", ModelKey: "key-value-0815", GuildID: 1, CorpusDir: "/corpus"}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}
	if strings.Contains(string(data), "token-value-4711") || strings.Contains(string(data), "key-value-0815") {
		t.Fatalf("JSON encoding of Config carries a secret: %s", data)
	}
	if !strings.Contains(string(data), "/corpus") {
		t.Fatalf("JSON encoding of Config lost a plain field: %s", data)
	}
}
