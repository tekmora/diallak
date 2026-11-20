package discord

import (
	"strings"
	"testing"
)

func TestReadInvitesWithManageGuild(t *testing.T) {
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
