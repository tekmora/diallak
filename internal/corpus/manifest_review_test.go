package corpus

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadAll_Boundary_SourceCountLimit(t *testing.T) {
	// A manifest built in code skips LoadManifest, so LoadAll holds the limit itself.
	m := Manifest{Version: 1}
	for i := 0; i <= MaxSources; i++ {
		m.Sources = append(m.Sources, ManifestSource{
			Directory: fmt.Sprintf("s%d", i), Repository: fmt.Sprintf("owner/r%d", i),
			Revision: "v1", Patterns: []string{"*.md"},
		})
	}
	_, err := LoadAll(context.Background(), fstest.MapFS{}, m, DefaultLimits())
	if err == nil || !strings.Contains(err.Error(), "exceeding limit") {
		t.Fatalf("LoadAll with %d sources err = %v, want the source limit error", len(m.Sources), err)
	}
}
