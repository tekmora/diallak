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

// A release candidate vX.Y.Z-rc.N is published for a test only: its release is a prerelease that
// never becomes the latest release, its notes come from the fragments the changelog does not hold
// yet, and a final version keeps its changelog section as the source of its notes. This test keeps
// those parts in the release workflow.
func TestReleaseWorkflow_Negative_CandidateIsPrereleaseNotLatest(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	parts := []struct{ missing, pattern string }{
		{
			"a candidate tag does not start the workflow",
			`(?m)^\s+- 'v\[0-9\]\+\.\[0-9\]\+\.\[0-9\]\+-rc\.\[0-9\]\+'$`,
		},
		{
			"the tag check does not accept a candidate and nothing else",
			`(?m)^\s+re='\^v\[0-9\]\+\\\.\[0-9\]\+\\\.\[0-9\]\+\(-rc\\\.\[0-9\]\+\)\?\$'\n\s+if ! printf '%s' "\$TAG" \| grep -Eq "\$re"; then$`,
		},
		{
			"a candidate's notes do not come from the fragments",
			`(?m)^\s+sh \.github/scripts/release-candidate-notes\.sh "\$v" changelog\.d \\\n\s+> notes\.md$`,
		},
		{
			"a candidate is not created as a prerelease that is not the latest",
			`(?m)^\s+flags='--prerelease --latest=false'$`,
		},
		{
			"the release is not created with the flags",
			`(?m)^\s+gh release create "\$TAG" --verify-tag --title "\$TAG" \$flags \\$`,
		},
	}
	for _, part := range parts {
		if !regexp.MustCompile(part.pattern).Match(workflow) {
			t.Errorf("%s", part.missing)
		}
	}
}
