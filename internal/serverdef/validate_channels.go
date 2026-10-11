package serverdef

import "fmt"

func (v *validator) checkTicketThreadReader() {
	for _, ch := range v.def.Channels {
		if !ch.PrivateThreads {
			continue
		}
		p := fmt.Sprintf("channels[%s]", ch.ID)
		for _, r := range v.def.Roles {
			if r.Kind == "operator" || r.Kind == "moderator" {
				continue
			}
			eff := Effective(v.def, r.ID, ch.ID)
			if _, ok := eff["MANAGE_THREADS"]; ok {
				v.add(CodeTicketThreadReader, fmt.Sprintf("%s.access[%s]", p, r.ID),
					"non-moderator role has effective MANAGE_THREADS in ticket channel")
			}
		}
	}
}

func (v *validator) checkTicketShape() {
	for _, ch := range v.def.Channels {
		if !ch.PrivateThreads {
			continue
		}
		p := fmt.Sprintf("channels[%s]", ch.ID)
		if ch.Type != "text" {
			v.add(CodeTicketShape, p+".type", "ticket channel must have type text")
		}
		if ch.Opens != "start" {
			v.add(CodeTicketShape, p+".opens", "ticket channel must have opens start")
		}
		eff := Effective(v.def, "everyone", ch.ID)
		if _, ok := eff["VIEW_CHANNEL"]; !ok {
			v.add(CodeTicketShape, p+".access[everyone]", "everyone must have VIEW_CHANNEL in tickets")
		}
		if _, ok := eff["CREATE_PRIVATE_THREADS"]; !ok {
			v.add(CodeTicketShape, p+".access[everyone]",
				"everyone must have CREATE_PRIVATE_THREADS in tickets")
		}
		if _, ok := eff["SEND_MESSAGES_IN_THREADS"]; !ok {
			v.add(CodeTicketShape, p+".access[everyone]",
				"everyone must have SEND_MESSAGES_IN_THREADS in tickets")
		}
		if _, ok := eff["SEND_MESSAGES"]; ok {
			v.add(CodeTicketShape, p+".access[everyone]", "everyone must not have SEND_MESSAGES in tickets")
		}
	}
}

func (v *validator) checkStaffVisibility() {
	for _, ch := range v.def.Channels {
		if !ch.Staff {
			continue
		}
		p := fmt.Sprintf("channels[%s]", ch.ID)
		for _, r := range v.def.Roles {
			if r.Kind == "operator" || r.Kind == "moderator" || r.Kind == "bot" {
				continue
			}
			eff := Effective(v.def, r.ID, ch.ID)
			if _, ok := eff["VIEW_CHANNEL"]; ok {
				v.add(CodeStaffVisibility, fmt.Sprintf("%s.access[%s]", p, r.ID),
					"staff channel visible to unauthorized role")
			}
		}
	}
}

func (v *validator) checkFirstAnswer() {
	for _, ch := range v.def.Channels {
		p := fmt.Sprintf("channels[%s]", ch.ID)
		if ch.Type != "forum" && len(ch.Tags) > 0 {
			v.add(CodeFirstAnswer, p+".tags", "non-forum channel must not carry tags")
		}
		if ch.Type == "forum" && len(ch.Tags) > MaxForumTags {
			v.add(CodeFirstAnswer, p+".tags", "forum channel exceeds 20 tags limit")
		}
		if !ch.FirstAnswer {
			continue
		}
		if ch.Type != "forum" {
			v.add(CodeFirstAnswer, p+".type", "first_answer channel must be forum")
		}
		v.checkFirstAnswerTagsAndBot(p, ch)
	}
}

func (v *validator) checkFirstAnswerTagsAndBot(path string, ch Channel) {
	optOutCount := 0
	for _, tag := range ch.Tags {
		if tag.OptOut {
			optOutCount++
			if tag.Moderated {
				v.add(CodeFirstAnswer, fmt.Sprintf("%s.tags[%s]", path, tag.Name),
					"opt_out tag must have moderated false")
			}
		}
	}
	if optOutCount != 1 {
		v.add(CodeFirstAnswer, path+".tags", "first_answer forum must have exactly one opt_out tag")
	}
	botRole := findRoleByKind(v.def.Roles, "bot")
	if botRole != nil {
		eff := Effective(v.def, botRole.ID, ch.ID)
		needed := []string{"VIEW_CHANNEL", "READ_MESSAGE_HISTORY", "SEND_MESSAGES_IN_THREADS"}
		for _, perm := range needed {
			if _, ok := eff[perm]; !ok {
				v.add(CodeFirstAnswer, fmt.Sprintf("%s.access[%s]", path, botRole.ID),
					fmt.Sprintf("bot missing %s in first_answer forum", perm))
			}
		}
	}
}

func findRoleByKind(roles []Role, kind string) *Role {
	for i := range roles {
		if roles[i].Kind == kind {
			return &roles[i]
		}
	}
	return nil
}

func (v *validator) checkVoiceClosed() {
	for _, ch := range v.def.Channels {
		if ch.Type != "voice" {
			continue
		}
		p := fmt.Sprintf("channels[%s].access[everyone]", ch.ID)
		eff := Effective(v.def, "everyone", ch.ID)
		if _, ok := eff["CONNECT"]; ok {
			v.add(CodeVoiceClosed, p, "everyone must not have effective CONNECT in voice channel")
		}
		if _, ok := eff["USE_VAD"]; ok {
			v.add(CodeVoiceClosed, p, "everyone must not have effective USE_VAD in voice channel")
		}
	}
}
