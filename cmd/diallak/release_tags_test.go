package main

import (
	"os"
	"regexp"
	"testing"
)

// The README and the hosting note say that the image has no floating "latest" tag, so that a
// deployment pins what it runs. The action that computes the image tags adds "latest" to every
// version tag unless its flavor says otherwise. This test keeps that setting in the workflow.
func TestReleaseImageWorkflow_Negative_NoLatestTag(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/release-image.yml")
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	if !regexp.MustCompile(`(?m)^\s+uses: docker/metadata-action@[0-9a-f]{40}`).Match(workflow) {
		t.Fatalf("the workflow no longer computes its tags with docker/metadata-action; rewrite this test for the step that does")
	}
	if !regexp.MustCompile(`(?m)^\s+flavor: \|\n\s+latest=false$`).Match(workflow) {
		t.Fatalf("the tag step does not set flavor latest=false, so a release would push a latest tag")
	}
}
