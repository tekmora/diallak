package main

import (
	"os"
	"regexp"
	"testing"
)

// The README and the hosting note send a deployment to the release page for the digest to pin.
// The release workflow writes it there: it appends the image block to the notes, with the digest
// the image job reports, and rewrites the notes of a release that already exists, because a
// rebuild has a new digest. This test keeps those three parts in the publish step.
func TestReleaseWorkflow_Negative_NotesNameTheImageDigest(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	parts := []struct{ missing, pattern string }{
		{
			"the digest of the image job is not passed to the publish step",
			`(?m)^\s+DIGEST: \$\{\{ needs\.image\.outputs\.digest \}\}$`,
		},
		{
			"the image block is not appended to the notes",
			`(?m)^\s+sh \.github/scripts/release-image-note\.sh \\\n\s+"\$IMAGE" "\$\{TAG#v\}" "\$DIGEST" >> notes\.md$`,
		},
		{
			"the notes of an existing release are not rewritten",
			`(?m)^\s+gh release edit "\$TAG" --notes-file notes\.md$`,
		},
	}
	for _, part := range parts {
		if !regexp.MustCompile(part.pattern).Match(workflow) {
			t.Errorf("%s, so the release page would not name the digest", part.missing)
		}
	}
}
