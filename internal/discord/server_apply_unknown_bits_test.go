package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/disgoorg/snowflake/v2"
	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"
)

// everyoneWithBit47 is VIEW_CHANNEL plus bit 47, which the permission table does not name.
const everyoneWithBit47 = "140737488356352"

// applyOnFake reads the fake server, plans against the definition and applies
// that plan. It returns the plan, the write requests the fake recorded during
// the apply and the error of the apply.
func applyOnFake(t *testing.T, fake *fakePlatform, guildID, botUserID uint64) (serverplan.Result, []recordedCall, error) {
	t.Helper()
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	defDigest := server.DefinitionDigest()
	fakeToken := makeFakeToken(botUserID)
	srv := fake.server()
	defer srv.Close()

	snap, err := readServerWithOptions(context.Background(), fakeToken, guildID, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatalf("read server: %v", err)
	}
	res, err := serverplan.Plan(def, snap)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	digest, err := serverplan.Digest(res, defDigest)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}

	fake.resetRecorded()
	var out bytes.Buffer
	applyErr := applyServerWithOptions(context.Background(), fakeToken, guildID, ApplyInput{
		Definition:       def,
		DefinitionDigest: defDigest,
		PlanDigest:       digest,
	}, &out, clientOptions{restURL: srv.URL})

	var writes []recordedCall
	for _, call := range fake.getRecorded() {
		if call.method != http.MethodGet {
			writes = append(writes, call)
		}
	}
	return res, writes, applyErr
}

// runControlBitClearedSubtest runs the control case without bit 47.
func runControlBitClearedSubtest(t *testing.T, guildID, botUserID, botRoleID uint64) {
	t.Helper()
	fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)

	res, writes, err := applyOnFake(t, fake, guildID, botUserID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(res.Refused) != 0 {
		t.Fatalf("plan holds %d refusals, want 0: %+v", len(res.Refused), res.Refused)
	}
	if len(writes) == 0 {
		t.Fatal("apply recorded no write, want the changes of the plan")
	}
}

// resolveEveryonePermissionMask returns the expected string permissions for the everyone role.
func resolveEveryonePermissionMask(t *testing.T) string {
	t.Helper()
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	var everyoneRole *serverdef.Role
	for i := range def.Roles {
		if def.Roles[i].ID == "everyone" {
			everyoneRole = &def.Roles[i]
			break
		}
	}
	if everyoneRole == nil {
		t.Fatal("definition has no everyone role")
	}
	mask, err := serverdef.PermissionMask(everyoneRole.Permissions)
	if err != nil {
		t.Fatalf("PermissionMask: %v", err)
	}
	return strconv.FormatUint(mask, 10)
}

// assertEveryoneRolePatched verifies changes and recorded patch for the everyone role.
func assertEveryoneRolePatched(t *testing.T, res serverplan.Result, writes []recordedCall, guildID uint64, wantPerms string) {
	t.Helper()
	var everyoneChanges []serverplan.Change
	for _, c := range res.Changes {
		if c.Action == serverplan.ActionSetRolePermissions && c.Object == "roles[everyone]" {
			everyoneChanges = append(everyoneChanges, c)
		}
	}
	if len(everyoneChanges) != 1 {
		t.Fatalf("plan holds %d changes for (set-role-permissions, roles[everyone]), want 1: %+v", len(everyoneChanges), res.Changes)
	}
	if everyoneChanges[0].Detail != "clears unlisted bits 47" {
		t.Errorf("change detail = %q, want %q", everyoneChanges[0].Detail, "clears unlisted bits 47")
	}

	var everyonePatches []recordedCall
	everyonePath := fmt.Sprintf("/guilds/%d/roles/%d", guildID, guildID)
	for _, w := range writes {
		if w.method == http.MethodPatch && w.path == everyonePath {
			everyonePatches = append(everyonePatches, w)
		}
	}
	if len(everyonePatches) != 1 {
		t.Fatalf("recorded writes hold %d PATCH calls on the everyone role, want exactly 1: %+v", len(everyonePatches), writes)
	}

	var reqBody struct {
		Permissions string `json:"permissions"`
	}
	if err := json.Unmarshal(everyonePatches[0].body, &reqBody); err != nil {
		t.Fatalf("unmarshal PATCH body: %v", err)
	}
	if reqBody.Permissions != wantPerms {
		t.Errorf("PATCH body permissions = %q, want %q", reqBody.Permissions, wantPerms)
	}
}

