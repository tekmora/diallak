package eval

import (
	"context"
	"math"
	"os"
	"strings"
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

// liveEmbedConfig reads the sweep's endpoint from the environment. The key file is optional;
// /dev/stdin lets a run pipe the key in without writing it to disk.
func liveEmbedConfig(t *testing.T, alias string) embed.Config {
	t.Helper()
	cfg := embed.Config{BaseURL: os.Getenv("DIALLAK_EVAL_EMBED_BASE_URL"), Alias: alias}
	keyFile := os.Getenv("DIALLAK_EVAL_EMBED_KEY_FILE")
	if keyFile == "" {
		return cfg
	}
	data, err := os.ReadFile(keyFile) // #nosec G304 -- the person running the sweep names the key file.
	if err != nil {
		t.Fatalf("read embed key file: %v", err)
	}
	cfg.APIKey = strings.TrimSpace(string(data))
	return cfg
}

// checkRefusedAliases builds the start-time index once per alias that the endpoint must refuse
// (a hosted model, or one the key may not call); each must fail, as the bot's start does.
func checkRefusedAliases(t *testing.T, base embed.Config, ix *retrieval.Index, chunks []corpus.Chunk) {
	t.Helper()
	list := os.Getenv("DIALLAK_EVAL_EMBED_REFUSED_ALIASES")
	if list == "" {
		return
	}
	for _, alias := range strings.Split(list, ",") {
		cfg := base
		cfg.Alias = strings.TrimSpace(alias)
		client, err := embed.New(cfg, nil)
		if err != nil {
			t.Fatalf("embed.New alias %q: %v", cfg.Alias, err)
		}
		_, hErr := retrieval.NewHybrid(context.Background(), ix, chunks, client, retrieval.DefaultMinSimilarity)
		if hErr == nil {
			t.Fatalf("alias %q: the endpoint answered; it must refuse this model", cfg.Alias)
		}
		t.Logf("alias %q refused: %s", cfg.Alias, embed.Classify(hErr))
	}
}

func TestSnatchArrEval_EmbedSweep(t *testing.T) {
	alias := os.Getenv("DIALLAK_EVAL_EMBED_ALIAS")
	if os.Getenv("DIALLAK_EVAL_EMBED_BASE_URL") == "" || alias == "" {
		t.Skip("skipping embed sweep: DIALLAK_EVAL_EMBED_BASE_URL and DIALLAK_EVAL_EMBED_ALIAS must be set")
	}
	cfg := liveEmbedConfig(t, alias)
	// The first batch may load the model on the route; the longest timeout the client allows
	// covers that cold start.
	cfg.Timeout = embed.MaxTimeout
	embClient, err := embed.New(cfg, nil)
	if err != nil {
		t.Fatalf("embed.New: %v", err)
	}

	set, chunks, ix := loadSnatchArrFixture(t)
	checkRefusedAliases(t, cfg, ix, chunks)
	emb := newSweepEmbedder(embClient, realClock{}, sweepCallInterval)
	if sweepErr := runEmbedSweep(t, set, chunks, ix, emb); sweepErr != nil {
		t.Fatalf("sweep: %v", sweepErr)
	}
	t.Logf("endpoint calls: %d", emb.calls)
}
