// Package retrieval provides in-memory lexical retrieval for documentation chunks.
package retrieval

import (
	"errors"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tekmora/discord-bot/internal/corpus"
)

const (
	// MaxLimit is the maximum number of hits that can be requested.
	MaxLimit = 10
	// MaxQueryBytes is the maximum allowed query size in bytes.
	MaxQueryBytes = 2000
	// WeightText is the term frequency multiplier for document body tokens.
	WeightText = 1
	// WeightHeading is the term frequency multiplier for heading tokens.
	WeightHeading = 3
	// WeightTitle is the term frequency multiplier for document title tokens.
	WeightTitle = 2
	// WeightPath is the term frequency multiplier for path base tokens.
	WeightPath = 2
	// BM25K1 is the BM25 term frequency saturation parameter.
	BM25K1 = 1.2
	// BM25B is the BM25 document length normalization parameter.
	BM25B = 0.75
	// MinMatchedTerms is the minimum number of distinct query terms a chunk must match to be eligible for retrieval.
	// One overlapping word is not a source.
	MinMatchedTerms = 2
	// DefaultMinScore is the default minimum score threshold for retrieval refusal.
	// Chosen from the sweep over MinScore 0.00..0.90 at Limit 3, where 0.20 maximized
	// answerable hits (29/32) subject to off-topic refusals >= 5 (6/6 refused,
	// undocumented refused 5/6; at 0.25 answerable hits drops to 28/32).
	DefaultMinScore = 0.20
)

// Hit pairs a retrieved documentation chunk with its coverage score in [0, 1].
// Results are ranked by raw BM25 score descending, while Score reflects query term coverage.
type Hit struct {
	Chunk corpus.Chunk
	Score float64
	rank  float64
}

// Options configures retrieval query execution parameters.
type Options struct {
	Limit    int
	MinScore float64
}

// DefaultOptions returns standard retrieval options with Limit 3 and DefaultMinScore.
func DefaultOptions() Options {
	return Options{
		Limit:    3,
		MinScore: DefaultMinScore,
	}
}

type posting struct {
	docIdx int
	tf     float64
}

// Index is an immutable lexical retrieval index. It is safe for concurrent callers to call Search.
type Index struct {
	chunks   []corpus.Chunk
	docLens  []float64
	avgdl    float64
	df       map[string]int
	postings map[string][]posting
	n        int
}

// Build creates an immutable Index from chunks.
func Build(chunks []corpus.Chunk) (*Index, error) {
	if len(chunks) == 0 {
		return nil, errors.New("chunks slice is empty")
	}
	if len(chunks) > 8192 {
		return nil, fmt.Errorf("chunks count %d exceeds maximum 8192", len(chunks))
	}

	seen := make(map[string]struct{}, len(chunks))
	for _, c := range chunks {
		if _, ok := seen[c.ID]; ok {
			return nil, fmt.Errorf("duplicate chunk ID %q", c.ID)
		}
		seen[c.ID] = struct{}{}
	}

	ix := &Index{
		chunks:   append([]corpus.Chunk(nil), chunks...),
		docLens:  make([]float64, len(chunks)),
		df:       make(map[string]int),
		postings: make(map[string][]posting),
		n:        len(chunks),
	}
	ix.indexChunks()
	return ix, nil
}

func (ix *Index) indexChunks() {
	var totalLen float64
	for i, c := range ix.chunks {
		bag := buildChunkBag(c)
		docLen := 0.0
		for tok, count := range bag {
			tf := float64(count)
			docLen += tf
			ix.df[tok]++
			ix.postings[tok] = append(ix.postings[tok], posting{docIdx: i, tf: tf})
		}
		ix.docLens[i] = docLen
		totalLen += docLen
	}
	ix.avgdl = totalLen / float64(ix.n)
	if ix.avgdl == 0 {
		ix.avgdl = 1.0
	}
}

func buildChunkBag(c corpus.Chunk) map[string]int {
	bag := make(map[string]int)
	addTokens(bag, Tokenize(c.Text), WeightText)
	addTokens(bag, Tokenize(c.Heading), WeightHeading)
	addTokens(bag, Tokenize(c.Title), WeightTitle)
	base := strings.TrimSuffix(path.Base(c.Path), path.Ext(c.Path))
	addTokens(bag, Tokenize(base), WeightPath)
	return bag
}

