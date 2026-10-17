package serverplan

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestActions_Positive_ContainsResolveNameAction(t *testing.T) {
	acts := Actions()
	want := []string{
		ActionCreateRole,
		ActionSetRolePermissions,
		ActionSetRoleDisplay,
		ActionSetRoleOrder,
		ActionCreateCategory,
		ActionCreateChannel,
		ActionMoveChannel,
		ActionSetAccess,
		ActionConvertChannel,
		ActionResolveName,
		ActionSetVerificationLevel,
		ActionCreateFilterRule,
		ActionSetFilterRule,
		ActionConvertFilterRule,
	}
	if !slices.Equal(acts, want) {
		t.Fatalf("Actions() = %v, want %v", acts, want)
	}
}

func TestActions_Negative_LastActionIsNotDuplicateCreateRole(t *testing.T) {
	acts := Actions()
	if len(acts) != 14 {
		t.Fatalf("Actions() length = %d, want 14", len(acts))
	}
	if acts[13] == ActionCreateRole {
		t.Fatal("Actions()[13] is ActionCreateRole, want ActionConvertFilterRule")
	}
	if acts[13] != ActionConvertFilterRule {
		t.Fatalf("Actions()[13] = %q, want %q", acts[13], ActionConvertFilterRule)
	}
}

type failingWriter struct{}

// Write implements io.Writer and returns an error.
func (f failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failure")
}

func TestRenderDifferences_Negative_PropagatesHeaderWriteError(t *testing.T) {
	err := RenderDifferences(failingWriter{}, false, nil)
	if err == nil {
		t.Fatal("RenderDifferences with failingWriter returned nil, want error")
	}
}

func TestRenderDifferences_Boundary_OmitsSemicolonWhenDetailEmpty(t *testing.T) {
	var buf bytes.Buffer
	diffs := []Difference{
		{Kind: KindChannelMissing, Object: "channels[general]", Detail: ""},
	}
	if err := RenderDifferences(&buf, false, diffs); err != nil {
		t.Fatalf("RenderDifferences: %v", err)
	}
	want := "mode: own\ndifferences: 1\n  channel-missing channels[general]\n"
	if got := buf.String(); got != want {
		t.Fatalf("RenderDifferences() = %q, want %q", got, want)
	}
}

func TestDigest_Boundary_IncludesDefinitionContentInChecksum(t *testing.T) {
	r := Result{}
	got, err := Digest(r, "sample-definition")
	if err != nil {
		t.Fatalf("Digest(): %v", err)
	}
	h := sha256.New()
	if err := Render(h, r); err != nil {
		t.Fatalf("Render(): %v", err)
	}
	if _, err := fmt.Fprintf(h, "definition: %s\n", "sample-definition"); err != nil {
		t.Fatalf("Fprintf(): %v", err)
	}
	want := fmt.Sprintf("sha256:%x", h.Sum(nil))
	if got != want {
		t.Fatalf("Digest() = %q, want %q", got, want)
	}
}

func TestLoadSnapshot_Boundary_EnforcesStructuralInvariants(t *testing.T) {
	s := validBaseSnapshot()
	s.Caller.Position = 0
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal snapshot: %v", err)
	}
	if _, err := LoadSnapshot(bytes.NewReader(data)); err == nil {
		t.Fatal("LoadSnapshot accepted caller position 0, want error from Check")
	}
}
