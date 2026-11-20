package serverplan

import (
	"fmt"
	"strings"

	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	KindInviteMissing = "invite-missing"
	KindInviteCreate  = "invite-create"
	KindInviteDiffers = "invite-differs"
	KindInviteExtra   = "invite-extra"
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
	if !snap.InvitesRead || len(def.Invites) == 0 {
		return nil
	}
	var diffs []Difference
	matched := make([]bool, len(snap.Invites))
	for _, want := range def.Invites {
		index := -1
		for i, got := range snap.Invites {
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
			diffs = append(diffs, Difference{Kind: kind, Object: object})
			continue
		}
		if fields := inviteFields(def, want, snap.Invites[index]); len(fields) > 0 {
			diffs = append(diffs, Difference{Kind: KindInviteDiffers, Object: object, Detail: strings.Join(fields, ", ")})
		}
	}
	for i, invite := range snap.Invites {
		if invite.ByBot && !matched[i] {
			diffs = append(diffs, Difference{Kind: KindInviteExtra, Object: fmt.Sprintf("server.invites[%s]", invite.Code)})
		}
	}
	return diffs
}
