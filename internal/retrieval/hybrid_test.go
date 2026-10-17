package retrieval

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/tekmora/diallak/internal/corpus"
)

type fakeTestEmbedder struct {
	embedFunc func(ctx context.Context, texts []string) ([][]float32, error)
}

func (f *fakeTestEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if f.embedFunc != nil {
		return f.embedFunc(ctx, texts)
	}
	res := make([][]float32, len(texts))
	for i := range texts {
		res[i] = []float32{1.0, 0.0}
	}
	return res, nil
}

func TestCosineSimilarity(t *testing.T) {
	// Identical
	if sim := CosineSimilarity([]float32{1, 0}, []float32{1, 0}); math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("identical: got %f, want 1.0", sim)
	}
	// Orthogonal
	if sim := CosineSimilarity([]float32{1, 0}, []float32{0, 1}); math.Abs(sim-0.0) > 1e-6 {
		t.Errorf("orthogonal: got %f, want 0.0", sim)
	}
	// Opposite
	if sim := CosineSimilarity([]float32{1, 0}, []float32{-1, 0}); math.Abs(sim-(-1.0)) > 1e-6 {
		t.Errorf("opposite: got %f, want -1.0", sim)
	}
	// Zero vectors
	if sim := CosineSimilarity([]float32{0, 0}, []float32{1, 1}); sim != 0 {
		t.Errorf("zero vector a: got %f, want 0", sim)
	}
	if sim := CosineSimilarity([]float32{1, 1}, []float32{0, 0}); sim != 0 {
		t.Errorf("zero vector b: got %f, want 0", sim)
	}
	// Unequal lengths
	if sim := CosineSimilarity([]float32{1}, []float32{1, 2}); sim != 0 {
		t.Errorf("unequal lengths: got %f, want 0", sim)
	}
	// Empty
	if sim := CosineSimilarity(nil, nil); sim != 0 {
		t.Errorf("empty: got %f, want 0", sim)
	}
}

func TestNewHybrid_PositiveAndNegative(t *testing.T) {
	chunks := []corpus.Chunk{
		{ID: "c1", Title: "Installation", Text: "Install the software using apt."},
		{ID: "c2", Title: "Configuration", Text: "Configure the settings in config.yaml."},
	}
	ix, err := Build(chunks)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	emb := &fakeTestEmbedder{
		embedFunc: func(ctx context.Context, texts []string) ([][]float32, error) {
			vecs := make([][]float32, len(texts))
			for i, text := range texts {
				if text == chunks[0].Text {
					vecs[i] = []float32{1.0, 0.0}
				} else {
					vecs[i] = []float32{0.0, 1.0}
				}
			}
			return vecs, nil
		},
	}

	h, err := NewHybrid(context.Background(), ix, chunks, emb, 0.5)
	if err != nil {
		t.Fatalf("NewHybrid: %v", err)
	}
	if h.ChunkCount() != 2 {
		t.Errorf("ChunkCount = %d, want 2", h.ChunkCount())
	}
	if h.VectorDim() != 2 {
		t.Errorf("VectorDim = %d, want 2", h.VectorDim())
	}

	// Negative controls
	if _, err := NewHybrid(context.Background(), nil, chunks, emb, 0.5); err == nil {
		t.Error("NewHybrid with nil index returned nil error")
	}
	if _, err := NewHybrid(context.Background(), ix, nil, emb, 0.5); err == nil {
		t.Error("NewHybrid with nil chunks returned nil error")
	}
	if _, err := NewHybrid(context.Background(), ix, chunks, nil, 0.5); err == nil {
		t.Error("NewHybrid with nil embedder returned nil error")
	}
	if _, err := NewHybrid(context.Background(), ix, chunks, emb, -0.1); err == nil {
		t.Error("NewHybrid with negative minSimilarity returned nil error")
	}
	if _, err := NewHybrid(context.Background(), ix, chunks, emb, 1.5); err == nil {
		t.Error("NewHybrid with minSimilarity > 1 returned nil error")
	}

	// Failing embedder at startup stops the start
	failingEmb := &fakeTestEmbedder{
		embedFunc: func(ctx context.Context, texts []string) ([][]float32, error) {
			return nil, errors.New("embedder unreachable")
		},
	}
	if _, err := NewHybrid(context.Background(), ix, chunks, failingEmb, 0.5); err == nil {
		t.Error("NewHybrid with failing embedder returned nil error")
	}
}

