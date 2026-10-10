package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/answer"
	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/corpus"
)

func setupTestCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	manifest := `{
		"version": 1,
		"sources": [
			{
				"directory": "alpha",
				"repository": "tekmora/alpha",
				"revision": "abcdef1234567890",
				"paths": ["docs/*.md"]
			},
			{
				"directory": "beta",
				"repository": "tekmora/beta",
				"revision": "123456abcdef",
				"paths": ["README.md"]
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(dir, corpus.ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	alphaDocs := filepath.Join(dir, "alpha", "docs")
	if err := os.MkdirAll(alphaDocs, 0o700); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
	}
	alphaContent := "# Authentication Guide\nTokens are stored in bounded secret files.\n"
	if err := os.WriteFile(filepath.Join(alphaDocs, "auth.md"), []byte(alphaContent), 0o600); err != nil {
		t.Fatalf("write alpha doc: %v", err)
	}

	betaDir := filepath.Join(dir, "beta")
	if err := os.MkdirAll(betaDir, 0o700); err != nil {
		t.Fatalf("mkdir beta: %v", err)
	}
	betaContent := "# Beta Overview\nBeta repository documentation.\n"
	if err := os.WriteFile(filepath.Join(betaDir, "README.md"), []byte(betaContent), 0o600); err != nil {
		t.Fatalf("write beta doc: %v", err)
	}

	return dir
}

func TestBuild_Positive_NoModelAnswersWithExcerptsAndRevision(t *testing.T) {
	corpusDir := setupTestCorpus(t)
	cfg := config.Config{
		CorpusDir: corpusDir,
	}

	assembly, cleanup, err := Build(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Build = %v, want nil", err)
	}
	defer func() {
		if closeErr := cleanup(); closeErr != nil {
			t.Errorf("cleanup = %v, want nil", closeErr)
		}
	}()

	if assembly.HasModel {
		t.Errorf("HasModel = true, want false")
	}
	if assembly.Chunks != 2 {
		t.Errorf("Chunks = %d, want 2", assembly.Chunks)
	}
	if len(assembly.Sources) != 2 {
		t.Errorf("len(Sources) = %d, want 2", len(assembly.Sources))
	}

	// Ask question matching alpha docs
	ans, err := assembly.Pipeline.Ask(context.Background(), "where are tokens stored in secret files?")
	if err != nil {
		t.Fatalf("Pipeline.Ask = %v, want nil", err)
	}
	if ans.Kind != answer.KindExcerpts {
		t.Fatalf("ans.Kind = %q, want %q", ans.Kind, answer.KindExcerpts)
	}
	if len(ans.Sources) == 0 {
		t.Fatalf("ans.Sources is empty")
	}

	// Verify revision from manifest reaches chunk and rendered source line
	src := ans.Sources[0]
	if src.Revision != "abcdef1234567890" {
		t.Errorf("source revision = %q, want abcdef1234567890", src.Revision)
	}

	rendered := answer.Render(ans)
	// Render truncates revisions to first 12 runes: "abcdef123456"
	if !strings.Contains(rendered, "abcdef123456") {
		t.Errorf("rendered excerpts %q does not contain truncated revision abcdef123456", rendered)
	}
}

func TestBuild_Positive_WithModelAnswersQuestion(t *testing.T) {
	corpusDir := setupTestCorpus(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": "Tokens are kept in bounded files [1].",
					},
				},
			},
		}
		if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
			http.Error(w, encErr.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	cfg := config.Config{
		CorpusDir:    corpusDir,
		ModelBaseURL: server.URL,
		ModelAlias:   "router/test",
	}

	assembly, cleanup, err := Build(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Build = %v, want nil", err)
	}
	defer func() {
		if closeErr := cleanup(); closeErr != nil {
			t.Errorf("cleanup = %v, want nil", closeErr)
		}
	}()

	if !assembly.HasModel {
		t.Errorf("HasModel = false, want true")
	}

	ans, err := assembly.Pipeline.Ask(context.Background(), "where are tokens stored in secret files?")
	if err != nil {
		t.Fatalf("Pipeline.Ask = %v, want nil", err)
	}
	if ans.Kind != answer.KindAnswer {
		t.Fatalf("ans.Kind = %q, want %q", ans.Kind, answer.KindAnswer)
	}
	if !strings.Contains(ans.Text, "Tokens are kept in bounded files [1].") {
		t.Errorf("ans.Text = %q, want model response text", ans.Text)
	}
}

func TestBuild_Negative_ConfigurationAndDirectoryErrors(t *testing.T) {
	t.Run("missing dir", func(t *testing.T) {
		cfg := config.Config{CorpusDir: filepath.Join(t.TempDir(), "nonexistent")}
		_, _, err := Build(context.Background(), cfg, nil)
		if err == nil || !strings.Contains(err.Error(), "open corpus root") {
			t.Fatalf("Build missing dir error = %v, want open corpus root error", err)
		}
	})

	t.Run("missing manifest", func(t *testing.T) {
		emptyDir := t.TempDir()
		cfg := config.Config{CorpusDir: emptyDir}
		_, _, err := Build(context.Background(), cfg, nil)
		if err == nil || !strings.Contains(err.Error(), "load manifest") {
			t.Fatalf("Build missing manifest error = %v, want load manifest error", err)
		}
	})

	t.Run("bad manifest", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, corpus.ManifestName), []byte("{invalid json"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := config.Config{CorpusDir: dir}
		_, _, err := Build(context.Background(), cfg, nil)
		if err == nil || !strings.Contains(err.Error(), "load manifest") {
			t.Fatalf("Build bad manifest error = %v, want load manifest error", err)
		}
	})

	t.Run("source dir missing", func(t *testing.T) {
		dir := t.TempDir()
		manifest := `{"version": 1, "sources": [{"directory": "absent", "repository": "tekmora/absent", "revision": "rev", "paths": ["*.md"]}]}`
		if err := os.WriteFile(filepath.Join(dir, corpus.ManifestName), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg := config.Config{CorpusDir: dir}
		_, _, err := Build(context.Background(), cfg, nil)
		if err == nil || !strings.Contains(err.Error(), "load corpus") {
			t.Fatalf("Build missing source dir error = %v, want load corpus error", err)
		}
	})
}

func TestBuild_Negative_CheckoutSymlinkEscapesCorpus(t *testing.T) {
	outsideDir := t.TempDir()
	outsideSecret := filepath.Join(outsideDir, "secret_outside.md")
	secretContent := "# Super Secret Outside Document\nPrivate leaked information."
	if err := os.WriteFile(outsideSecret, []byte(secretContent), 0o600); err != nil {
		t.Fatalf("write outside secret: %v", err)
	}

	corpusDir := setupTestCorpus(t)
	symlinkPath := filepath.Join(corpusDir, "alpha", "docs", "escaped_link.md")
	if err := os.Symlink(outsideSecret, symlinkPath); err != nil {
		t.Skipf("platform refuses symlinks: %v", err)
	}

	cfg := config.Config{CorpusDir: corpusDir}
	_, _, err := Build(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("Build succeeded with escaping symlink, want failure")
	}

	// Error must name the symlink path
	if !strings.Contains(err.Error(), "docs/escaped_link.md") && !strings.Contains(err.Error(), "escaped_link.md") {
		t.Fatalf("Build error %q does not name symlink path escaped_link.md", err.Error())
	}
	// Outside content must never appear in error
	if strings.Contains(err.Error(), "Private leaked information") {
		t.Fatalf("Build error leaked outside file content: %v", err)
	}
}

func TestBuild_Negative_DirectorySymlinkEscapesCorpus(t *testing.T) {
	outsideDir := t.TempDir()
	outsideDoc := filepath.Join(outsideDir, "doc.md")
	if err := os.WriteFile(outsideDoc, []byte("# Outside\nContent"), 0o600); err != nil {
		t.Fatalf("write outside doc: %v", err)
	}

	corpusDir := t.TempDir()
	symlinkCheckout := filepath.Join(corpusDir, "linked_checkout")
	if err := os.Symlink(outsideDir, symlinkCheckout); err != nil {
		t.Skipf("platform refuses symlinks: %v", err)
	}

	manifest := `{"version": 1, "sources": [{"directory": "linked_checkout", "repository": "tekmora/linked", "revision": "rev", "paths": ["*.md"]}]}`
	if err := os.WriteFile(filepath.Join(corpusDir, corpus.ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{CorpusDir: corpusDir}
	_, _, err := Build(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("Build with directory symlink escaping corpus succeeded, want failure")
	}
	if !strings.Contains(err.Error(), "load corpus") {
		t.Fatalf("Build error = %v, want load corpus error", err)
	}
}

func TestBuild_Negative_ModelEndpointDownFallsBackToExcerpts(t *testing.T) {
	corpusDir := setupTestCorpus(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	serverURL := server.URL
	server.Close() // Immediately close to simulate endpoint down

	cfg := config.Config{
		CorpusDir:    corpusDir,
		ModelBaseURL: serverURL,
		ModelAlias:   "router/test",
	}

	assembly, cleanup, err := Build(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Build = %v, want nil", err)
	}
	defer func() {
		if closeErr := cleanup(); closeErr != nil {
			t.Errorf("cleanup = %v, want nil", closeErr)
		}
	}()

	ans, err := assembly.Pipeline.Ask(context.Background(), "where are tokens stored in secret files?")
	if err != nil {
		t.Fatalf("Pipeline.Ask = %v, want nil", err)
	}
	if ans.Kind != answer.KindExcerpts {
		t.Fatalf("ans.Kind = %q, want %q", ans.Kind, answer.KindExcerpts)
	}
	if ans.Reason != answer.ReasonModelUnavailable {
		t.Fatalf("ans.Reason = %q, want %q", ans.Reason, answer.ReasonModelUnavailable)
	}
}

func TestBuild_Boundary_DoubleCloseDoesNotPanic(t *testing.T) {
	corpusDir := setupTestCorpus(t)
	cfg := config.Config{CorpusDir: corpusDir}

	_, cleanup, err := Build(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("Build = %v, want nil", err)
	}

	if closeErr := cleanup(); closeErr != nil {
		t.Fatalf("first cleanup = %v, want nil", closeErr)
	}
	if closeErr := cleanup(); closeErr != nil {
		t.Fatalf("second cleanup = %v, want nil", closeErr)
	}
}
