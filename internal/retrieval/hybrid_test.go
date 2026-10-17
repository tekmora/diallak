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

// tableEmbedder returns the vector table holds for each text, and zero vectors of dim
// dimensions for texts it does not hold.
func tableEmbedder(table map[string][]float32, dim int) *fakeTestEmbedder {
	return &fakeTestEmbedder{
		embedFunc: func(_ context.Context, texts []string) ([][]float32, error) {
			vecs := make([][]float32, len(texts))
			for i, text := range texts {
				vec, ok := table[text]
				if !ok {
					vec = make([]float32, dim)
				}
				vecs[i] = vec
			}
			return vecs, nil
		},
	}
}

func hybridFixture(t *testing.T) ([]corpus.Chunk, *Index) {
	t.Helper()
	chunks := []corpus.Chunk{
		{ID: "c1", Title: "Installation", Text: "Install the software using apt."},
		{ID: "c2", Title: "Configuration", Text: "Configure the settings in config.yaml."},
		{ID: "c3", Title: "Troubleshooting", Text: "Fix connection issues and network errors."},
	}
	ix, err := Build(chunks)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return chunks, ix
}

func fixtureEmbedder(chunks []corpus.Chunk) *fakeTestEmbedder {
	return tableEmbedder(map[string][]float32{
		chunks[0].Text: {1, 0, 0},
		chunks[1].Text: {0, 1, 0},
		chunks[2].Text: {0, 0, 1},
	}, 3)
}

func TestNewHybrid_Positive(t *testing.T) {
	chunks, ix := hybridFixture(t)
	h, err := NewHybrid(context.Background(), ix, chunks, fixtureEmbedder(chunks), 0.5)
	if err != nil {
		t.Fatalf("NewHybrid: %v", err)
	}
	if h.ChunkCount() != 3 || h.VectorDim() != 3 {
		t.Errorf("ChunkCount, VectorDim = %d, %d, want 3, 3", h.ChunkCount(), h.VectorDim())
	}
}

func TestNewHybrid_Negative(t *testing.T) {
	chunks, ix := hybridFixture(t)
	emb := fixtureEmbedder(chunks)
	failing := &fakeTestEmbedder{
		embedFunc: func(context.Context, []string) ([][]float32, error) {
			return nil, errors.New("embedder unreachable")
		},
	}
	cases := []struct {
		name   string
		ix     *Index
		chunks []corpus.Chunk
		emb    Embedder
		minSim float64
	}{
		{"nil index", nil, chunks, emb, 0.5},
		{"nil chunks", ix, nil, emb, 0.5},
		{"nil embedder", ix, chunks, nil, 0.5},
		{"negative similarity", ix, chunks, emb, -0.1},
		{"similarity above one", ix, chunks, emb, 1.5},
		{"embedder fails at start", ix, chunks, failing, 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewHybrid(context.Background(), tc.ix, tc.chunks, tc.emb, tc.minSim); err == nil {
				t.Error("NewHybrid returned nil error")
			}
		})
	}
}

func TestIndex_SearchMode_Keyword(t *testing.T) {
	_, ix := hybridFixture(t)
	hits, mode, err := ix.SearchMode(context.Background(), "install software", DefaultOptions())
	if err != nil || mode != ModeKeyword || len(hits) == 0 {
		t.Errorf("ix.SearchMode = %v, %v, %v", hits, mode, err)
	}
}

func TestHybrid_SearchMode_LexicalFirst(t *testing.T) {
	chunks, ix := hybridFixture(t)
	h, err := NewHybrid(context.Background(), ix, chunks, fixtureEmbedder(chunks), 0.5)
	if err != nil {
		t.Fatalf("NewHybrid: %v", err)
	}
	hits, mode, err := h.SearchMode(context.Background(), "install software", DefaultOptions())
	if err != nil || mode != ModeHybrid {
		t.Fatalf("SearchMode = %v, %v, want ModeHybrid, nil", mode, err)
	}
	if len(hits) == 0 || hits[0].Chunk.ID != "c1" {
		t.Errorf("hits = %v, want c1 first (lexical first)", hits)
	}
}

func TestHybrid_SearchMode_FallbackOnEmbedFailure(t *testing.T) {
	chunks, ix := hybridFixture(t)
	failingQuery := &fakeTestEmbedder{
		embedFunc: func(_ context.Context, texts []string) ([][]float32, error) {
			if len(texts) == 1 && texts[0] == "install software" {
				return nil, errors.New("timeout connecting to embed endpoint")
			}
			res := make([][]float32, len(texts))
			for i := range texts {
				res[i] = []float32{1, 0, 0}
			}
			return res, nil
		},
	}
	h, err := NewHybrid(context.Background(), ix, chunks, failingQuery, 0.5)
	if err != nil {
		t.Fatalf("NewHybrid: %v", err)
	}
	hits, mode, err := h.SearchMode(context.Background(), "install software", DefaultOptions())
	var embErr *EmbedError
	if !errors.As(err, &embErr) || mode != ModeKeywordFallback {
		t.Errorf("SearchMode = %v, %v, want ModeKeywordFallback and *EmbedError", mode, err)
	}
	if len(hits) == 0 || hits[0].Chunk.ID != "c1" {
		t.Errorf("hits = %v, want lexical hit c1", hits)
	}
}
