package serverplan

import (
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
	if ctx.hasInvitePlanChange(ActionCreateChannel, "channels["+ch.ID+"]") {
		return "", ""
	}
	observed := ctx.observedInviteChannel(ch.Name)
	if observed != nil && observed.MemberEntries > 0 {
		return ReasonLacksRight, "CREATE_INSTANT_INVITE: channel holds entries for single members"
	}
	if ctx.hasInvitePlanChange(ActionSetAccess, "channels["+ch.ID+"].access["+ctx.botRoleID+"]") {
		return "", ""
	}
	if ctx.hasObservedInviteRight(observed) {
		return "", ""
	}
	if ctx.rights.holds("CREATE_INSTANT_INVITE") && !ctx.inviteDenied(ch, observed) {
		return "", ""
	}
	return ReasonLacksRight, "CREATE_INSTANT_INVITE"
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

func (ctx *planContext) hasObservedInviteRight(ch *Channel) bool {
	if ch == nil {
		return false
	}
	for _, entry := range ch.Access {
		if entry.Role == ctx.snap.Caller.Role && slices.Contains(entry.Allow, "CREATE_INSTANT_INVITE") {
			return true
		}
	}
	return false
}

func (ctx *planContext) inviteDenied(ch *serverdef.Channel, observed *Channel) bool {
	for _, entry := range ch.Access {
		if (entry.Role == ctx.botRoleID || entry.Role == everyoneID(ctx.def)) && slices.Contains(entry.Deny, "CREATE_INSTANT_INVITE") {
			return true
		}
	}
	everyone := ctx.observedEveryoneName()
	if observed != nil {
		for _, entry := range observed.Access {
			if (entry.Role == ctx.snap.Caller.Role || entry.Role == everyone) && slices.Contains(entry.Deny, "CREATE_INSTANT_INVITE") {
				return true
			}
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
