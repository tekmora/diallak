package discord

import (
	"bytes"
	"context"
	"regexp"
	"slices"
	"testing"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

// every call the tool may make against the platform; no call on it needs MANAGE_GUILD (ADR-0015).
var allowedCallShapes = []string{
	"GET /applications/@me",
	"GET /guilds/{id}",
	"GET /guilds/{id}/channels",
	"GET /guilds/{id}/members/{id}",
	"GET /guilds/{id}/roles",
	"PATCH /guilds/{id}/roles",
	"PATCH /guilds/{id}/roles/{id}",
	"POST /guilds/{id}/channels",
	"POST /guilds/{id}/roles",
	"PUT /channels/{id}/permissions/{id}",
}

var idPattern = regexp.MustCompile(`[0-9]+`)

func callsOutsideList(calls []recordedCall) []string {
	var outside []string
	for _, c := range calls {
		shape := c.method + " " + idPattern.ReplaceAllString(c.path, "{id}")
		if !slices.Contains(allowedCallShapes, shape) {
			outside = append(outside, shape)
		}
	}
	return outside
}

func runFirstSetUpForCallsTest(
	t *testing.T,
	ctx context.Context,
	fakeToken string,
	guildID uint64,
	def *serverdef.Definition,
	defDigest string,
	opts clientOptions,
) {
	t.Helper()
	snapBefore, err := readServerWithOptions(ctx, fakeToken, guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions before: %v", err)
	}
	resBefore, err := serverplan.Plan(def, snapBefore)
	if err != nil {
		t.Fatalf("Plan before: %v", err)
	}
	planDigest, err := serverplan.Digest(resBefore, defDigest)
	if err != nil {
		t.Fatalf("Digest before: %v", err)
	}

	var out bytes.Buffer
	err = applyServerWithOptions(ctx, fakeToken, guildID, ApplyInput{
		Definition:       def,
		DefinitionDigest: defDigest,
		PlanDigest:       planDigest,
	}, &out, opts)
	if err != nil {
		t.Fatalf("applyServerWithOptions: %v", err)
	}
}

func runDisplayChangeForCallsTest(
	t *testing.T,
	ctx context.Context,
	fakeToken string,
	guildID uint64,
	def *serverdef.Definition,
	defDigest string,
	opts clientOptions,
) {
	t.Helper()
	snapDisp, err := readServerWithOptions(ctx, fakeToken, guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions display: %v", err)
	}
	resDisp, err := serverplan.Plan(def, snapDisp)
	if err != nil {
		t.Fatalf("Plan display: %v", err)
	}
	dispDigest, err := serverplan.Digest(resDisp, defDigest)
	if err != nil {
		t.Fatalf("Digest display: %v", err)
	}
	var out bytes.Buffer
	err = applyServerWithOptions(ctx, fakeToken, guildID, ApplyInput{
		Definition:       def,
		DefinitionDigest: defDigest,
		PlanDigest:       dispDigest,
	}, &out, opts)
	if err != nil {
		t.Fatalf("applyServerWithOptions display: %v", err)
	}
}

func TestServerApplyCalls_Positive_FullRunAndDisplayChange(t *testing.T) {
	ctx := context.Background()
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	fakeToken := makeFakeToken(botUserID)
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition: %v", err)
	}
	defDigest := server.DefinitionDigest()

	fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
	fake.createShifts = true
	srv := fake.server()
	defer srv.Close()

	opts := clientOptions{restURL: srv.URL}
	runFirstSetUpForCallsTest(t, ctx, fakeToken, guildID, def, defDigest, opts)

	snapAfter, err := readServerWithOptions(ctx, fakeToken, guildID, opts)
	if err != nil {
		t.Fatalf("readServerWithOptions after: %v", err)
	}
	if _, err = serverplan.Compare(def, snapAfter); err != nil {
		t.Fatalf("Compare: %v", err)
	}

	// Trigger a set-role-display run by modifying a role display attribute.
	for i := range fake.roles {
		if fake.roles[i].name == "Helper" {
			fake.roles[i].hoist = false
			break
		}
	}
	runDisplayChangeForCallsTest(t, ctx, fakeToken, guildID, def, defDigest, opts)

	outside := callsOutsideList(fake.getRecorded())
	if len(outside) != 0 {
		t.Errorf("calls outside allow-list: %v", outside)
	}
}

func TestServerApplyCalls_Negative_SyntheticCallsOutsideList(t *testing.T) {
	cases := []struct {
		call recordedCall
		want string
	}{
		{recordedCall{method: "PATCH", path: "/guilds/1001"}, "PATCH /guilds/{id}"},
		{recordedCall{method: "GET", path: "/guilds/1001/auto-moderation/rules"}, "GET /guilds/{id}/auto-moderation/rules"},
		{recordedCall{method: "POST", path: "/guilds/1001/auto-moderation/rules"}, "POST /guilds/{id}/auto-moderation/rules"},
		{recordedCall{method: "DELETE", path: "/guilds/1001/integrations/2001"}, "DELETE /guilds/{id}/integrations/{id}"},
		{recordedCall{method: "GET", path: "/guilds/1001/invites"}, "GET /guilds/{id}/invites"},
		{recordedCall{method: "PATCH", path: "/guilds/1001/widget"}, "PATCH /guilds/{id}/widget"},
		{recordedCall{method: "PATCH", path: "/guilds/1001/welcome-screen"}, "PATCH /guilds/{id}/welcome-screen"},
		{recordedCall{method: "PUT", path: "/guilds/1001/incident-actions"}, "PUT /guilds/{id}/incident-actions"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			got := callsOutsideList([]recordedCall{tc.call})
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("callsOutsideList(%+v) = %v, want [%s]", tc.call, got, tc.want)
			}
		})
	}
}

func TestServerApplyCalls_Boundary_ListExclusionsAndDifferentMethod(t *testing.T) {
	test2Shapes := []string{
		"PATCH /guilds/{id}",
		"GET /guilds/{id}/auto-moderation/rules",
		"POST /guilds/{id}/auto-moderation/rules",
		"DELETE /guilds/{id}/integrations/{id}",
		"GET /guilds/{id}/invites",
		"PATCH /guilds/{id}/widget",
		"PATCH /guilds/{id}/welcome-screen",
		"PUT /guilds/{id}/incident-actions",
	}
	for _, shape := range test2Shapes {
		if slices.Contains(allowedCallShapes, shape) {
			t.Errorf("allowedCallShapes unexpectedly contains test 2 shape %q", shape)
		}
	}

	call := recordedCall{method: "DELETE", path: "/applications/@me"}
	got := callsOutsideList([]recordedCall{call})
	want := "DELETE /applications/@me"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("callsOutsideList for different method = %v, want [%s]", got, want)
	}
}
