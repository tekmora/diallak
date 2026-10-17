package eval

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/embed"
	"github.com/tekmora/diallak/internal/retrieval"
)

func loadSnatchArrFixture(t *testing.T) (Set, []corpus.Chunk, *retrieval.Index) {
	t.Helper()
	setFS := os.DirFS("testdata/snatcharr")
	set, err := LoadSet(setFS, "cases.json")
	if err != nil {
		t.Fatalf("LoadSet error: %v", err)
	}

	corpusFS := os.DirFS("testdata/snatcharr/corpus")
	chunks, err := corpus.Load(context.Background(), corpusFS, set.Corpus, corpus.DefaultLimits())
	if err != nil {
		t.Fatalf("corpus.Load error: %v", err)
	}

	if valErr := Validate(set, chunks); valErr != nil {
		t.Fatalf("Validate error: %v", valErr)
	}

	ix, err := retrieval.Build(chunks)
	if err != nil {
		t.Fatalf("retrieval.Build error: %v", err)
	}
	return set, chunks, ix
}

func TestSnatchArrEval(t *testing.T) {
	set, _, ix := loadSnatchArrFixture(t)
	rep, err := Run(context.Background(), set, ix, retrieval.DefaultOptions())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	t.Logf("answerable: %d/%d hits", rep.AnswerableHit, rep.Answerable)
	t.Logf("off_topic: %d/%d refused", rep.OffTopicRefused, rep.OffTopic)
	t.Logf("undocumented: %d/%d refused", rep.UndocumentedRefused, rep.Undocumented)

	for _, out := range rep.Outcomes {
		if !out.Pass {
			if len(out.Hits) > 0 {
				top := out.Hits[0]
				t.Logf("failing case %s (%s): top hit %s (score %.4f)", out.Case.ID, out.Case.Kind, top.Chunk.Path, top.Score)
			} else {
				t.Logf("failing case %s (%s): no hits", out.Case.ID, out.Case.Kind)
			}
		}
	}

	bars := Bars{MinAnswerableHit: 28, MinOffTopicRefused: 5}
	if checkErr := Check(rep, bars); checkErr != nil {
		t.Fatalf("Check failed: %v", checkErr)
	}
}

const (
	recordedParaphraseHit = 2
	recordedEmbeddingsDue = true
)

func TestSnatchArrEval_Paraphrase(t *testing.T) {
	set, _, ix := loadSnatchArrFixture(t)
	rep, err := Run(context.Background(), set, ix, retrieval.DefaultOptions())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	t.Logf("paraphrase: %d/%d hits", rep.ParaphraseHit, rep.Paraphrase)

	for _, out := range rep.Outcomes {
		if out.Case.Kind == KindParaphrase && !out.Pass {
			if len(out.Hits) > 0 {
				top := out.Hits[0]
				t.Logf("missing case %s: top hit %s (score %.4f)", out.Case.ID, top.Chunk.Path, top.Score)
			} else {
				t.Logf("missing case %s: no hits", out.Case.ID)
			}
		}
	}

	misses := rep.Paraphrase - rep.ParaphraseHit
	due := EmbeddingsRecordDue(rep)
	t.Logf("misses: %d (max: %d), embeddings record due: %t", misses, MaxParaphraseMisses, due)

	if rep.ParaphraseHit != recordedParaphraseHit || due != recordedEmbeddingsDue {
		t.Fatalf("run changed: got hits %d due %t, want hits %d due %t; update README record and test constants",
			rep.ParaphraseHit, due, recordedParaphraseHit, recordedEmbeddingsDue)
	}
}

func TestSnatchArrEval_Sweep(t *testing.T) {
	set, _, ix := loadSnatchArrFixture(t)
	t.Log("minscore | answerable_hit | off_topic_refused | undocumented_refused")
	t.Log("---------|----------------|-------------------|---------------------")

	for i := 0; i <= 18; i++ {
		score := float64(i) * 0.05
		opt := retrieval.Options{Limit: 3, MinScore: score}
		rep, err := Run(context.Background(), set, ix, opt)
		if err != nil {
			t.Fatalf("Run score %.2f error: %v", score, err)
		}
		t.Logf("%8.2f | %14d | %17d | %20d", score, rep.AnswerableHit, rep.OffTopicRefused, rep.UndocumentedRefused)
	}
}

