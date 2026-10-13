package discord

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
)

const (
	orderGuildID   uint64 = 1001
	orderBotUserID uint64 = 2001
	orderBotRoleID uint64 = 3001
	orderDigest           = "sha256:abc"
)

type positionEntry struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
}

// orderRoles returns the server's roles for a role order case: the role every
// member holds, the bot's role at botPos and the given other roles.
func orderRoles(botPos int, others ...fakeRole) []fakeRole {
	bID := snowflake.ID(orderBotUserID)
	roles := []fakeRole{
		{id: snowflake.ID(orderGuildID), name: "@everyone", position: 0},
		{id: snowflake.ID(orderBotRoleID), name: "Diallak", position: botPos, botID: &bID, managed: true},
	}
	return append(roles, others...)
}

// runOrderChange runs one set-role-order change against a fake that holds
// serverRoles. The index is built from indexRoles, the roles the run's first
// read saw.
func runOrderChange(
	t *testing.T,
	fake *fakePlatform,
	indexRoles []fakeRole,
	detail string,
	timeout time.Duration,
) error {
	t.Helper()
	idx := newServerIndex(orderGuildID, snowflake.ID(orderBotRoleID))
	for _, r := range indexRoles {
		idx.addRole(r.name, r.id)
	}
	c := serverplan.Change{Step: 1, Action: "set-role-order", Object: "role_order", Detail: detail}
	return runChange(t, fake, mustDefinition(t), idx, c, timeout)
}

func positionPatches(t *testing.T, recorded []recordedCall) [][]positionEntry {
	t.Helper()
	var res [][]positionEntry
	for _, call := range recorded {
		if call.method == http.MethodGet {
			continue
		}
		if call.method != http.MethodPatch || !strings.HasSuffix(call.path, "/roles") {
			t.Errorf("unexpected write %s %s", call.method, call.path)
			continue
		}
		var entries []positionEntry
		if err := json.Unmarshal(call.body, &entries); err != nil {
			t.Fatalf("decode the position update: %v", err)
		}
		res = append(res, entries)
	}
	return res
}

// assertPositionPatches verifies the patches and audit reasons for a role order test.
func assertPositionPatches(t *testing.T, recorded []recordedCall, want []positionEntry, wantReason string) {
	t.Helper()
	assertRequestCounts(t, recorded, 1, 1)
	patches := positionPatches(t, recorded)
	if len(patches) != 1 {
		t.Fatalf("position updates = %d, want 1", len(patches))
	}
	if len(patches[0]) != len(want) {
		t.Fatalf("position update = %+v, want %+v", patches[0], want)
	}
	for i := range want {
		if patches[0][i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, patches[0][i], want[i])
		}
	}
	for _, call := range recorded {
		expected := wantReason
		if call.method == http.MethodGet {
			expected = ""
		}
		if call.auditReason != expected {
			t.Errorf("%s %s audit reason = %q, want %q", call.method, call.path, call.auditReason, expected)
		}
	}
}

// runRoleOrderSubtest executes one role order scenario and verifies its output.
func runRoleOrderSubtest(t *testing.T, roles []fakeRole, want []positionEntry, wantReason string) {
	t.Helper()
	fake := newFakePlatform(t, orderGuildID, orderBotUserID, orderBotRoleID)
	fake.roles = roles
	err := runOrderChange(t, fake, roles, "operator, contributor, supporter", 10*time.Second)
	if err != nil {
		t.Fatalf("set-role-order error = %v, want nil", err)
	}
	assertPositionPatches(t, fake.getRecorded(), want, wantReason)
}

func TestApplySetRoleOrder_Positive_OnePatchWithEveryRoleBelow(t *testing.T) {
	operator := fakeRole{id: 4001, name: "Operator"}
	contributor := fakeRole{id: 6001, name: "Contributor"}
	supporter := fakeRole{id: 6002, name: "Supporter"}
	other := fakeRole{id: 7001, name: "Another bot", managed: true}
	at := func(r fakeRole, pos int) fakeRole {
		r.position = pos
		return r
	}
	tests := []struct {
		name  string
		roles []fakeRole
		want  []positionEntry
	}{
		{
			name:  "two definition roles swapped",
			roles: orderRoles(3, at(operator, 4), at(supporter, 2), at(contributor, 1)),
			want:  []positionEntry{{"6001", 2}, {"6002", 1}},
		},
		{
			name:  "a role the definition does not hold keeps its place",
			roles: orderRoles(4, at(operator, 5), at(supporter, 3), at(other, 2), at(contributor, 1)),
			want:  []positionEntry{{"6001", 3}, {"7001", 2}, {"6002", 1}},
		},
		{
			name:  "a gap and a tie are normalised",
			roles: orderRoles(8, at(operator, 9), at(contributor, 6), at(other, 2), at(supporter, 2)),
			want:  []positionEntry{{"6001", 3}, {"6002", 2}, {"7001", 1}},
		},
	}
	wantReason := orderAuditReason()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runRoleOrderSubtest(t, tc.roles, tc.want, wantReason)
		})
	}
}

