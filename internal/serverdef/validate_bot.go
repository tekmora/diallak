package serverdef

import "fmt"

func (v *validator) checkBotRights() {
	botRole := findRoleByKind(v.def.Roles, "bot")
	if botRole == nil {
		return
	}
	v.checkBotRolePermissions(botRole)
	v.checkBotChannelCeilings(botRole)
	v.checkBotStaffChannels(botRole)
	v.checkBotTicketChannels(botRole)
}

func (v *validator) checkBotRolePermissions(botRole *Role) {
	path := fmt.Sprintf("roles[%s].permissions", botRole.ID)
	permSet := make(map[string]struct{}, len(botRole.Permissions))
	for _, p := range botRole.Permissions {
		permSet[p] = struct{}{}
	}
	for _, right := range botRights {
		if _, ok := permSet[right]; !ok {
			v.add(CodeBotRights, path, fmt.Sprintf("bot role lacks %s", right))
		}
	}
	for _, p := range botRole.Permissions {
		if !inSet(p, botRights[:]) {
			v.add(CodeBotRights, path, fmt.Sprintf("bot role permission %s is beyond the bot rights set", p))
		}
	}
}

func (v *validator) checkBotChannelCeilings(botRole *Role) {
	for _, ch := range v.def.Channels {
		acc := findAccess(ch.Access, botRole.ID)
		if acc == nil {
			continue
		}
		allowed := make(map[string]struct{}, 4)
		if ch.FirstAnswer {
			allowed["VIEW_CHANNEL"] = struct{}{}
			allowed["READ_MESSAGE_HISTORY"] = struct{}{}
			allowed["SEND_MESSAGES_IN_THREADS"] = struct{}{}
		}
		if ch.Staff {
			allowed["VIEW_CHANNEL"] = struct{}{}
			allowed["SEND_MESSAGES"] = struct{}{}
		}
		if ch.Area != "" {
			allowed["VIEW_CHANNEL"] = struct{}{}
		}

		p := fmt.Sprintf("channels[%s].access[%s]", ch.ID, botRole.ID)
		for _, perm := range acc.Allow {
			if _, ok := allowed[perm]; !ok {
				v.add(CodeBotRights, p, fmt.Sprintf("bot channel entry allows %s beyond channel ceiling", perm))
			}
		}
	}
}

func (v *validator) checkBotStaffChannels(botRole *Role) {
	for _, ch := range v.def.Channels {
		if !ch.Staff {
			continue
		}
		eff := Effective(v.def, botRole.ID, ch.ID)
		p := fmt.Sprintf("channels[%s].access[%s]", ch.ID, botRole.ID)
		if _, ok := eff["VIEW_CHANNEL"]; !ok {
			v.add(CodeBotRights, p, "bot in staff channel lacks VIEW_CHANNEL")
		}
		if _, ok := eff["SEND_MESSAGES"]; !ok {
			v.add(CodeBotRights, p, "bot in staff channel lacks SEND_MESSAGES")
		}
	}
}

func (v *validator) checkBotTicketChannels(botRole *Role) {
	for _, ch := range v.def.Channels {
		if !ch.PrivateThreads {
			continue
		}
		eff := Effective(v.def, botRole.ID, ch.ID)
		if _, ok := eff["VIEW_CHANNEL"]; ok {
			v.add(CodeBotRights, fmt.Sprintf("channels[%s].access[%s]", ch.ID, botRole.ID),
				fmt.Sprintf("bot must not see ticket channel %s", ch.ID))
		}
	}
}
