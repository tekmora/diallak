package eval

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/tekmora/diallak/internal/retrieval"
)

// Limits on the memory of building the index over the fixture corpus and
// answering every evaluation question, about twice the use measured on
// 2026-10-13 (309,832 to 349,320 bytes held, 1,793,176 to 2,004,056 bytes
// allocated, with and without -race). docs/hosting.md cites them; the
// container memory limit rests on them, so raising one is a reviewed change.
const (
	maxEvalLiveHeapBytes  = 700_000
	maxEvalAllocatedBytes = 4_000_000
)

// evalMemory is the heap the evaluation keeps while the index is alive and
// the bytes it allocated on the way there.
type evalMemory struct {
	live      int64
	allocated uint64
}

func heapAfterGC() runtime.MemStats {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms
}

// measureEval loads the fixture corpus, builds the index and answers every
// question, then measures with the index and keep still alive. keep plants
// extra memory the evaluation holds; nil plants nothing.
func measureEval(t *testing.T, keep func() any) evalMemory {
	t.Helper()
	before := heapAfterGC()
	set, _, ix := loadSnatchArrFixture(t)
	if _, err := Run(context.Background(), set, ix, retrieval.DefaultOptions()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	var held any
	if keep != nil {
		held = keep()
	}
	after := heapAfterGC()
	runtime.KeepAlive(ix)
	runtime.KeepAlive(held)
	return evalMemory{
		live:      int64(after.HeapAlloc) - int64(before.HeapAlloc),
		allocated: after.TotalAlloc - before.TotalAlloc,
	}
}

func checkEvalMemory(m evalMemory) error {
	if m.live > maxEvalLiveHeapBytes {
		return fmt.Errorf("live heap %d bytes, limit %d", m.live, maxEvalLiveHeapBytes)
	}
	if m.allocated > maxEvalAllocatedBytes {
		return fmt.Errorf("allocated %d bytes, limit %d", m.allocated, maxEvalAllocatedBytes)
	}
	return nil
}

func TestEvalMemory_Positive_UnderTheLimits(t *testing.T) {
	m := measureEval(t, nil)
	t.Logf("live heap %d bytes, allocated %d bytes", m.live, m.allocated)
	if err := checkEvalMemory(m); err != nil {
		t.Fatal(err)
	}
}

func TestEvalMemory_Negative_HeldMemoryOverTheLimitFails(t *testing.T) {
	m := measureEval(t, func() any { return make([]byte, maxEvalLiveHeapBytes+1) })
	if err := checkEvalMemory(m); err == nil {
		t.Fatalf("live heap %d bytes with %d bytes planted passed the limit %d", m.live, maxEvalLiveHeapBytes+1, maxEvalLiveHeapBytes)
	}
}

func TestEvalMemory_Boundary_LimitsAreInclusive(t *testing.T) {
	at := evalMemory{live: maxEvalLiveHeapBytes, allocated: maxEvalAllocatedBytes}
	if err := checkEvalMemory(at); err != nil {
		t.Errorf("use at the limits failed: %v", err)
	}
	over := []evalMemory{
		{live: maxEvalLiveHeapBytes + 1, allocated: maxEvalAllocatedBytes},
		{live: maxEvalLiveHeapBytes, allocated: maxEvalAllocatedBytes + 1},
	}
	for _, m := range over {
		if checkEvalMemory(m) == nil {
			t.Errorf("use %+v one byte over a limit passed", m)
		}
	}
}
