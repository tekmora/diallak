package serverdef

import (
	"fmt"
	"regexp"
	"strings"
)

var inviteCodeRegexp = regexp.MustCompile(`^[A-Za-z0-9-]{1,32}$`)

func (v *validator) checkInvites() {
	seen := make(map[string]bool, len(v.def.Invites))
	for _, invite := range v.def.Invites {
		p := fmt.Sprintf("invites[%s]", invite.ID)
		if !idRegexp.MatchString(invite.ID) || seen[invite.ID] {
			v.add(CodeInvite, p+".id", "invalid or duplicate invite id")
		}
		seen[invite.ID] = true
		v.checkInviteFields(invite, p)
	}
}

func (v *validator) checkInviteFields(invite Invite, p string) {
	ch := findChannel(v.def.Channels, invite.Channel)
	if ch == nil || (ch.Type != "text" && ch.Type != "announcement") {
		v.add(CodeInvite, p+".channel", "invite channel must name a text or announcement channel")
	}
	if strings.TrimSpace(invite.Purpose) == "" {
		v.add(CodeInvite, p+".purpose", "invite purpose must not be blank")
	}
	if invite.MaxAge < 0 || invite.MaxAge > 604800 {
		v.add(CodeInvite, p+".max_age", "max_age must be between 0 and 604800")
	}
	if invite.MaxUses < 0 || invite.MaxUses > 100 {
		v.add(CodeInvite, p+".max_uses", "max_uses must be between 0 and 100")
	}
	if invite.Code != "" && !inviteCodeRegexp.MatchString(invite.Code) {
		v.add(CodeInvite, p+".code", "invalid invite code")
	}
}

func (v *validator) invitesChannel(id string) bool {
	for _, invite := range v.def.Invites {
		if invite.Channel == id {
			return true
		}
	}
	return false
}

func (v *validator) checkInviteBotRights(role *Role) {
	for _, invite := range v.def.Invites {
		ch := findChannel(v.def.Channels, invite.Channel)
		if ch == nil {
			continue
		}
		acc := findAccess(ch.Access, role.ID)
		if acc == nil || !inSet("CREATE_INSTANT_INVITE", acc.Allow) {
			v.add(CodeBotRights, fmt.Sprintf("channels[%s].access[%s]", ch.ID, role.ID), "invite channel bot entry lacks CREATE_INSTANT_INVITE")
		}
	}
}
