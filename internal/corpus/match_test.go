package corpus

import (
	"strings"
	"testing"
)

func TestMatch_Positive_PatternsAndFiles(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"README.md", "README.md", true},
		{"README.md", "docs/README.md", false},
		{"docs/**/*.md", "docs/concepts.md", true},
		{"docs/**/*.md", "docs/adr/0001.md", true},
		{"docs/**/*.md", "docs/adr/sub/0001.md", true},
		{"docs/**/*.md", "README.md", false},
		{"docs/**/*.md", "docs_other/concepts.md", false},
		{"docs/*.md", "docs/concepts.md", true},
		{"docs/*.md", "docs/adr/0001.md", false},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/b", true},
		{"a/**/b", "other/b", false},
	}
	for _, tc := range tests {
		got, err := Match(tc.pattern, tc.name)
		if err != nil {
			t.Errorf("Match(%q, %q) error = %v, want nil", tc.pattern, tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

func TestMatch_Negative_InvalidPatternsAndNames(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		wantErr string
	}{
		{"empty pattern", "", "file.md", "pattern is empty"},
		{"leading slash in pattern", "/docs/**/*.md", "docs/a.md", "leading slash"},
		{"leading slash in name", "docs/**/*.md", "/docs/a.md", "leading slash"},
		{"dotdot in pattern start", "../docs/**/*.md", "docs/a.md", ".. element"},
		{"dotdot in pattern middle", "docs/**/../a.md", "docs/a.md", ".. element"},
		{"dotdot in pattern end", "docs/..", "docs/a.md", ".. element"},
		{"dotdot in name start", "docs/**/*.md", "../docs/a.md", ".. element"},
		{"dotdot in name middle", "docs/**/*.md", "docs/../a.md", ".. element"},
		{"dotdot in name end", "docs/**/*.md", "docs/..", ".. element"},
		{"standalone double star", "**", "docs/a.md", "unsupported **"},
		{"double star without slash prefix", "**/*.md", "docs/a.md", "unsupported **"},
		{"double star at end", "docs/**", "docs/a.md", "unsupported **"},
		{"multiple double stars", "docs/**/**/*.md", "docs/a.md", "unsupported **"},
		{"double star in glob", "docs/**/a**b", "docs/ab", "unsupported **"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Match(tc.pattern, tc.target)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Match(%q, %q) error = %v, want containing %q", tc.pattern, tc.target, err, tc.wantErr)
			}
			if got {
				t.Errorf("Match(%q, %q) = true on error, want false", tc.pattern, tc.target)
			}
		})
	}
}

func TestMatch_Boundary_EdgeCases(t *testing.T) {
	// Zero-depth match for prefix/**/glob
	matched, err := Match("docs/**/file.md", "docs/file.md")
	if err != nil || !matched {
		t.Errorf("Match zero-depth = %v, %v; want true, nil", matched, err)
	}

	// Exact glob match with special characters
	matchedSpecial, err := Match("docs/**/[a-z].md", "docs/x.md")
	if err != nil || !matchedSpecial {
		t.Errorf("Match character class = %v, %v; want true, nil", matchedSpecial, err)
	}

	// Invalid glob syntax
	_, errInvalid := Match("docs/**/[a-", "docs/a")
	if errInvalid == nil {
		t.Error("Match with malformed glob = nil error, want error")
	}
}
