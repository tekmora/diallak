package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	identityBlockStart = "<!-- identity block: start -->"
	identityBlockEnd   = "<!-- identity block: end -->"
)

// identityBlock returns the text between the two identity block markers of a document.
func identityBlock(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(data)
	if strings.Count(text, identityBlockStart) != 1 || strings.Count(text, identityBlockEnd) != 1 {
		t.Fatalf("%s does not hold exactly one identity block", path)
	}
	start := strings.Index(text, identityBlockStart) + len(identityBlockStart)
	end := strings.Index(text, identityBlockEnd)
	if end < start {
		t.Fatalf("%s: the identity block ends before it starts", path)
	}
	return text[start:end]
}

// Who provides the bot is stated in one block that the imprint and the privacy notice both
// carry. It is filled once before the public launch, so the two copies must never differ, and
// no other compliance document may name a contact of its own.
func TestCompliance_Negative_IdentityIsOneBlock(t *testing.T) {
	dir := filepath.Join("..", "..", "docs", "compliance")
	imprint := identityBlock(t, filepath.Join(dir, "imprint.md"))
	notice := identityBlock(t, filepath.Join(dir, "privacy-notice.md"))
	if strings.TrimSpace(imprint) == "" {
		t.Fatalf("the identity block of the imprint is empty")
	}
	if imprint != notice {
		t.Fatalf("the identity block differs between the imprint and the privacy notice:\n%q\n%q", imprint, notice)
	}

	documents, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil || len(documents) == 0 {
		t.Fatalf("no compliance documents found: %v", err)
	}
	address := regexp.MustCompile(`[A-Za-z0-9._+-]+@[A-Za-z0-9-]+\.[A-Za-z0-9.-]+`)
	for _, path := range documents {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		if start := strings.Index(text, identityBlockStart); start >= 0 {
			end := strings.Index(text, identityBlockEnd)
			if end > start {
				text = text[:start] + text[end:]
			}
		}
		if found := address.FindString(text); found != "" {
			t.Errorf("%s names the address %q outside the identity block", path, found)
		}
	}
}
