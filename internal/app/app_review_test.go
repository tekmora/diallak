package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/answer"
	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/corpus"
)

// rotationQuestion matches every document of the wide corpus with all of its words.
const rotationQuestion = "rotate the signing key"

// setupWideCorpus writes four documents that each answer rotationQuestion with a paragraph of
// more than 400 runes, so that the number of passages and the length of an excerpt show which
// limit is in effect. A fifth document holds a word the four do not, for a question they cover
// only in part.
func setupWideCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"version":1,"sources":[{"directory":"wide","repository":"tekmora/wide","revision":"0123456789abcdef","paths":["docs/*.md"]}]}`
	if err := os.WriteFile(filepath.Join(dir, corpus.ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	docs := filepath.Join(dir, "wide", "docs")
	if err := os.MkdirAll(docs, 0o700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	filler := strings.Repeat("The procedure is described step by step for every operator. ", 8)
	for i := 1; i <= 4; i++ {
		text := fmt.Sprintf("# Guide %d\nTo rotate the signing key use procedure %d. %s\n", i, i, filler)
		if err := os.WriteFile(filepath.Join(docs, fmt.Sprintf("guide%d.md", i)), []byte(text), 0o600); err != nil {
			t.Fatalf("write guide %d: %v", i, err)
		}
	}
	if err := os.WriteFile(filepath.Join(docs, "zebra.md"), []byte("# Zebra\nA zebra crossing is marked.\n"), 0o600); err != nil {
		t.Fatalf("write zebra: %v", err)
	}
	return dir
}

// askWith builds the application from cfg and asks question.
func askWith(t *testing.T, cfg config.Config, question string) (answer.Answer, error) {
	t.Helper()
	assembly, cleanup, err := Build(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Build = %v, want nil", err)
	}
	t.Cleanup(func() {
		if closeErr := cleanup(); closeErr != nil {
			t.Errorf("cleanup = %v", closeErr)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return assembly.Pipeline.Ask(ctx, question)
}

func TestBuild_Positive_PassageCountFollowsTheSetting(t *testing.T) {
	dir := setupWideCorpus(t)
	cases := []struct {
		name    string
		setting int
		want    int
	}{
		{"not set: the built-in three", 0, 3},
		{"one", 1, 1},
		{"four", 4, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ans, err := askWith(t, config.Config{CorpusDir: dir, TopPassages: tc.setting}, rotationQuestion)
			if err != nil {
				t.Fatalf("Ask = %v", err)
			}
			if len(ans.Sources) != tc.want {
				t.Fatalf("sources = %d, want %d", len(ans.Sources), tc.want)
			}
		})
	}
}

func TestBuild_Positive_ScoreFollowsTheSetting(t *testing.T) {
	dir := setupWideCorpus(t)
	// No guide holds the last word, so the guides cover the question only in part.
	const partial = "rotate the signing key zebra"

	ans, err := askWith(t, config.Config{CorpusDir: dir}, partial)
	if err != nil {
		t.Fatalf("Ask with the built-in score = %v", err)
	}
	if ans.Kind != answer.KindExcerpts {
		t.Fatalf("kind with the built-in score = %q, want %q", ans.Kind, answer.KindExcerpts)
	}

	ans, err = askWith(t, config.Config{CorpusDir: dir, MinScore: 1}, partial)
	if err != nil {
		t.Fatalf("Ask with score 1 = %v", err)
	}
	if ans.Kind != answer.KindRefusal {
		t.Fatalf("kind with score 1 = %q, want %q", ans.Kind, answer.KindRefusal)
	}
}

func TestBuild_Boundary_QuestionLimitFollowsTheSetting(t *testing.T) {
	dir := setupWideCorpus(t)
	limit := utf8.RuneCountInString(rotationQuestion)

	if _, err := askWith(t, config.Config{CorpusDir: dir, QuestionMaxChars: limit}, rotationQuestion); err != nil {
		t.Fatalf("a question of exactly the limit was refused: %v", err)
	}
	if _, err := askWith(t, config.Config{CorpusDir: dir, QuestionMaxChars: limit - 1}, rotationQuestion); err == nil {
		t.Fatalf("a question one rune over the limit was accepted")
	}
	if _, err := askWith(t, config.Config{CorpusDir: dir}, rotationQuestion); err != nil {
		t.Fatalf("the question was refused without a configured limit: %v", err)
	}
}

func TestBuild_Positive_ExcerptLengthFollowsTheSetting(t *testing.T) {
	dir := setupWideCorpus(t)
	cases := []struct {
		name    string
		setting int
		want    int
	}{
		{"not set: the built-in 400", 0, 400},
		{"fifty", 50, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ans, err := askWith(t, config.Config{CorpusDir: dir, ExcerptChars: tc.setting}, rotationQuestion)
			if err != nil {
				t.Fatalf("Ask = %v", err)
			}
			if len(ans.Sources) == 0 {
				t.Fatalf("no sources")
			}
			excerpt := ans.Sources[0].Excerpt
			if !strings.HasSuffix(excerpt, "...") {
				t.Fatalf("the passage is longer than the limit, but its excerpt is not marked as cut: %q", excerpt)
			}
			if got := utf8.RuneCountInString(strings.TrimSuffix(excerpt, "...")); got != tc.want {
				t.Fatalf("excerpt holds %d runes of the passage, want %d", got, tc.want)
			}
		})
	}
}

func TestBuild_Positive_ModelTimeoutFollowsTheSetting(t *testing.T) {
	dir := setupWideCorpus(t)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	// The endpoint never answers. With the timeout set to one second the request ends there and
	// the reply falls back to excerpts; with the built-in timeout the ten seconds of askWith
	// would end first and Ask would return an error.
	cfg := config.Config{CorpusDir: dir, ModelBaseURL: srv.URL, ModelAlias: "router/test", ModelPolicy: "example-policy", ModelTimeout: time.Second}
	start := time.Now()
	ans, err := askWith(t, cfg, rotationQuestion)
	if err != nil {
		t.Fatalf("Ask = %v after %v, want the excerpt fallback", err, time.Since(start))
	}
	if ans.Kind != answer.KindExcerpts || ans.Reason != answer.ReasonModelUnavailable {
		t.Fatalf("kind %q reason %q, want excerpts because the model is unavailable", ans.Kind, ans.Reason)
	}
}
