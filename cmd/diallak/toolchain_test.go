package main

import (
	"os"
	"regexp"
	"testing"
)

// The release image is built by the Go toolchain inside the builder image the Dockerfile names.
// The official image sets GOTOOLCHAIN=local, so the toolchain line of go.mod does not make it
// fetch a newer one: an older builder would build the release with a toolchain the tests never
// ran on, and say nothing. This test keeps the two in step by their text. The digest behind the
// tag is checked by the build itself (the "builder runs Go" step of the Dockerfile).
func TestDockerfile_Negative_BuilderTagIsTheToolchainOfGoMod(t *testing.T) {
	goMod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	dockerfile, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}

	toolchain := regexp.MustCompile(`(?m)^toolchain go(\d+\.\d+\.\d+)$`).FindSubmatch(goMod)
	if toolchain == nil {
		t.Fatalf("go.mod names no toolchain with a patch version")
	}
	builder := regexp.MustCompile(`(?m)^FROM --platform=\$BUILDPLATFORM golang:(\d+\.\d+\.\d+)-[a-z0-9]+@sha256:[0-9a-f]{64} AS builder$`).FindSubmatch(dockerfile)
	if builder == nil {
		t.Fatalf("the Dockerfile's builder is not golang:<major.minor.patch>-<variant>@sha256:<digest> on the build platform")
	}
	if string(builder[1]) != string(toolchain[1]) {
		t.Fatalf("builder image is Go %s, go.mod names toolchain %s", builder[1], toolchain[1])
	}
}
