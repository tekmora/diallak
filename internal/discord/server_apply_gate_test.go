package discord

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

// applyFixture is a fresh server with a raised bot role, read and planned.
type applyFixture struct {
	fake   *fakePlatform
	url    string
	def    *serverdef.Definition
	plan   serverplan.Result
	digest string
}

func (fx applyFixture) input() ApplyInput {
	return ApplyInput{Definition: fx.def, DefinitionDigest: server.DefinitionDigest(), PlanDigest: fx.digest}
}

func newApplyFixture(t *testing.T) applyFixture {
	t.Helper()
	def := mustDefinition(t)
	fake := setupFreshServerFake(t, orderGuildID, orderBotUserID, orderBotRoleID, true)
	srv := fake.server()
	t.Cleanup(srv.Close)

	opts := clientOptions{restURL: srv.URL}
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(orderBotUserID), orderGuildID, opts)
	if err != nil {
		t.Fatalf("read the server: %v", err)
	}
	plan, err := serverplan.Plan(def, snap)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	digest, err := serverplan.Digest(plan, server.DefinitionDigest())
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	fake.resetRecorded()
	return applyFixture{fake: fake, url: srv.URL, def: def, plan: plan, digest: digest}
}

func TestApplyServer_Negative_InputRefusedBeforeAnyRequest(t *testing.T) {
	const required = "definition, definition digest and plan digest are required"
	cases := []struct {
		name    string
		token   string
		guildID uint64
		change  func(in *ApplyInput)
		want    string
	}{
		{"no token", "", orderGuildID, nil, "bot token is required"},
		{"no server id", makeFakeToken(orderBotUserID), 0, nil, "guild ID must not be zero"},
		{"no definition digest", makeFakeToken(orderBotUserID), orderGuildID,
			func(in *ApplyInput) { in.DefinitionDigest = "" }, required},
		{"no plan digest", makeFakeToken(orderBotUserID), orderGuildID,
			func(in *ApplyInput) { in.PlanDigest = "" }, required},
		{"no definition", makeFakeToken(orderBotUserID), orderGuildID,
			func(in *ApplyInput) { in.Definition = nil }, required},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newApplyFixture(t)
			in := fx.input()
			if tc.change != nil {
				tc.change(&in)
			}
			var out bytes.Buffer
			err := applyServerWithOptions(context.Background(), tc.token, tc.guildID, in, &out, clientOptions{restURL: fx.url})
			if err == nil || err.Error() != tc.want {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want none", out.String())
			}
			assertRequestCounts(t, fx.fake.getRecorded(), 0, 0)
		})
	}
}

// gateInput returns the input whose plan digest fits the given result.
func gateInput(t *testing.T, def *serverdef.Definition, result serverplan.Result) ApplyInput {
	t.Helper()
	digest, err := serverplan.Digest(result, server.DefinitionDigest())
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	return ApplyInput{Definition: def, DefinitionDigest: server.DefinitionDigest(), PlanDigest: digest}
}

func TestVerifyApplyGates_Boundary_OneRefusalIsEnough(t *testing.T) {
	def := mustDefinition(t)
	result := serverplan.Result{
		Changes: []serverplan.Change{{Step: 1, Action: "set-role-order", Object: "role_order"}},
		Refused: []serverplan.Refusal{{Action: "convert-channel", Object: "channels[x]", Reason: "type-differs"}},
	}
	const want = "the plan holds 1 refused changes; a plan with a refusal is not applied"
	if err := verifyApplyGates(result, gateInput(t, def, result)); err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}

	result.Refused = nil
	if err := verifyApplyGates(result, gateInput(t, def, result)); err != nil {
		t.Errorf("the same plan without the refusal: %v", err)
	}
}

func TestVerifyApplyGates_Negative_ChangeOutsideTheDefinition(t *testing.T) {
	def := mustDefinition(t)
	channel := def.Channels[0].ID
	cases := []struct {
		action string
		object string
		want   string
	}{
		{"delete-role", "roles[" + def.Roles[0].ID + "]", "unsupported action"},
		{"create-role", "roles[no-such]", `role "no-such" not found in definition`},
		{"set-role-permissions", "roles[no-such]", `role "no-such" not found in definition`},
		{"set-role-permissions", "roles[]", `empty identifier in "roles[]"`},
		{"create-role", "channels[" + channel + "]", `invalid object "channels[` + channel + `]"`},
		{"set-role-order", "roles[" + def.Roles[0].ID + "]", "unexpected object for set-role-order"},
		{"create-category", "categories[no-such]", `category "no-such" not found in definition`},
		{"create-channel", "channels[no-such]", `channel "no-such" not found in definition`},
		{"move-channel", "channels[no-such]", `channel "no-such" not found in definition`},
		{"set-access", "channels[no-such].access[everyone]", `channel "no-such" not found in definition`},
		{"set-access", "channels[" + channel + "].access[no-such]", `access for role "no-such" not found in definition`},
		{"set-access", "channels[" + channel + "].access[]", "empty channel or role"},
		{"set-access", "channels[].access[everyone]", "empty channel or role"},
		{"set-access", "channels[" + channel + "]", "invalid access object"},
	}
	for _, tc := range cases {
		t.Run(tc.action+" "+tc.object, func(t *testing.T) {
			// A change of the definition first: the check has to reach the second one.
			result := serverplan.Result{Changes: []serverplan.Change{
				{Step: 1, Action: "set-role-order", Object: "role_order"},
				{Step: 2, Action: tc.action, Object: tc.object},
			}}
			err := verifyApplyGates(result, gateInput(t, def, result))
			if err == nil {
				t.Fatal("the change passed the check")
			}
			prefix := fmt.Sprintf("step 2 %s %s: ", tc.action, tc.object)
			if !strings.HasPrefix(err.Error(), prefix) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want prefix %q and %q", err.Error(), prefix, tc.want)
			}
		})
	}
}

// cancelOnDone ends the run's context when the first change is reported done.
type cancelOnDone struct {
	io.Writer
	cancel context.CancelFunc
}

func (w cancelOnDone) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte(": done\n")) {
		w.cancel()
	}
	return w.Writer.Write(p)
}

func TestApplyServer_Boundary_CancelledBetweenTwoChanges(t *testing.T) {
	fx := newApplyFixture(t)
	if len(fx.plan.Changes) < 2 {
		t.Fatalf("the fixture's plan holds %d changes, want at least 2", len(fx.plan.Changes))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var out bytes.Buffer
	err := applyServerWithOptions(ctx, makeFakeToken(orderBotUserID), orderGuildID, fx.input(),
		cancelOnDone{Writer: &out, cancel: cancel}, clientOptions{restURL: fx.url})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	first, n := fx.plan.Changes[0], len(fx.plan.Changes)
	want := fmt.Sprintf("changes: %d\n  %d. %s %s: done\napplied: 1 of %d\n", n, first.Step, first.Action, first.Object, n)
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
	assertRequestCounts(t, fx.fake.getRecorded(), 5, 1)
}