func TestHybrid_SearchMode(t *testing.T) {
	chunks := []corpus.Chunk{
		{ID: "c1", Title: "Installation", Text: "Install the software using apt."},
		{ID: "c2", Title: "Configuration", Text: "Configure the settings in config.yaml."},
		{ID: "c3", Title: "Troubleshooting", Text: "Fix connection issues and network errors."},
	}
	ix, err := Build(chunks)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Vector assignments:
	// c1: [1, 0, 0]
	// c2: [0, 1, 0]
	// c3: [0, 0, 1]
	emb := &fakeTestEmbedder{
		embedFunc: func(ctx context.Context, texts []string) ([][]float32, error) {
			vecs := make([][]float32, len(texts))
			for i, txt := range texts {
				switch txt {
				case chunks[0].Text:
					vecs[i] = []float32{1, 0, 0}
				case chunks[1].Text:
					vecs[i] = []float32{0, 1, 0}
				case chunks[2].Text:
					vecs[i] = []float32{0, 0, 1}
				case "query for troubleshoot":
					// Query vector aligned with c3
					vecs[i] = []float32{0, 0, 1}
				default:
					vecs[i] = []float32{0, 0, 0}
				}
			}
			return vecs, nil
		},
	}

	h, err := NewHybrid(context.Background(), ix, chunks, emb, 0.5)
	if err != nil {
		t.Fatalf("NewHybrid: %v", err)
	}

	// 1. Index SearchMode returns ModeKeyword
	ixHits, mode, err := ix.SearchMode(context.Background(), "install software", DefaultOptions())
	if err != nil || mode != ModeKeyword || len(ixHits) == 0 {
		t.Errorf("ix.SearchMode = %v, %v, %v", ixHits, mode, err)
	}

	// 2. Hybrid SearchMode succeeds: merges lexical and semantic
	// "install software" matches c1 lexically. Query also gets semantic hits.
	hits, mode, err := h.SearchMode(context.Background(), "install software", DefaultOptions())
	if err != nil {
		t.Fatalf("SearchMode: %v", err)
	}
	if mode != ModeHybrid {
		t.Errorf("mode = %v, want ModeHybrid", mode)
	}
	if len(hits) == 0 {
		t.Fatal("got 0 hits, want >= 1")
	}
	if hits[0].Chunk.ID != "c1" {
		t.Errorf("first hit = %s, want c1 (lexical first)", hits[0].Chunk.ID)
	}

	// 3. Hybrid SearchMode when embedder fails at query time: fallback to lexical with ModeKeywordFallback
	failingEmb := &fakeTestEmbedder{
		embedFunc: func(ctx context.Context, texts []string) ([][]float32, error) {
			if len(texts) == 1 && texts[0] == "install software" {
				return nil, errors.New("timeout connecting to embed endpoint")
			}
			// Startup embedding succeeds
			res := make([][]float32, len(texts))
			for i := range texts {
				res[i] = []float32{1, 0, 0}
			}
			return res, nil
		},
	}
	hFailing, err := NewHybrid(context.Background(), ix, chunks, failingEmb, 0.5)
	if err != nil {
		t.Fatalf("NewHybrid: %v", err)
	}
	fbHits, fbMode, fbErr := hFailing.SearchMode(context.Background(), "install software", DefaultOptions())
	var embErr *EmbedError
	if !errors.As(fbErr, &embErr) {
		t.Errorf("expected *EmbedError on embed failure fallback, got %v", fbErr)
	}
	if fbMode != ModeKeywordFallback {
		t.Errorf("mode = %v, want ModeKeywordFallback", fbMode)
	}
	if len(fbHits) == 0 || fbHits[0].Chunk.ID != "c1" {
		t.Errorf("fbHits = %v, want lexical hit c1", fbHits)
	}
}
