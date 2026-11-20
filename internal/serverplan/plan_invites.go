package serverplan

// ActionCreateInvite creates an unrecorded invite.
const ActionCreateInvite = "create-invite"

func (ctx *planContext) addInviteCandidates() {
	for _, diff := range ctx.diffs {
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
	for _, observed := range ctx.snap.Channels {
		if observed.Name != ch.Name {
			continue
		}
		for _, entry := range observed.Access {
			if entry.Role == ctx.snap.Caller.Role {
				for _, right := range entry.Allow {
					if right == "CREATE_INSTANT_INVITE" {
						return "", ""
					}
				}
			}
		}
	}
	return ReasonLacksRight, "CREATE_INSTANT_INVITE"
}
