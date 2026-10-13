package answer

import (
	"testing"
)

func TestStats_Boundary_InitialAskedCountZero(t *testing.T) {
	p, err := New(Config{
		Retriever: &stubRetriever{},
	})
	if err != nil {
		t.Fatalf("New pipeline: %v", err)
	}
	got := p.Stats()
	if got.Asked != 0 {
		t.Fatalf("Stats().Asked = %d, want 0", got.Asked)
	}
}