// runEveryoneBit47ClearedSubtest runs the scenario where bit 47 is present on the everyone role.
func runEveryoneBit47ClearedSubtest(t *testing.T, guildID, botUserID, botRoleID uint64) {
	t.Helper()
	fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
	fake.roles[0].permissions = everyoneWithBit47
	wantPerms := resolveEveryonePermissionMask(t)

	res, writes, err := applyOnFake(t, fake, guildID, botUserID)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(res.Refused) != 0 {
		t.Fatalf("plan holds %d refusals, want 0: %+v", len(res.Refused), res.Refused)
	}

	assertEveryoneRolePatched(t, res, writes, guildID, wantPerms)

	if got := fake.roles[0].permissions; got != wantPerms {
		t.Errorf("everyone role permissions = %q after the run, want %q", got, wantPerms)
	}
}

func TestApplyServer_Positive_UnlistedBitOnAChangedRoleIsCleared(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)

	t.Run("control: without the bit the run writes", func(t *testing.T) {
		runControlBitClearedSubtest(t, guildID, botUserID, botRoleID)
	})

	t.Run("bit 47 on the everyone role", func(t *testing.T) {
		runEveryoneBit47ClearedSubtest(t, guildID, botUserID, botRoleID)
	})
}

// findChannelOverwrite finds a specific overwrite for a subject ID in a named channel.
func findChannelOverwrite(channels []fakeChannel, channelName string, subjectID snowflake.ID) *fakeOverwrite {
	for i := range channels {
		if channels[i].name != channelName {
			continue
		}
		for j := range channels[i].overwrites {
			if channels[i].overwrites[j].id == subjectID {
				return &channels[i].overwrites[j]
			}
		}
	}
	return nil
}

// setupUnknownBitChannelFixture initializes roles and sets an unknown bit on the home-lab everyone overwrite.
func setupUnknownBitChannelFixture(t *testing.T, fake *fakePlatform, guildID, botUserID, botRoleID uint64) {
	t.Helper()
	_, guildJSON, _, channelsJSON := buildPlatformFixtureFromDef(t, guildID, botUserID, botRoleID)
	var apiGuild struct {
		Roles []fakeRole `json:"roles"`
	}
	if err := json.Unmarshal([]byte(guildJSON), &apiGuild); err != nil {
		t.Fatalf("Unmarshal guild roles: %v", err)
	}
	var chs []fakeChannel
	if err := json.Unmarshal([]byte(channelsJSON), &chs); err != nil {
		t.Fatalf("Unmarshal channels: %v", err)
	}
	fake.roles = apiGuild.Roles
	for i := range fake.roles {
		if fake.roles[i].name == "Diallak" {
			fake.roles[i].permissions = "8"
		}
	}
	fake.channels = chs

	targetOw := findChannelOverwrite(fake.channels, "home-lab", snowflake.ID(guildID))
	if targetOw == nil {
		t.Fatal("home-lab channel or @everyone overwrite not found in fixture")
	}
	targetOw.allow = everyoneWithBit47
	targetOw.deny = "0"
}

// assertUnknownBitRefusalAndApplyError verifies refusal details and failure behavior when an unknown bit is present.
func assertUnknownBitRefusalAndApplyError(t *testing.T, res serverplan.Result, writes []recordedCall, err error) {
	t.Helper()
	var foundRefusal *serverplan.Refusal
	for i := range res.Refused {
		if res.Refused[i].Action == serverplan.ActionSetAccess && res.Refused[i].Object == "channels[home-lab].access[everyone]" {
			foundRefusal = &res.Refused[i]
			break
		}
	}
	if foundRefusal == nil {
		t.Fatalf("plan holds no refusal for (set-access, channels[home-lab].access[everyone]): %+v", res.Refused)
	}
	if foundRefusal.Reason != serverplan.ReasonUnknownBits {
		t.Errorf("refusal reason = %q, want %q", foundRefusal.Reason, serverplan.ReasonUnknownBits)
	}

	if err == nil {
		t.Fatal("apply succeeded on a channel entry with an unnamed bit, want error")
	}
	want := fmt.Sprintf("the plan holds %d refused changes; a plan with a refusal is not applied", len(res.Refused))
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if len(writes) != 0 {
		t.Errorf("apply recorded %d writes, want 0: first %s %s", len(writes), writes[0].method, writes[0].path)
	}
}

func TestApplyServer_Negative_UnknownBitOnAnEntryStopsTheRun(t *testing.T) {
	const (
		guildID   uint64 = 1001
		botUserID uint64 = 2001
		botRoleID uint64 = 3001
	)
	fake := setupFreshServerFake(t, guildID, botUserID, botRoleID, true)
	setupUnknownBitChannelFixture(t, fake, guildID, botUserID, botRoleID)

	res, writes, err := applyOnFake(t, fake, guildID, botUserID)
	assertUnknownBitRefusalAndApplyError(t, res, writes, err)
}
