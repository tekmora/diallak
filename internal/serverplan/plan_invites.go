package serverplan

import "slices"

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
	ch := ctx.defChannel(c.channelID)
	if ch == nil {
		return ReasonLacksRight, "CREATE_INSTANT_INVITE"
	}
	for _, change := range ctx.changes {
		if change.Action == ActionCreateChannel && change.Object == "channels["+ch.ID+"]" {
			return "", ""
		}
		if change.Action == ActionSetAccess && change.Object == "channels["+ch.ID+"].access["+ctx.botRoleID+"]" {
			return "", ""
		}
	}
	if ctx.hasObservedInviteRight(ch.Name) {
		return "", ""
	}
	return ReasonLacksRight, "CREATE_INSTANT_INVITE"
}

func (ctx *planContext) hasObservedInviteRight(name string) bool {
	for _, observed := range ctx.snap.Channels {
		if observed.Name != name {
			continue
		}
		for _, entry := range observed.Access {
			if entry.Role == ctx.snap.Caller.Role && slices.Contains(entry.Allow, "CREATE_INSTANT_INVITE") {
				return true
			}
		}
	}
	return false
}
