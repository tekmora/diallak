package retrieval

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/tekmora/diallak/internal/corpus"
)

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// It returns 0 if either vector has zero magnitude or if vector lengths differ.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := 0; i < len(a); i++ {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// Hybrid combines in-memory lexical retrieval with semantic cosine similarity over embeddings.
type Hybrid struct {
	ix            *Index
	chunks        []corpus.Chunk
	vectors       [][]float32
	emb           Embedder
	minSimilarity float64
}

// NewHybrid pre-computes chunk embeddings and builds a Hybrid retriever.
// Chunks are embedded in batches of at most 64. Fails if the embedder fails at startup.
func NewHybrid(ctx context.Context, ix *Index, chunks []corpus.Chunk, emb Embedder, minSimilarity float64) (*Hybrid, error) {
	if ix == nil {
		return nil, errors.New("retrieval index is required")
	}
	if len(chunks) == 0 {
		return nil, errors.New("chunks slice is empty")
	}
	if emb == nil {
		return nil, errors.New("embedder is required")
	}
	if minSimilarity == 0 {
		minSimilarity = DefaultMinSimilarity
	}
	if minSimilarity < 0 || minSimilarity > 1.0 {
		return nil, fmt.Errorf("min similarity %g outside (0, 1]", minSimilarity)
	}

	vectors, err := embedAllChunks(ctx, chunks, emb)
	if err != nil {
		return nil, err
	}

	return &Hybrid{
		ix:            ix,
		chunks:        append([]corpus.Chunk(nil), chunks...),
		vectors:       vectors,
		emb:           emb,
		minSimilarity: minSimilarity,
	}, nil
}

const batchSize = 64

func embedAllChunks(ctx context.Context, chunks []corpus.Chunk, emb Embedder) ([][]float32, error) {
	vectors := make([][]float32, len(chunks))
	for start := 0; start < len(chunks); start += batchSize {
		end := min(start+batchSize, len(chunks))
		batchTexts := make([]string, end-start)
		for i := start; i < end; i++ {
			batchTexts[i-start] = chunks[i].Text
		}

		batchVecs, err := emb.Embed(ctx, batchTexts)
		if err != nil {
			return nil, fmt.Errorf("embed chunks batch [%d:%d]: %w", start, end, err)
		}
		if len(batchVecs) != end-start {
			return nil, fmt.Errorf("expected %d chunk vectors, got %d", end-start, len(batchVecs))
		}
		for i := start; i < end; i++ {
			vectors[i] = batchVecs[i-start]
		}
	}
	return vectors, nil
}

// VectorDim returns the dimension of stored chunk vectors, or 0 when empty.
func (h *Hybrid) VectorDim() int {
	if len(h.vectors) == 0 {
		return 0
	}
	return len(h.vectors[0])
}

// ChunkCount returns the number of indexed chunks.
func (h *Hybrid) ChunkCount() int {
	return len(h.chunks)
}

// EmbedError reports that the query embedding failed and retrieval fell back to the lexical
// hits returned beside it. It is not fatal: the caller answers from those hits, counts the
// failure and logs its class only.
type EmbedError struct {
	Err error
}

func (e *EmbedError) Error() string {
	return "embed query: " + e.Err.Error()
}

func (e *EmbedError) Unwrap() error {
	return e.Err
}

// SearchMode executes hybrid retrieval: lexical search merged with cosine similarity over embeddings.
// On embedder failure, it returns the lexical hits with ModeKeywordFallback and an *EmbedError.
func (h *Hybrid) SearchMode(ctx context.Context, q string, opt Options) ([]Hit, Mode, error) {
	lexHits, err := h.ix.Search(q, opt)
	if err != nil {
		return nil, ModeKeywordFallback, err
	}

	qVecs, err := h.emb.Embed(ctx, []string{q})
	if err != nil {
		return lexHits, ModeKeywordFallback, &EmbedError{Err: err}
	}
	if len(qVecs) != 1 {
		return lexHits, ModeKeywordFallback, &EmbedError{Err: fmt.Errorf("expected 1 query vector, got %d", len(qVecs))}
	}

	return h.mergeHits(lexHits, qVecs[0], opt), ModeHybrid, nil
}

func (h *Hybrid) mergeHits(lexHits []Hit, qVec []float32, opt Options) []Hit {
	seen := make(map[string]struct{}, len(lexHits))
	for _, hit := range lexHits {
		seen[hit.Chunk.ID] = struct{}{}
	}

	var semHits []Hit
	for i := 0; i < len(h.chunks); i++ {
		c := h.chunks[i]
		if _, ok := seen[c.ID]; ok {
			continue
		}
		sim := CosineSimilarity(qVec, h.vectors[i])
		if sim >= h.minSimilarity {
			semHits = append(semHits, Hit{
				Chunk:      c,
				Score:      0,
				Similarity: sim,
			})
		}
	}

	sort.Slice(semHits, func(i, j int) bool {
		if semHits[i].Similarity != semHits[j].Similarity {
			return semHits[i].Similarity > semHits[j].Similarity
		}
		return semHits[i].Chunk.ID < semHits[j].Chunk.ID
	})

	merged := append(lexHits, semHits...)
	if len(merged) > opt.Limit {
		merged = merged[:opt.Limit]
	}
	if len(merged) == 0 {
		return []Hit{}
	}
	return merged
}