func TestApplySetRoleOrder_Boundary_OrderHeldSendsNoWrite(t *testing.T) {
	roles := orderRoles(3,
		fakeRole{id: 4001, name: "Operator", position: 4},
		fakeRole{id: 6001, name: "Contributor", position: 2},
		fakeRole{id: 6002, name: "Supporter", position: 1},
	)
	fake := newFakePlatform(t, orderGuildID, orderBotUserID, orderBotRoleID)
	fake.roles = roles
	if err := runOrderChange(t, fake, roles, "operator, contributor, supporter", 10*time.Second); err != nil {
		t.Fatalf("set-role-order error = %v, want nil", err)
	}
	assertRequestCounts(t, fake.getRecorded(), 1, 0)
}

func TestApplySetRoleOrder_Negative_ServerChangedSinceThePlan(t *testing.T) {
	operator := fakeRole{id: 4001, name: "Operator", position: 5}
	moderator := fakeRole{id: 6003, name: "Moderator", position: 6}
	contributor := fakeRole{id: 6001, name: "Contributor", position: 2}
	supporter := fakeRole{id: 6002, name: "Supporter", position: 1}
	seen := orderRoles(4, operator, contributor, supporter)
	tests := []struct {
		name   string
		roles  []fakeRole
		index  []fakeRole
		detail string
		want   string
	}{
		{
			name:   "a target role left the server",
			roles:  orderRoles(4, operator, supporter),
			index:  seen,
			detail: "operator, supporter, contributor",
			want:   "the roles changed during the run",
		},
		{
			name:   "the bot's role left the server",
			roles:  []fakeRole{{id: snowflake.ID(orderGuildID), name: "@everyone"}, operator, supporter, contributor},
			index:  seen,
			detail: "operator, supporter, contributor",
			want:   "the roles changed during the run",
		},
		{
			name:   "the roles above the bot's role stand in another order",
			roles:  orderRoles(4, moderator, operator, supporter, contributor),
			index:  orderRoles(4, moderator, operator, supporter, contributor),
			detail: "operator, moderator, supporter, contributor",
			want:   "a role above the bot's role is not where the plan saw it",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakePlatform(t, orderGuildID, orderBotUserID, orderBotRoleID)
			fake.roles = tc.roles
			err := runOrderChange(t, fake, tc.index, tc.detail, 10*time.Second)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("set-role-order error = %v, want it to hold %q", err, tc.want)
			}
			assertRequestCounts(t, fake.getRecorded(), 1, 0)
		})
	}
}

// The step makes two requests. Each has the whole deadline: with one deadline
// for both, the second request of this case would start after 400 ms of 700 ms
// and fail.
func TestApplySetRoleOrder_Boundary_EachRequestHasItsOwnDeadline(t *testing.T) {
	roles := orderRoles(3,
		fakeRole{id: 4001, name: "Operator", position: 4},
		fakeRole{id: 6002, name: "Supporter", position: 2},
		fakeRole{id: 6001, name: "Contributor", position: 1},
	)
	fake := newFakePlatform(t, orderGuildID, orderBotUserID, orderBotRoleID)
	fake.roles = roles
	fake.delaySuffix = "/roles"
	fake.delay = 400 * time.Millisecond
	err := runOrderChange(t, fake, roles, "operator, contributor, supporter", 700*time.Millisecond)
	if err != nil {
		t.Fatalf("set-role-order error = %v, want nil", err)
	}
	assertRequestCounts(t, fake.getRecorded(), 1, 1)
}