func addTokens(bag map[string]int, tokens []string, weight int) {
	for _, t := range tokens {
		bag[t] += weight
	}
}

// Search retrieves chunks matching query according to BM25 ranking and coverage threshold.
// Eligible chunks must match at least MinMatchedTerms distinct query terms (or all query terms if fewer).
// Results are ranked by raw unclipped BM25 score descending, with ties broken by chunk ID ascending.
func (ix *Index) Search(query string, opt Options) ([]Hit, error) {
	if err := validateSearchOptions(query, opt); err != nil {
		return nil, err
	}

	tokens := Tokenize(query)
	if len(tokens) == 0 {
		return []Hit{}, nil
	}

	uniqueTokens := deduplicateTokens(tokens)
	weightQ := ix.calculateWeightQ(uniqueTokens)
	if weightQ == 0 {
		return []Hit{}, nil
	}

	hits := ix.collectHits(uniqueTokens, weightQ, opt)
	sortHits(hits)
	if len(hits) > opt.Limit {
		hits = hits[:opt.Limit]
	}
	return hits, nil
}

func validateSearchOptions(query string, opt Options) error {
	if opt.Limit < 1 || opt.Limit > MaxLimit {
		return fmt.Errorf("limit %d outside 1..%d", opt.Limit, MaxLimit)
	}
	if opt.MinScore < 0.0 || opt.MinScore > 1.0 {
		return fmt.Errorf("min score %f outside [0, 1]", opt.MinScore)
	}
	if len(query) > MaxQueryBytes {
		return fmt.Errorf("query length %d exceeds max %d bytes", len(query), MaxQueryBytes)
	}
	if !utf8.ValidString(query) {
		return errors.New("query is not valid UTF-8")
	}
	return nil
}

func deduplicateTokens(tokens []string) []string {
	var unique []string
	seen := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			unique = append(unique, t)
		}
	}
	return unique
}

func (ix *Index) calculateWeightQ(tokens []string) float64 {
	var weightQ float64
	idfAbsent := math.Log(1.0 + (float64(ix.n)+0.5)/0.5)
	for _, t := range tokens {
		if count, ok := ix.df[t]; ok && count > 0 {
			weightQ += math.Log(1.0 + (float64(ix.n-count)+0.5)/(float64(count)+0.5))
		} else {
			weightQ += idfAbsent
		}
	}
	return weightQ
}

type docAccum struct {
	rawBM25      float64
	coverageIDF  float64
	matchedTerms int
}

func (ix *Index) collectHits(uniqueTokens []string, weightQ float64, opt Options) []Hit {
	minMatched := MinMatchedTerms
	if len(uniqueTokens) < minMatched {
		minMatched = len(uniqueTokens)
	}

	accums := make([]docAccum, ix.n)
	for _, t := range uniqueTokens {
		count, ok := ix.df[t]
		if !ok || count == 0 {
			continue
		}
		idf := math.Log(1.0 + (float64(ix.n-count)+0.5)/(float64(count)+0.5))
		for _, p := range ix.postings[t] {
			docLen := ix.docLens[p.docIdx]
			denom := p.tf + BM25K1*(1.0-BM25B+BM25B*(docLen/ix.avgdl))
			bm25TF := (p.tf * (BM25K1 + 1.0)) / denom
			accums[p.docIdx].rawBM25 += idf * bm25TF
			accums[p.docIdx].coverageIDF += idf
			accums[p.docIdx].matchedTerms++
		}
	}

	var hits []Hit
	for i := range accums {
		if accums[i].matchedTerms < minMatched {
			continue
		}
		score := accums[i].coverageIDF / weightQ
		if score >= opt.MinScore {
			hits = append(hits, Hit{
				Chunk: ix.chunks[i],
				Score: score,
				rank:  accums[i].rawBM25,
			})
		}
	}
	return hits
}

func sortHits(hits []Hit) {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank > hits[j].rank
		}
		return hits[i].Chunk.ID < hits[j].Chunk.ID
	})
}
