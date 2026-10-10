package retrieval

import (
	"fmt"
	"math"
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

func TestSearch_PartA_Ranking_StrongerChunkLargerIDComesFirst(t *testing.T) {
	chunks := []corpus.Chunk{
		{
			ID:      "chunk-a-weaker",
			Path:    "docs/a.md",
			Title:   "A",
			Heading: "Heading",
			Text:    "media release automated",
		},
		{
			ID:      "chunk-z-stronger",
			Path:    "docs/z.md",
			Title:   "Z",
			Heading: "media release automated",
			Text:    "media release automated",
		},
	}
	ix, err := Build(chunks)
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}

	hits, err := ix.Search("media release automated", Options{Limit: 2, MinScore: 0.0})
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("Search returned %d hits, want 2", len(hits))
	}
	if hits[0].Chunk.ID != "chunk-z-stronger" {
		t.Fatalf("top hit chunk ID = %q, want %q", hits[0].Chunk.ID, "chunk-z-stronger")
	}
}

func TestSearch_PartA_OneWordOverlap_RequiresTwoMatchedTerms(t *testing.T) {
	ix, err := Build(sampleChunks())
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	// Query has two content tokens ("helm", "download").
	// docs/deployment.md has "helm", docs/download-clients.md has "download".
	// Neither chunk matches both tokens, so MinMatchedTerms=2 must reject both at MinScore 0.
	hits, err := ix.Search("helm download", Options{Limit: 3, MinScore: 0.0})
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("Search returned %d hits for one-word overlap, want 0 hits", len(hits))
	}
}

func TestSearch_PartA_SingleTokenQuery_OneMatchIsEligible(t *testing.T) {
	ix, err := Build(sampleChunks())
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	// Single-token query matches transmission in docs/download-clients.md#1.
	// Since query has only 1 unique token, min(MinMatchedTerms, 1) = 1 is required.
	// Coverage score must be 1.0 (binary presence idf / weightQ) and eligible even at MinScore 1.0.
	hits, err := ix.Search("transmission", Options{Limit: 3, MinScore: 1.0})
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("Search returned %d hits for single token query, want 1", len(hits))
	}
	if math.Abs(hits[0].Score-1.0) > 1e-9 {
		t.Fatalf("hit score = %f, want 1.0", hits[0].Score)
	}
}

func TestSearch_PartA_CoverageValue_HandComputedThreeChunkCorpus(t *testing.T) {
	// Corpus: N = 3 chunks
	// c0: "apple banana"
	// c1: "apple cherry"
	// c2: "date elderberry"
	//
	// Query: "apple cherry fig" (3 unique tokens)
	//
	// Term frequencies and IDFs:
	// - "apple": df = 2, idf = ln(1 + (3 - 2 + 0.5) / (2 + 0.5)) = ln(1 + 1.5/2.5) = ln(1.6)
	// - "cherry": df = 1, idf = ln(1 + (3 - 1 + 0.5) / (1 + 0.5)) = ln(1 + 2.5/1.5) = ln(8/3)
	// - "fig": df = 0 (absent), idfAbsent = ln(1 + (3 + 0.5) / 0.5) = ln(8)
	//
	// Query weight:
	// weight(q) = ln(1.6) + ln(8/3) + ln(8)
	//
	// Chunk c1 contains "apple" and "cherry" (2 distinct terms matched >= min(2, 3)).
	// Sum of IDF of matched query terms in c1 = ln(1.6) + ln(8/3)
	// Expected coverage Score = (ln(1.6) + ln(8/3)) / (ln(1.6) + ln(8/3) + ln(8))
	chunks := []corpus.Chunk{
		{ID: "c0", Path: "docs/c0.md", Text: "apple banana"},
		{ID: "c1", Path: "docs/c1.md", Text: "apple cherry"},
		{ID: "c2", Path: "docs/c2.md", Text: "date elderberry"},
	}
	ix, err := Build(chunks)
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}

	hits, err := ix.Search("apple cherry fig", Options{Limit: 3, MinScore: 0.0})
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("Search returned %d hits, want 1 (c1)", len(hits))
	}
	if hits[0].Chunk.ID != "c1" {
		t.Fatalf("top hit chunk ID = %q, want c1", hits[0].Chunk.ID)
	}

	idfApple := math.Log(1.6)
	idfCherry := math.Log(8.0 / 3.0)
	idfFigAbsent := math.Log(8.0)
	weightQ := idfApple + idfCherry + idfFigAbsent
	wantScore := (idfApple + idfCherry) / weightQ

	if math.Abs(hits[0].Score-wantScore) > 1e-9 {
		t.Fatalf("c1 score = %.12f, want %.12f (diff %e)", hits[0].Score, wantScore, math.Abs(hits[0].Score-wantScore))
	}
}
