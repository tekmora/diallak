package serverplan

import (
	"fmt"
	"slices"

	"github.com/tekmora/diallak/internal/serverdef"
)

// ActionCreateInvite creates an unrecorded invite.
const ActionCreateInvite = "create-invite"

func (ctx *planContext) addInviteCandidates() {
	for _, diff := range ctx.diffs {
		if diff.Kind == KindInvitesNotRead {
			ctx.addUnreadInviteCandidates()
			continue
		}
		if diff.Kind != KindInviteCreate {
			continue
		}
		id := extractBracketID(diff.Object)
		for _, invite := range ctx.def.Invites {
			if invite.ID == id {
				ctx.evaluateCandidate(candidate{action: ActionCreateInvite, object: diff.Object, needs: []string{"CREATE_INSTANT_INVITE"}, channelID: invite.Channel})
				break
			}
		}
	}
}

func (ctx *planContext) addUnreadInviteCandidates() {
	for _, invite := range ctx.def.Invites {
		if invite.Code == "" {
			ctx.evaluateCandidate(candidate{action: ActionCreateInvite, object: "invites[" + invite.ID + "]", needs: []string{"CREATE_INSTANT_INVITE"}, channelID: invite.Channel, isNotVisible: true})
		}
	}
}

func (ctx *planContext) checkInviteRight(c candidate) (string, string) {
	if ctx.rights.holds("ADMINISTRATOR") {
		return "", ""
	}
	ch := ctx.defChannel(c.channelID)
	if ch == nil {
		return ReasonLacksRight, "CREATE_INSTANT_INVITE"
	}
	if ctx.inviteDenied(ch) {
		return ReasonLacksRight, "CREATE_INSTANT_INVITE"
	}
	projected := ctx.projectedInviteChannel(ch, ctx.observedInviteChannel(ch.Name))
	if projected != nil && projected.MemberEntries > 0 {
		return ReasonLacksRight, "CREATE_INSTANT_INVITE: channel holds entries for single members"
	}
	if projected != nil {
		return ctx.checkObservedInviteRight(projected)
	}
	if ctx.rights.holds("CREATE_INSTANT_INVITE") {
		return "", ""
	}
	return ReasonLacksRight, "CREATE_INSTANT_INVITE"
}

func (ctx *planContext) projectedInviteChannel(ch *serverdef.Channel, observed *Channel) *Channel {
	created := ctx.hasInvitePlanChange(ActionCreateChannel, "channels["+ch.ID+"]")
	if !created && observed == nil {
		return nil
	}
	projected := &Channel{Name: ch.Name}
	if !created {
		projected.Access = slices.Clone(observed.Access)
		projected.MemberEntries = observed.MemberEntries
	}
	for _, entry := range ch.Access {
		if !created && !ctx.hasInvitePlanChange(ActionSetAccess, "channels["+ch.ID+"].access["+entry.Role+"]") {
			continue
		}
		converted, ok := ctx.inviteAccessSnapshot(entry)
		if !ok {
			continue
		}
		index := slices.IndexFunc(projected.Access, func(access Access) bool { return access.Role == converted.Role })
		if index < 0 {
			projected.Access = append(projected.Access, converted)
		} else {
			projected.Access[index] = converted
		}
	}
	return projected
}

func (ctx *planContext) inviteAccessSnapshot(entry serverdef.Access) (Access, bool) {
	var name string
	switch entry.Role {
	case everyoneID(ctx.def):
		name = ctx.observedEveryoneName()
	case ctx.botRoleID:
		name = ctx.snap.Caller.Role
	default:
		role := ctx.defRole(entry.Role)
		if role == nil {
			return Access{}, false
		}
		name = role.Name
	}
	return Access{Role: name, Allow: slices.Clone(entry.Allow), Deny: slices.Clone(entry.Deny)}, true
}

func (ctx *planContext) checkObservedInviteRight(observed *Channel) (string, string) {
	if detail := ctx.inviteRoleAmbiguity(observed); detail != "" {
		return ReasonLacksRight, detail
	}
	if !ctx.observedInviteRight(observed, "VIEW_CHANNEL") {
		return ReasonLacksRight, "CREATE_INSTANT_INVITE: the bot cannot view the channel"
	}
	if !ctx.observedInviteRight(observed, "CREATE_INSTANT_INVITE") {
		return ReasonLacksRight, "CREATE_INSTANT_INVITE"
	}
	return "", ""
}

func (ctx *planContext) hasInvitePlanChange(action, object string) bool {
	for _, change := range ctx.changes {
		if change.Action == action && change.Object == object {
			return true
		}
	}
	return false
}

func (ctx *planContext) observedInviteChannel(name string) *Channel {
	for i := range ctx.snap.Channels {
		if ctx.snap.Channels[i].Name == name {
			return &ctx.snap.Channels[i]
		}
	}
	return nil
}

func (ctx *planContext) inviteRoleAmbiguity(ch *Channel) string {
	roles := ctx.snap.Caller.Roles
	if len(roles) == 0 {
		for _, entry := range ch.Access {
			if entry.Role != ctx.observedEveryoneName() && entry.Role != ctx.snap.Caller.Role {
				return "CREATE_INSTANT_INVITE: the snapshot names no held roles"
			}
		}
		roles = []string{ctx.snap.Caller.Role}
	}
	for _, name := range roles {
		count := 0
		for _, role := range ctx.snap.Roles {
			if role.Name == name {
				count++
			}
		}
		if count > 1 {
			return fmt.Sprintf("CREATE_INSTANT_INVITE: role name %s names several roles", name)
		}
	}
	return ""
}

func (ctx *planContext) observedInviteRight(ch *Channel, right string) bool {
	allowed := ctx.rights.holds(right)
	everyone := ctx.observedEveryoneName()
	roles := ctx.snap.Caller.Roles
	if len(roles) == 0 {
		roles = []string{ctx.snap.Caller.Role}
	}
	roleAllow, roleDeny := false, false
	for _, entry := range ch.Access {
		if entry.Role == everyone {
			if slices.Contains(entry.Deny, right) {
				allowed = false
			}
			if slices.Contains(entry.Allow, right) {
				allowed = true
			}
		} else if slices.Contains(roles, entry.Role) {
			roleAllow = roleAllow || slices.Contains(entry.Allow, right)
			roleDeny = roleDeny || slices.Contains(entry.Deny, right)
		}
	}
	if roleAllow {
		return true
	}
	if roleDeny {
		return false
	}
	return allowed
}

func (ctx *planContext) inviteDenied(ch *serverdef.Channel) bool {
	for _, entry := range ch.Access {
		if (entry.Role == ctx.botRoleID || entry.Role == everyoneID(ctx.def)) && slices.Contains(entry.Deny, "CREATE_INSTANT_INVITE") {
			return true
		}
	}
	return false
}

func (ctx *planContext) observedEveryoneName() string {
	for _, role := range ctx.snap.Roles {
		if role.Everyone {
			return role.Name
		}
	}
	return ""
}
