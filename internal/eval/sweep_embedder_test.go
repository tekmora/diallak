package eval

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/embed"
	"github.com/tekmora/diallak/internal/retrieval"
)

// The live route's key allows 60 requests a minute. The sweep spaces its calls so that at most
// 55 fall in any minute, and backs off on 429 for a bounded number of tries.
const (
	sweepCallInterval  = 1100 * time.Millisecond
	sweepMaxRetries    = 5
	sweepFirstBackoff  = 2 * time.Second
	stubRequestsPerMin = 60
)

// sweepClock is the time source of the sweep's pacing; tests replace it with a fake one.
type sweepClock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// sweepEmbedder embeds each text once and serves it from memory afterwards, so the eleven
// similarity steps of the sweep cost one embedding per chunk and per query. Every call that
// reaches the endpoint waits for the interval since the previous one; a 429 is retried after a
// doubling pause, at most sweepMaxRetries times.
type sweepEmbedder struct {
	next     retrieval.Embedder
	clock    sweepClock
	interval time.Duration
	cache    map[string][]float32
	last     time.Time
	calls    int
}

func newSweepEmbedder(next retrieval.Embedder, clock sweepClock, interval time.Duration) *sweepEmbedder {
	return &sweepEmbedder{next: next, clock: clock, interval: interval, cache: map[string][]float32{}}
}

func (s *sweepEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	var missing []string
	queued := map[string]struct{}{}
	for _, text := range texts {
		_, cached := s.cache[text]
		_, seen := queued[text]
		if !cached && !seen {
			queued[text] = struct{}{}
			missing = append(missing, text)
		}
	}
	if len(missing) > 0 {
		vecs, err := s.fetch(ctx, missing)
		if err != nil {
			return nil, err
		}
		if len(vecs) != len(missing) {
			return nil, fmt.Errorf("expected %d vectors, got %d", len(missing), len(vecs))
		}
		for i, text := range missing {
			s.cache[text] = vecs[i]
		}
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = s.cache[text]
	}
	return out, nil
}

func (s *sweepEmbedder) fetch(ctx context.Context, texts []string) ([][]float32, error) {
	backoff := sweepFirstBackoff
	for attempt := 0; attempt <= sweepMaxRetries; attempt++ {
		if err := s.pace(ctx); err != nil {
			return nil, err
		}
		vecs, err := s.next.Embed(ctx, texts)
		var statusErr *embed.StatusError
		if !errors.As(err, &statusErr) || statusErr.Code != http.StatusTooManyRequests {
			return vecs, err
		}
		if sleepErr := s.clock.Sleep(ctx, backoff); sleepErr != nil {
			return nil, sleepErr
		}
		backoff *= 2
	}
	return nil, fmt.Errorf("embedding endpoint still returned 429 after %d retries", sweepMaxRetries)
}

func (s *sweepEmbedder) pace(ctx context.Context) error {
	if s.calls > 0 {
		if wait := s.last.Add(s.interval).Sub(s.clock.Now()); wait > 0 {
			if err := s.clock.Sleep(ctx, wait); err != nil {
				return err
			}
		}
	}
	s.last = s.clock.Now()
	s.calls++
	return nil
}

// runEmbedSweep builds the hybrid index for each similarity step and runs the set against it,
// logging one table row per step. Callers pass an embedder that caches, so only the first step
// reaches the endpoint for the chunks.
func runEmbedSweep(t *testing.T, set Set, chunks []corpus.Chunk, ix *retrieval.Index, emb retrieval.Embedder) error {
	t.Helper()
	bars := Bars{MinAnswerableHit: 28, MinOffTopicRefused: 5}
	t.Log("similarity | answerable_hit | off_topic_refused | undocumented_refused | paraphrase_hit | bars_hold")
	t.Log("-----------|----------------|-------------------|----------------------|----------------|----------")

	const sweepSteps = 11 // 0.30 .. 0.80 step 0.05
	for i := 0; i < sweepSteps; i++ {
		sim := 0.30 + float64(i)*0.05
		hybrid, err := retrieval.NewHybrid(context.Background(), ix, chunks, emb, sim)
		if err != nil {
			return fmt.Errorf("NewHybrid similarity %.2f: %w", sim, err)
		}
		rep, err := Run(context.Background(), set, hybrid, retrieval.DefaultOptions())
		if err != nil {
			return fmt.Errorf("Run similarity %.2f: %w", sim, err)
		}
		t.Logf("%10.2f | %14d | %17d | %20d | %14d | %9t",
			sim, rep.AnswerableHit, rep.OffTopicRefused, rep.UndocumentedRefused, rep.ParaphraseHit, Check(rep, bars) == nil)
	}
	return nil
}