func TestApplySetRoleOrder_Negative_ReadPastItsDeadline(t *testing.T) {
	roles := orderRoles(3,
		fakeRole{id: 4001, name: "Operator", position: 4},
		fakeRole{id: 6002, name: "Supporter", position: 2},
		fakeRole{id: 6001, name: "Contributor", position: 1},
	)
	fake := newFakePlatform(t, orderGuildID, orderBotUserID, orderBotRoleID)
	fake.roles = roles
	fake.delaySuffix = "/roles"
	fake.delay = 5 * time.Second
	start := time.Now()
	err := runOrderChange(t, fake, roles, "operator, contributor, supporter", 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "read roles") {
		t.Fatalf("set-role-order error = %v, want it to name the call step read roles", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("the read took %v, want it to end at its deadline", took)
	}
	assertRequestCounts(t, fake.getRecorded(), 1, 0)
}

// swappedOrderFake holds two definition roles below the bot's role in the
// wrong order, so that the change has to write.
func swappedOrderFake(t *testing.T) (*fakePlatform, []fakeRole) {
	t.Helper()
	roles := orderRoles(3,
		fakeRole{id: 4001, name: "Operator", position: 4},
		fakeRole{id: 6002, name: "Supporter", position: 2},
		fakeRole{id: 6001, name: "Contributor", position: 1},
	)
	fake := newFakePlatform(t, orderGuildID, orderBotUserID, orderBotRoleID)
	fake.roles = roles
	return fake, roles
}

func TestApplySetRoleOrder_Negative_WritePastItsDeadline(t *testing.T) {
	fake, roles := swappedOrderFake(t)
	fake.hangWrite = 1
	// The fake lets go after three seconds, so a deadline that is far too long
	// fails this test instead of the whole run.
	release := time.AfterFunc(3*time.Second, func() { close(fake.hangRelease) })
	defer release.Stop()

	start := time.Now()
	err := runOrderChange(t, fake, roles, "operator, contributor, supporter", 50*time.Millisecond)
	if err == nil || !strings.HasPrefix(err.Error(), "set role order: ") {
		t.Errorf("set-role-order error = %v, want one of the call step set role order", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("the write took %v, want it to end at its deadline", took)
	}
	assertRequestCounts(t, fake.getRecorded(), 1, 1)
}

func TestApplySetRoleOrder_Negative_RefusedWriteNamesTheCallStep(t *testing.T) {
	fake, roles := swappedOrderFake(t)
	fake.failWrite = 1
	fake.failStatus = http.StatusForbidden
	fake.failBody = missingPermissionsBody
	err := runOrderChange(t, fake, roles, "operator, contributor, supporter", 10*time.Second)
	assertRefusedCall(t, err, "set role order")
	assertRequestCounts(t, fake.getRecorded(), 1, 1)
}

func TestResolveTargetRoles_Negative_TargetOutsideTheDefinition(t *testing.T) {
	idx := newServerIndex(orderGuildID, snowflake.ID(orderBotRoleID))
	idx.addRole("Operator", 4001)
	idx.addRole("Supporter", 6002)
	got, err := resolveTargetRoles(mustDefinition(t), "operator, no-such, supporter", idx)
	const want = `target role "no-such" not found in definition`
	if err == nil || err.Error() != want {
		t.Errorf("resolveTargetRoles = %v, %v, want the error %q", got, err, want)
	}
}

func TestResolveTargetRoles_Boundary_ResolvesByKindNotByID(t *testing.T) {
	def := &serverdef.Definition{Roles: []serverdef.Role{
		{ID: "bot", Name: "Botany", Kind: "interest"},
		{ID: "everyone", Name: "Everyone welcome", Kind: "interest"},
		{ID: "helper", Name: "Diallak", Kind: "bot"},
	}}
	idx := newServerIndex(orderGuildID, snowflake.ID(orderBotRoleID))
	idx.addRole("Botany", 8001)
	idx.addRole("Everyone welcome", 8002)
	got, err := resolveTargetRoles(def, "bot, everyone, helper", idx)
	if err != nil {
		t.Fatalf("resolveTargetRoles error = %v, want nil", err)
	}
	want := []snowflake.ID{8001, 8002, snowflake.ID(orderBotRoleID)}
	if len(got) != len(want) {
		t.Fatalf("resolveTargetRoles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("target %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestApplyMoveChannel_Negative_NoCategoryInTheDefinition(t *testing.T) {
	def := mustDefinition(t)
	ch := &def.Channels[0]
	ch.Category = ""
	fake := newFakePlatform(t, orderGuildID, orderBotUserID, orderBotRoleID)

	idx := newServerIndex(orderGuildID, snowflake.ID(orderBotRoleID))
	idx.addChannel(ch.Name, 9001)
	c := serverplan.Change{Step: 1, Action: "move-channel", Object: "channels[" + ch.ID + "]"}
	err := runChange(t, fake, def, idx, c, 10*time.Second)
	if err == nil || !strings.Contains(err.Error(), "has no category in the definition") {
		t.Fatalf("move-channel error = %v, want it to say the channel has no category", err)
	}
	assertRequestCounts(t, fake.getRecorded(), 0, 0)
}