func TestSnatchArrEval_NegativeControls(t *testing.T) {
	set, _, ix := loadSnatchArrFixture(t)
	bars := Bars{MinAnswerableHit: 28, MinOffTopicRefused: 5}

	// Negative control (a): Stub returning no hits fails answerable bar
	noHitsRetriever := &stubRetriever{hits: nil}
	repNoHits, err := Run(context.Background(), set, noHitsRetriever, retrieval.DefaultOptions())
	if err != nil {
		t.Fatalf("Run noHits error: %v", err)
	}
	if checkErr := Check(repNoHits, bars); checkErr == nil {
		t.Error("Check with noHits stub passed, want error on answerable bar")
	}

	// Negative control (b): Real index with MinScore 0.0 fails off_topic bar if dropped below 5,
	// otherwise test with an all-hits stub.
	repZeroScore, err := Run(context.Background(), set, ix, retrieval.Options{Limit: 3, MinScore: 0.0})
	if err != nil {
		t.Fatalf("Run zero score error: %v", err)
	}
	if repZeroScore.OffTopicRefused < bars.MinOffTopicRefused {
		if checkErr := Check(repZeroScore, bars); checkErr == nil {
			t.Error("Check with MinScore 0.0 passed, want error on off_topic bar")
		}
	} else {
		allHitsStub := &stubRetriever{hits: []retrieval.Hit{{Chunk: corpus.Chunk{Path: "docs/dummy.md"}, Score: 1.0}}}
		repAllHits, rErr := Run(context.Background(), set, allHitsStub, retrieval.DefaultOptions())
		if rErr != nil {
			t.Fatalf("Run allHits error: %v", rErr)
		}
		if checkErr := Check(repAllHits, bars); checkErr == nil {
			t.Error("Check with allHits stub passed, want error on off_topic bar")
		}
	}
}

type hashEmbedder struct{}

func (hashEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i, text := range texts {
		vec := make([]float32, 256)
		tokens := retrieval.Tokenize(text)
		for _, tok := range tokens {
			h := fnv32(tok) % 256
			vec[h] += 1.0
		}
		var sumSq float64
		for _, v := range vec {
			sumSq += float64(v * v)
		}
		if sumSq > 0 {
			norm := float32(math.Sqrt(sumSq))
			for j := range vec {
				vec[j] /= norm
			}
		}
		vecs[i] = vec
	}
	return vecs, nil
}

func fnv32(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func TestSnatchArrEval_Hybrid(t *testing.T) {
	set, chunks, ix := loadSnatchArrFixture(t)
	hybrid, err := retrieval.NewHybrid(context.Background(), ix, chunks, hashEmbedder{}, retrieval.DefaultMinSimilarity)
	if err != nil {
		t.Fatalf("NewHybrid error: %v", err)
	}

	rep, err := Run(context.Background(), set, hybrid, retrieval.DefaultOptions())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	t.Logf("answerable: %d/%d hits", rep.AnswerableHit, rep.Answerable)
	t.Logf("off_topic: %d/%d refused", rep.OffTopicRefused, rep.OffTopic)
	t.Logf("undocumented: %d/%d refused", rep.UndocumentedRefused, rep.Undocumented)
	t.Logf("paraphrase: %d/%d hits", rep.ParaphraseHit, rep.Paraphrase)

	bars := Bars{MinAnswerableHit: 28, MinOffTopicRefused: 5}
	if checkErr := Check(rep, bars); checkErr != nil {
		t.Fatalf("Check failed: %v", checkErr)
	}
}

func TestSnatchArrEval_EmbedSweep(t *testing.T) {
	baseURL := os.Getenv("DIALLAK_EVAL_EMBED_BASE_URL")
	alias := os.Getenv("DIALLAK_EVAL_EMBED_ALIAS")
	if baseURL == "" || alias == "" {
		t.Skip("skipping embed sweep: DIALLAK_EVAL_EMBED_BASE_URL and DIALLAK_EVAL_EMBED_ALIAS must be set")
	}

	embClient, err := embed.New(embed.Config{
		BaseURL: baseURL,
		Alias:   alias,
	}, nil)
	if err != nil {
		t.Fatalf("embed.New: %v", err)
	}

	set, chunks, ix := loadSnatchArrFixture(t)
	bars := Bars{MinAnswerableHit: 28, MinOffTopicRefused: 5}

	t.Log("similarity | answerable_hit | off_topic_refused | undocumented_refused | paraphrase_hit | bars_hold")
	t.Log("-----------|----------------|-------------------|----------------------|----------------|----------")

	const sweepSteps = 11 // 0.30 .. 0.80 step 0.05
	for i := 0; i < sweepSteps; i++ {
		sim := 0.30 + float64(i)*0.05
		hybrid, hErr := retrieval.NewHybrid(context.Background(), ix, chunks, embClient, sim)
		if hErr != nil {
			t.Fatalf("NewHybrid similarity %.2f: %v", sim, hErr)
		}
		rep, rErr := Run(context.Background(), set, hybrid, retrieval.DefaultOptions())
		if rErr != nil {
			t.Fatalf("Run similarity %.2f: %v", sim, rErr)
		}
		barsHold := Check(rep, bars) == nil
		t.Logf("%10.2f | %14d | %17d | %20d | %14d | %9t",
			sim, rep.AnswerableHit, rep.OffTopicRefused, rep.UndocumentedRefused, rep.ParaphraseHit, barsHold)
	}
}