// fakeClock advances only when the code under test sleeps.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.now = c.now.Add(d)
	return nil
}

// rateStub stands in for the gateway key: it answers with 429 once a call would make more than
// stubRequestsPerMin calls in the trailing minute, and otherwise embeds with hashEmbedder.
type rateStub struct {
	clock    *fakeClock
	stamps   []time.Time
	refused  int
	answered int
	limit429 int // calls refused before the stub answers again; 0 = refuse only over the rate
}

func (r *rateStub) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	now := r.clock.Now()
	recent := r.stamps[:0]
	for _, stamp := range r.stamps {
		if now.Sub(stamp) < time.Minute {
			recent = append(recent, stamp)
		}
	}
	r.stamps = append(recent, now)
	if len(r.stamps) > stubRequestsPerMin || r.refused < r.limit429 {
		r.refused++
		return nil, &embed.StatusError{Code: http.StatusTooManyRequests}
	}
	r.answered++
	return hashEmbedder{}.Embed(ctx, texts)
}

func TestSweepEmbedder_SweepStaysUnderRate(t *testing.T) {
	set, chunks, ix := loadSnatchArrFixture(t)
	clock := &fakeClock{now: time.Unix(0, 0)}
	stub := &rateStub{clock: clock}
	emb := newSweepEmbedder(stub, clock, sweepCallInterval)

	if err := runEmbedSweep(t, set, chunks, ix, emb); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if stub.refused != 0 {
		t.Fatalf("stub refused %d calls with 429; the sweep must stay under %d a minute", stub.refused, stubRequestsPerMin)
	}
	wantMax := len(set.Cases) + (len(chunks)+63)/64
	if stub.answered > wantMax {
		t.Fatalf("stub answered %d calls, want at most %d (one per query and per chunk batch)", stub.answered, wantMax)
	}
}

// With the cache the sweep needs fewer calls than the stub allows in a minute; this case makes
// twice that many distinct calls, so only the pacing keeps it under the rate.
func TestSweepEmbedder_PacesManyCalls(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	stub := &rateStub{clock: clock}
	emb := newSweepEmbedder(stub, clock, sweepCallInterval)

	for i := 0; i < 2*stubRequestsPerMin; i++ {
		if _, err := emb.Embed(context.Background(), []string{fmt.Sprintf("text %d", i)}); err != nil {
			t.Fatalf("Embed call %d: %v", i, err)
		}
	}
	if stub.refused != 0 || stub.answered != 2*stubRequestsPerMin {
		t.Fatalf("refused %d, answered %d; want 0, %d", stub.refused, stub.answered, 2*stubRequestsPerMin)
	}
}

// The stub is only a proof when an unpaced, uncached sweep trips it.
func TestSweepEmbedder_StubRefusesUnpacedSweep(t *testing.T) {
	set, chunks, ix := loadSnatchArrFixture(t)
	clock := &fakeClock{now: time.Unix(0, 0)}
	stub := &rateStub{clock: clock}

	err := runEmbedSweep(t, set, chunks, ix, stub)
	var statusErr *embed.StatusError
	if !errors.As(err, &statusErr) || statusErr.Code != http.StatusTooManyRequests {
		t.Fatalf("unpaced sweep error = %v, want a 429 from the stub", err)
	}
}

func TestSweepEmbedder_RetriesAfter429(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	stub := &rateStub{clock: clock, limit429: 2}
	emb := newSweepEmbedder(stub, clock, sweepCallInterval)

	vecs, err := emb.Embed(context.Background(), []string{"a", "b", "a"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 3 || stub.answered != 1 || stub.refused != 2 {
		t.Fatalf("vectors %d, answered %d, refused %d; want 3, 1, 2", len(vecs), stub.answered, stub.refused)
	}
	if waited := clock.now.Sub(time.Unix(0, 0)); waited < sweepFirstBackoff*3 {
		t.Fatalf("waited %v, want at least %v of backoff", waited, sweepFirstBackoff*3)
	}
}

func TestSweepEmbedder_GivesUpAfterRetries(t *testing.T) {
	clock := &fakeClock{now: time.Unix(0, 0)}
	stub := &rateStub{clock: clock, limit429: sweepMaxRetries + 1}
	emb := newSweepEmbedder(stub, clock, sweepCallInterval)

	if _, err := emb.Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("Embed succeeded, want an error after the last retry")
	}
	if stub.refused != sweepMaxRetries+1 {
		t.Fatalf("refused %d calls, want %d", stub.refused, sweepMaxRetries+1)
	}
}
