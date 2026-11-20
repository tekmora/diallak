package serverplan

import (
	"fmt"
	"strings"

	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	// KindInvitesNotRead reports invites that could not be listed.
	KindInvitesNotRead = "invites-not-read"
	// KindInviteMissing reports a missing recorded invite.
	KindInviteMissing = "invite-missing"
	// KindInviteCreate reports an invite that needs creation.
	KindInviteCreate = "invite-create"
	// KindInviteDiffers reports a recorded invite with different fields.
	KindInviteDiffers = "invite-differs"
	// KindInviteExtra reports an unmatched bot invite.
	KindInviteExtra = "invite-extra"
)

func inviteFields(def *serverdef.Definition, want serverdef.Invite, got Invite) []string {
	var fields []string
	if defChannelName(def, want.Channel) != got.Channel {
		fields = append(fields, "channel")
	}
	if want.MaxAge != got.MaxAge {
		fields = append(fields, "max_age")
	}
	if want.MaxUses != got.MaxUses {
		fields = append(fields, "max_uses")
	}
	if want.Temporary != got.Temporary {
		fields = append(fields, "temporary")
	}
	return fields
}

func inviteMatches(def *serverdef.Definition, want serverdef.Invite, got Invite) bool {
	if !got.ByBot {
		return false
	}
	if want.Code != "" {
		return want.Code == got.Code
	}
	return len(inviteFields(def, want, got)) == 0
}

func compareInvites(def *serverdef.Definition, snap *Snapshot) []Difference {
	if len(def.Invites) == 0 {
		return nil
	}
	if !snap.InvitesRead {
		return []Difference{{Kind: KindInvitesNotRead, Object: "invites"}}
	}
	var diffs []Difference
	matched := make([]bool, len(snap.Invites))
	for _, want := range def.Invites {
		if diff := compareInvite(def, want, snap.Invites, matched); diff != nil {
			diffs = append(diffs, *diff)
		}
	}
	for i, invite := range snap.Invites {
		if invite.ByBot && !matched[i] {
			diffs = append(diffs, Difference{Kind: KindInviteExtra, Object: fmt.Sprintf("server.invites[%s]", invite.Code)})
		}
	}
	return diffs
}

func compareInvite(def *serverdef.Definition, want serverdef.Invite, invites []Invite, matched []bool) *Difference {
	index := -1
	for i, got := range invites {
		if inviteMatches(def, want, got) {
			matched[i] = true
			if index < 0 {
				index = i
			}
		}
	}
	object := fmt.Sprintf("invites[%s]", want.ID)
	if index < 0 {
		kind := KindInviteCreate
		if want.Code != "" {
			kind = KindInviteMissing
		}
		return &Difference{Kind: kind, Object: object}
	}
	if fields := inviteFields(def, want, invites[index]); len(fields) > 0 {
		return &Difference{Kind: KindInviteDiffers, Object: object, Detail: strings.Join(fields, ", ")}
	}
	return nil
}
