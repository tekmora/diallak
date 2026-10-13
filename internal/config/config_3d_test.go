package config

import (
	"strings"
	"testing"
)

func TestHasModel_Negative_RequiresBothURLAndAlias(t *testing.T) {
	c1 := Config{ModelBaseURL: "https://example.com", ModelAlias: ""}
	if c1.HasModel() {
		t.Errorf("HasModel() with empty alias = true, want false")
	}
	c2 := Config{ModelBaseURL: "", ModelAlias: "claude"}
	if c2.HasModel() {
		t.Errorf("HasModel() with empty base URL = true, want false")
	}
}

func TestGoString_Positive_MatchesStringFormat(t *testing.T) {
	c := Config{GuildID: 12345}
	got := c.GoString()
	want := c.String()
	if got != want || got == "" {
		t.Fatalf("GoString() = %q, want non-empty %q", got, want)
	}
}

func TestGoString_Negative_OmitsRawTokenSecret(t *testing.T) {
	c := Config{Token: "unredacted-secret-token"}
	got := c.GoString()
	if strings.Contains(got, "unredacted-secret-token") {
		t.Fatalf("GoString() contains unredacted token: %q", got)
	}
}

func TestGoString_Boundary_OmitsRawModelKeySecret(t *testing.T) {
	c := Config{ModelKey: "unredacted-model-key"}
	got := c.GoString()
	if strings.Contains(got, "unredacted-model-key") {
		t.Fatalf("GoString() contains unredacted model key: %q", got)
	}
}

func TestLogValue_Negative_RedactsModelKey(t *testing.T) {
	c := Config{ModelKey: "super-secret-model-key"}
	val := c.LogValue()
	for _, attr := range val.Group() {
		if attr.Key == "ModelKey" {
			if attr.Value.String() == "super-secret-model-key" {
				t.Fatalf("LogValue() leaked ModelKey: %q", attr.Value.String())
			}
			if attr.Value.String() != "[redacted]" {
				t.Fatalf("LogValue() ModelKey = %q, want [redacted]", attr.Value.String())
			}
			return
		}
	}
	t.Fatal("LogValue() missing ModelKey attribute")
}
