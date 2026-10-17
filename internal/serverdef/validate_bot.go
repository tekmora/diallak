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
	for _, right := range botRights {
		if !inSet(right, botRole.Permissions) {
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
		allowed := botChannelCeiling(ch)
		p := fmt.Sprintf("channels[%s].access[%s]", ch.ID, botRole.ID)
		for _, perm := range acc.Allow {
			if _, ok := allowed[perm]; !ok {
				v.add(CodeBotRights, p, fmt.Sprintf("bot channel entry allows %s beyond channel ceiling", perm))
			}
		}
	}
}

// botChannelCeiling returns what an entry of the bot's role may allow in ch: what the functions
// of ch's kind need.
func botChannelCeiling(ch Channel) map[string]struct{} {
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
	if ch.Type == "announcement" {
		// The bot posts own releases and advisories there and crossposts them (ADR-0019).
		allowed["VIEW_CHANNEL"] = struct{}{}
		allowed["SEND_MESSAGES"] = struct{}{}
	}
	return allowed
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

// checkTrail keeps one trail channel, a staff channel whose history the bot reads: the bot lists
// its own lines there to delete them after the retention (ADR-0021).
func (v *validator) checkTrail() {
	botRole := findRoleByKind(v.def.Roles, "bot")
	trails := 0
	for _, ch := range v.def.Channels {
		if ch.Trail {
			trails++
		}
	}
	for _, ch := range v.def.Channels {
		if !ch.Trail {
			continue
		}
		path := fmt.Sprintf("channels[%s]", ch.ID)
		if trails > 1 {
			v.add(CodeTrail, path+".trail", "the definition holds more than one trail channel")
		}
		if !ch.Staff {
			v.add(CodeTrail, path+".trail", "a trail channel must be a staff channel")
		}
		if botRole == nil {
			continue
		}
		if _, ok := Effective(v.def, botRole.ID, ch.ID)["READ_MESSAGE_HISTORY"]; !ok {
			v.add(CodeTrail, fmt.Sprintf("%s.access[%s]", path, botRole.ID),
				"bot in the trail channel lacks READ_MESSAGE_HISTORY")
		}
	}
}
