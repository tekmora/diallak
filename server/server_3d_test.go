package server

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestDefinitionDigest_Boundary_FullThirtyTwoByteChecksum(t *testing.T) {
	digest := DefinitionDigest()
	sum := sha256.Sum256([]byte(definitionJSON))
	want := fmt.Sprintf("sha256:%x", sum)
	if digest != want {
		t.Fatalf("DefinitionDigest() = %q, want %q", digest, want)
	}
	const wantLen = len("sha256:") + 64
	if len(digest) != wantLen {
		t.Fatalf("DefinitionDigest() length = %d, want %d", len(digest), wantLen)
	}
}
