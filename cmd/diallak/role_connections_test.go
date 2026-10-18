package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	disgodiscord "github.com/disgoorg/disgo/discord"
	"github.com/tekmora/diallak/server"
)

func TestRun_Positive_RoleConnectionsPlanWithEmptyGetenv(t *testing.T) {
	args := []string{"-role-connections=plan"}
	emptyGetenv := func(string) string { return "" }
	var out bytes.Buffer

	err := run(context.Background(), args, emptyGetenv, &out, defaultDependencies())
	if err != nil {
		t.Fatalf("run(-role-connections=plan) error = %v, want nil", err)
	}

	got := out.String()
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 4 {
		t.Fatalf("output lines = %d, want at least 4: %q", len(lines), got)
	}
	if lines[0] != "PUT /applications/{application.id}/role-connections/metadata" {
		t.Errorf("line 0 = %q, want 'PUT /applications/{application.id}/role-connections/metadata'", lines[0])
	}
	if !strings.Contains(got, "merged_contribution") {
		t.Error("output missing 'merged_contribution'")
	}

	digest := server.RoleConnectionsDigest()
	wantDigestLine := "digest " + digest
	if !strings.Contains(got, wantDigestLine) {
		t.Errorf("output missing %q", wantDigestLine)
	}
	wantFooter := "dry run: nothing sent; push with -role-connections=push=" + digest
	if !strings.Contains(got, wantFooter) {
		t.Errorf("output missing %q", wantFooter)
	}
}

func TestRun_Negative_RoleConnectionsInvalidPushDigest(t *testing.T) {
	args := []string{"-role-connections=push=abc"}
	emptyGetenv := func(string) string { return "" }
	var out bytes.Buffer

	err := run(context.Background(), args, emptyGetenv, &out, defaultDependencies())
	if err == nil {
		t.Fatal("expected error for invalid push digest, got nil")
	}
	if !strings.Contains(err.Error(), "invalid -role-connections mode") {
		t.Errorf("error = %q, want to contain 'invalid -role-connections mode'", err.Error())
	}
}

func TestRun_Negative_RoleConnectionsCombinedWithServer(t *testing.T) {
	args := []string{"-role-connections=plan", "-server=plan"}
	emptyGetenv := func(string) string { return "" }
	var out bytes.Buffer

	err := run(context.Background(), args, emptyGetenv, &out, defaultDependencies())
	if err == nil {
		t.Fatal("expected error for combined flags, got nil")
	}
	if !strings.Contains(err.Error(), "-role-connections") {
		t.Errorf("error = %q, want to contain '-role-connections'", err.Error())
	}
}

func TestRun_Negative_RoleConnectionsPushWrongDigestRefusedWithoutCallingDep(t *testing.T) {
	calls := 0
	deps := dependencies{
		pushRoleConnections: func(_ context.Context, _ string, _ []disgodiscord.ApplicationRoleConnectionMetadata, _ io.Writer) error {
			calls++
			return nil
		},
	}
	wrongDigest := "sha256:" + strings.Repeat("0", 64)
	args := []string{"-role-connections=push=" + wrongDigest}
	var out bytes.Buffer

	err := run(context.Background(), args, validEnv(t), &out, deps)
	if err == nil {
		t.Fatal("expected error for wrong digest, got nil")
	}
	if !strings.Contains(err.Error(), "records changed since plan") {
		t.Errorf("error = %q, want 'records changed since plan'", err.Error())
	}
	if calls != 0 {
		t.Errorf("pushRoleConnections called %d times, want 0", calls)
	}
}

func TestRun_Positive_RoleConnectionsPushRightDigestCallsFakeDepOnce(t *testing.T) {
	calls := 0
	deps := dependencies{
		pushRoleConnections: func(_ context.Context, _ string, _ []disgodiscord.ApplicationRoleConnectionMetadata, _ io.Writer) error {
			calls++
			return nil
		},
	}
	rightDigest := server.RoleConnectionsDigest()
	args := []string{"-role-connections=push=" + rightDigest}
	var out bytes.Buffer

	err := run(context.Background(), args, validEnv(t), &out, deps)
	if err != nil {
		t.Fatalf("run(-role-connections=push=<rightDigest>) error = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("pushRoleConnections called %d times, want 1", calls)
	}
}
