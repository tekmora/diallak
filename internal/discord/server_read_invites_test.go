package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/serverplan"
)

func TestReadInvites_Positive_WithManageGuild(t *testing.T) {
	snap, requests, err := readSettingsSnapshot(t, settingsGuildJSON(manageGuildPerms, 0), `[]`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range requests {
		if strings.HasSuffix(r.path, "/invites") {
			found = true
		}
	}
	if !found || !snap.InvitesRead {
		t.Fatalf("invites read=%v, requests=%v", snap.InvitesRead, requests)
	}
}

func TestReadInvites_Positive_NoInviterInSnapshot(t *testing.T) {
	const guildID, botUserID, botRoleID uint64 = 1001, 2001, 3001
	f := buildPlatformFixture(t, guildID, botUserID, botRoleID)
	def := mustDefinition(t)
	channel := fixtureChannelIDs(def)["rules"]
	handler := standardHandler(guildID, botUserID, f.app, f.guild, f.member, f.channels, f.rules)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == fmt.Sprintf("/guilds/%d/invites", guildID) {
			writeTestResponse(w, http.StatusOK, fmt.Sprintf(`[{"code":"abc","channel":{"id":%q,"name":"rules","type":0},"inviter":{"id":"2001","username":"private-name"},"max_age":0,"max_uses":0,"temporary":false}]`, channel))
			return
		}
		handler(w, r)
	}))
	defer srv.Close()
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(botUserID), guildID, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Invites) != 1 || !snap.Invites[0].ByBot || snap.Invites[0].Channel != "rules" || snap.Invites[0].ChannelUnlisted {
		t.Fatalf("invites: %+v", snap.Invites)
	}
	data, err := json.Marshal(snap.Invites)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "inviter") || strings.Contains(string(data), "private-name") || strings.Contains(string(data), "2001") {
		t.Fatalf("inviter leaked: %s", data)
	}
}

func TestReadInvites_Positive_UnlistedChannels(t *testing.T) {
	const guildID, botUserID, botRoleID uint64 = 1001, 2001, 3001
	f := buildPlatformFixture(t, guildID, botUserID, botRoleID)
	handler := standardHandler(guildID, botUserID, f.app, f.guild, f.member, f.channels, f.rules)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == fmt.Sprintf("/guilds/%d/invites", guildID) {
			writeTestResponse(w, http.StatusOK, `[{"code":"other","channel":{"id":"99991","name":"","type":0},"inviter":{"id":"9999"}},{"code":"bot","channel":{"id":"99992","name":"hidden","type":0},"inviter":{"id":"2001"}}]`)
			return
		}
		handler(w, r)
	}))
	defer srv.Close()
	snap, err := readServerWithOptions(context.Background(), makeFakeToken(botUserID), guildID, clientOptions{restURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	assertUnlistedInvites(t, snap.Invites)
	diffs, err := serverplan.Compare(mustDefinition(t), snap)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diffs {
		if d.Object == "server.invites[other]" {
			t.Fatalf("other user's invite reported: %+v", diffs)
		}
	}
}

func assertUnlistedInvites(t *testing.T, invites []serverplan.Invite) {
	t.Helper()
	if len(invites) != 2 {
		t.Fatalf("invites: %+v", invites)
	}
	if invites[0].ByBot || invites[0].Channel != "" || !invites[0].ChannelUnlisted {
		t.Fatalf("invites: %+v", invites)
	}
	if !invites[1].ByBot || invites[1].Channel != "hidden" || !invites[1].ChannelUnlisted {
		t.Fatalf("invites: %+v", invites)
	}
}
