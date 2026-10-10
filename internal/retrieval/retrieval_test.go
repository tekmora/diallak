package retrieval

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/tekmora/discord-bot/internal/corpus"
)

func sampleChunks() []corpus.Chunk {
	return []corpus.Chunk{
		{
			ID:         "docs/concepts.md#1",
			Repository: "lusoris/SnatchArr",
			Revision:   "abc",
			Path:       "docs/concepts.md",
			Title:      "Concepts",
			Heading:    "Overview",
			Text:       "SnatchArr performs automated searches for media releases.",
		},
		{
			ID:         "docs/deployment.md#1",
			Repository: "lusoris/SnatchArr",
			Revision:   "abc",
			Path:       "docs/deployment.md",
			Title:      "Deployment",
			Heading:    "Helm",
			Text:       "Deploy SnatchArr with helm install on your cluster.",
		},
		{
			ID:         "docs/download-clients.md#1",
			Repository: "lusoris/SnatchArr",
			Revision:   "abc",
			Path:       "docs/download-clients.md",
			Title:      "Clients",
			Heading:    "Supported Clients",
			Text:       "Supported download clients include qBittorrent and Transmission.",
		},
	}
}

func TestBuild_Positive_ConstructsIndex(t *testing.T) {
	chunks := sampleChunks()
	ix, err := Build(chunks)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if ix == nil || ix.n != len(chunks) {
		t.Fatalf("Build() returned unexpected index: %+v", ix)
	}
}

func TestBuild_Negative_EmptyDuplicateAndOversized(t *testing.T) {
	if _, err := Build(nil); err == nil {
		t.Error("Build(nil) = nil, want error")
	}

	dup := []corpus.Chunk{
		{ID: "same#1", Path: "a.md", Text: "text one"},
		{ID: "same#1", Path: "b.md", Text: "text two"},
	}
	if _, err := Build(dup); err == nil || !strings.Contains(err.Error(), "duplicate chunk ID") {
		t.Errorf("Build duplicate error = %v, want duplicate chunk ID", err)
	}
}

func TestBuild_Boundary_MaxChunksLimit(t *testing.T) {
	chunks := make([]corpus.Chunk, 8193)
	for i := range chunks {
		chunks[i] = corpus.Chunk{ID: fmt.Sprintf("file#%d", i), Path: "file.md", Text: "text"}
	}
	if _, err := Build(chunks); err == nil || !strings.Contains(err.Error(), "exceeds maximum 8192") {
		t.Errorf("Build >8192 error = %v, want exceeds maximum 8192", err)
	}

	valid := chunks[:8192]
	if _, err := Build(valid); err != nil {
		t.Fatalf("Build 8192 chunks = %v, want nil", err)
	}
}

func TestSearch_Positive_DeterministicScoringAndTies(t *testing.T) {
	ix, err := Build(sampleChunks())
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	hits, err := ix.Search("helm install deployment", Options{Limit: 3, MinScore: 0.1})
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Search returned zero hits for matching query")
	}
	if hits[0].Chunk.Path != "docs/deployment.md" {
		t.Errorf("top hit path = %q, want docs/deployment.md", hits[0].Chunk.Path)
	}
	if hits[0].Score <= 0 || hits[0].Score > 1.0 {
		t.Errorf("top hit score = %f, want within (0, 1]", hits[0].Score)
	}
}

func TestSearch_Positive_ConcurrentSafe(t *testing.T) {
	ix, err := Build(sampleChunks())
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			q := "media releases automated"
			if id%2 == 0 {
				q = "supported download clients"
			}
			hits, sErr := ix.Search(q, DefaultOptions())
			if sErr != nil {
				t.Errorf("concurrent Search error: %v", sErr)
			}
			if len(hits) == 0 {
				t.Errorf("concurrent Search returned 0 hits")
			}
		}(i)
	}
	wg.Wait()
}

func TestSearch_Negative_ValidationErrors(t *testing.T) {
	ix, err := Build(sampleChunks())
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	cases := []struct {
		name    string
		query   string
		opt     Options
		wantErr string
	}{
		{"limit zero", "query", Options{Limit: 0, MinScore: 0.1}, "limit 0 outside 1..10"},
		{"limit over max", "query", Options{Limit: 11, MinScore: 0.1}, "limit 11 outside 1..10"},
		{"min score negative", "query", Options{Limit: 3, MinScore: -0.1}, "min score -0.100000 outside [0, 1]"},
		{"min score over one", "query", Options{Limit: 3, MinScore: 1.1}, "min score 1.100000 outside [0, 1]"},
		{"query too large", strings.Repeat("x", 2001), Options{Limit: 3, MinScore: 0.1}, "exceeds max 2000 bytes"},
		{"query bad utf8", string([]byte{0xff, 0xfe}), Options{Limit: 3, MinScore: 0.1}, "not valid UTF-8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, sErr := ix.Search(tc.query, tc.opt)
			if sErr == nil || !strings.Contains(sErr.Error(), tc.wantErr) {
				t.Fatalf("Search err = %v, want containing %q", sErr, tc.wantErr)
			}
		})
	}
}

func TestSearch_Boundary_LimitsAndEmptyQueries(t *testing.T) {
	ix, err := Build(sampleChunks())
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	// Zero tokens query -> empty slice, nil
	hits, err := ix.Search("   ", DefaultOptions())
	if err != nil || len(hits) != 0 {
		t.Errorf("Search blank query = %v, %v; want empty slice, nil", hits, err)
	}

	// Limit 1
	hits1, err := ix.Search("automated", Options{Limit: 1, MinScore: 0.0})
	if err != nil || len(hits1) != 1 {
		t.Errorf("Search Limit 1 returned %d hits, want 1", len(hits1))
	}

	// Limit MaxLimit (10)
	hits10, err := ix.Search("automated", Options{Limit: MaxLimit, MinScore: 0.0})
	if err != nil || len(hits10) > MaxLimit {
		t.Errorf("Search Limit MaxLimit returned %d hits", len(hits10))
	}

	// MinScore 1.0 (boundary)
	hitsMaxScore, err := ix.Search("completely unknown nonexistent token", Options{Limit: 3, MinScore: 1.0})
	if err != nil || len(hitsMaxScore) != 0 {
		t.Errorf("Search MinScore 1.0 for unknown = %d hits, want 0", len(hitsMaxScore))
	}
}
