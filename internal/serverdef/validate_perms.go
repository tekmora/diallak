package serverdef

import "fmt"

func (v *validator) checkPermissions() {
	vocabMap := make(map[string]struct{}, len(permissionsTable))
	for i := range permissionsTable {
		vocabMap[permissionsTable[i].name] = struct{}{}
	}
	v.checkRolePermissionNames(vocabMap)
	v.checkChannelPermissionNames(vocabMap)
}

func (v *validator) checkRolePermissionNames(vocabMap map[string]struct{}) {
	for _, r := range v.def.Roles {
		p := fmt.Sprintf("roles[%s].permissions", r.ID)
		for _, perm := range r.Permissions {
			if _, ok := vocabMap[perm]; !ok {
				v.add(CodePermissionName, p, fmt.Sprintf("unknown permission %q", perm))
			}
		}
	}
}

func (v *validator) checkChannelPermissionNames(vocabMap map[string]struct{}) {
	for _, ch := range v.def.Channels {
		p := fmt.Sprintf("channels[%s]", ch.ID)
		for _, acc := range ch.Access {
			checkSingleAccessPermNames(v, fmt.Sprintf("%s.access[%s]", p, acc.Role), acc, vocabMap)
		}
	}
}

func checkSingleAccessPermNames(v *validator, accPath string, acc Access, vocabMap map[string]struct{}) {
	for _, perm := range acc.Allow {
		if _, ok := vocabMap[perm]; !ok {
			v.add(CodePermissionName, accPath+".allow", fmt.Sprintf("unknown permission %q", perm))
		}
	}
	for _, perm := range acc.Deny {
		if _, ok := vocabMap[perm]; !ok {
			v.add(CodePermissionName, accPath+".deny", fmt.Sprintf("unknown permission %q", perm))
		}
	}
}

func checkDuplicates(list []string) bool {
	seen := make(map[string]struct{}, len(list))
	for _, item := range list {
		if _, ok := seen[item]; ok {
			return true
		}
		seen[item] = struct{}{}
	}
	return false
}

func (v *validator) checkAccessShape() {
	for _, r := range v.def.Roles {
		if checkDuplicates(r.Permissions) {
			v.add(CodeAccessShape, fmt.Sprintf("roles[%s].permissions", r.ID),
				"duplicate permission in role permissions")
		}
	}
	for _, ch := range v.def.Channels {
		v.checkSingleChannelAccessShape(ch)
	}
}

func (v *validator) checkSingleChannelAccessShape(ch Channel) {
	p := fmt.Sprintf("channels[%s]", ch.ID)
	seenRoles := make(map[string]struct{}, len(ch.Access))
	for _, acc := range ch.Access {
		accPath := fmt.Sprintf("%s.access[%s]", p, acc.Role)
		if _, ok := seenRoles[acc.Role]; ok {
			v.add(CodeAccessShape, accPath, fmt.Sprintf("duplicate access entry for role %q", acc.Role))
		}
		seenRoles[acc.Role] = struct{}{}
		checkSingleAccessLists(v, accPath, acc)
	}
}

func checkSingleAccessLists(v *validator, accPath string, acc Access) {
	if checkDuplicates(acc.Allow) {
		v.add(CodeAccessShape, accPath+".allow", "duplicate permission in allow list")
	}
	if checkDuplicates(acc.Deny) {
		v.add(CodeAccessShape, accPath+".deny", "duplicate permission in deny list")
	}
	allowSet := make(map[string]struct{}, len(acc.Allow))
	for _, perm := range acc.Allow {
		allowSet[perm] = struct{}{}
	}
	for _, perm := range acc.Deny {
		if _, ok := allowSet[perm]; ok {
			v.add(CodeAccessShape, accPath, fmt.Sprintf("permission %q in both allow and deny", perm))
		}
	}
}

func (v *validator) checkMentionEveryone() {
	for _, r := range v.def.Roles {
		if r.Kind == "operator" {
			continue
		}
		p := fmt.Sprintf("roles[%s].permissions", r.ID)
		for _, perm := range r.Permissions {
			if perm == "MENTION_EVERYONE" {
				v.add(CodeMentionEveryone, p, "non-operator role has MENTION_EVERYONE in base permissions")
			}
		}
	}
	for _, ch := range v.def.Channels {
		v.checkChannelMentionEveryone(ch)
	}
}

func (v *validator) checkChannelMentionEveryone(ch Channel) {
	roleMap := make(map[string]Role, len(v.def.Roles))
	for _, r := range v.def.Roles {
		roleMap[r.ID] = r
	}
	p := fmt.Sprintf("channels[%s]", ch.ID)
	checkChannelAllowMentionEveryone(v, p, ch.Access, roleMap)
	checkChannelEffectiveMentionEveryone(v, p, ch.ID)
}

func checkChannelAllowMentionEveryone(v *validator, p string, access []Access, roleMap map[string]Role) {
	for _, acc := range access {
		r, ok := roleMap[acc.Role]
		if ok && r.Kind != "operator" {
			for _, perm := range acc.Allow {
				if perm == "MENTION_EVERYONE" {
					v.add(CodeMentionEveryone, fmt.Sprintf("%s.access[%s]", p, acc.Role),
						"non-operator role has MENTION_EVERYONE in channel allow list")
				}
			}
		}
	}
}

func checkChannelEffectiveMentionEveryone(v *validator, p, channelID string) {
	for _, r := range v.def.Roles {
		if r.Kind != "operator" {
			eff := granted(v.def, r.ID, channelID)
			if _, ok := eff["MENTION_EVERYONE"]; ok {
				v.add(CodeMentionEveryone, fmt.Sprintf("%s.access[%s]", p, r.ID),
					"non-operator role has effective MENTION_EVERYONE in channel")
			}
		}
	}
}

func (v *validator) checkChannelOperatorOnly(ch Channel, r Role, opPerms []string) {
	if r.Kind == "operator" {
		return
	}
	p := fmt.Sprintf("channels[%s].access[%s]", ch.ID, r.ID)
	eff := granted(v.def, r.ID, ch.ID)
	for _, perm := range opPerms {
		if r.Kind == "bot" && inSet(perm, botRights[:]) {
			continue
		}
		if _, ok := eff[perm]; ok {
			v.add(CodeOperatorOnly, p, fmt.Sprintf("operator-only permission %s effective for role %s", perm, r.ID))
		}
	}
}

func (v *validator) checkOperatorOnly() {
	opPerms := []string{
		"ADMINISTRATOR", "MANAGE_GUILD", "MANAGE_ROLES", "MANAGE_CHANNELS",
		"MANAGE_WEBHOOKS", "BAN_MEMBERS", "KICK_MEMBERS",
	}
	for _, ch := range v.def.Channels {
		for _, r := range v.def.Roles {
			v.checkChannelOperatorOnly(ch, r, opPerms)
		}
	}
}

func (v *validator) checkChannelModerationOnly(ch Channel, r Role, modPerms []string) {
	if r.Kind == "operator" {
		return
	}
	p := fmt.Sprintf("channels[%s].access[%s]", ch.ID, r.ID)
	eff := granted(v.def, r.ID, ch.ID)
	for _, perm := range modPerms {
		if r.Kind == "bot" && inSet(perm, botRights[:]) {
			continue
		}
		if _, ok := eff[perm]; ok {
			v.add(CodeModerationOnly, p, fmt.Sprintf("moderation-only permission %s effective for role %s", perm, r.ID))
		}
	}
}

func (v *validator) checkModeratorRolePermissions() {
	for _, r := range v.def.Roles {
		if r.Kind == "moderator" {
			path := fmt.Sprintf("roles[%s].permissions", r.ID)
			for _, perm := range r.Permissions {
				v.add(CodeModerationOnly, path, fmt.Sprintf("role kind moderator must hold no permission; %s is the bot's to use", perm))
			}
		}
	}
}

func (v *validator) checkModerationOnly() {
	v.checkModeratorRolePermissions()
	modPerms := []string{
		"MANAGE_MESSAGES", "MANAGE_THREADS", "MODERATE_MEMBERS",
		"MUTE_MEMBERS", "MOVE_MEMBERS", "VIEW_AUDIT_LOG",
	}
	for _, ch := range v.def.Channels {
		for _, r := range v.def.Roles {
			v.checkChannelModerationOnly(ch, r, modPerms)
		}
	}
}

func (v *validator) checkNothingAttaches() {
	roleMap := make(map[string]Role, len(v.def.Roles))
	for _, r := range v.def.Roles {
		roleMap[r.ID] = r
		if r.Kind == "recognition" || r.Kind == "interest" || r.Kind == "language" || r.Kind == "managed" || r.Kind == "notification" {
			if len(r.Permissions) > 0 {
				v.add(CodeNothingAttaches, fmt.Sprintf("roles[%s].permissions", r.ID),
					fmt.Sprintf("role kind %s must have empty permissions list", r.Kind))
			}
		}
	}
	for _, ch := range v.def.Channels {
		v.checkChannelNothingAttaches(ch, roleMap)
	}
}

func (v *validator) checkChannelNothingAttaches(ch Channel, roleMap map[string]Role) {
	p := fmt.Sprintf("channels[%s]", ch.ID)
	for _, acc := range ch.Access {
		r, ok := roleMap[acc.Role]
		if !ok {
			continue
		}
		accPath := fmt.Sprintf("%s.access[%s]", p, acc.Role)
		if (r.Kind == "recognition" || r.Kind == "notification") && len(acc.Allow) > 0 {
			v.add(CodeNothingAttaches, accPath, fmt.Sprintf("%s role must have empty allow list", r.Kind))
		}
		if r.Kind == "interest" || r.Kind == "language" {
			checkInterestOrLangAllow(v, accPath, r.Kind, acc.Allow)
		}
	}
}

func checkInterestOrLangAllow(v *validator, accPath, kind string, allow []string) {
	for _, perm := range allow {
		if perm != "VIEW_CHANNEL" {
			v.add(CodeNothingAttaches, accPath,
				fmt.Sprintf("%s role allow list may only contain VIEW_CHANNEL", kind))
		}
	}
}

func (v *validator) checkRoleGrants() {
	v.checkRoleAssignedBy()
	v.checkChoiceGrantRoles()
	v.checkRoleDisplayFields()
}

func (v *validator) checkRoleAssignedBy() {
	for _, r := range v.def.Roles {
		p := fmt.Sprintf("roles[%s].assigned_by", r.ID)
		checkSingleRoleAssignedBy(v, p, r)
	}
}

func checkSingleRoleAssignedBy(v *validator, p string, r Role) {
	if r.AssignedBy == "integration" && r.Kind != "managed" {
		v.add(CodeRoleGrant, p, "assigned_by integration is only for role kind managed")
		return
	}
	switch r.Kind {
	case "operator", "moderator":
		if r.AssignedBy != "operator" {
			v.add(CodeRoleGrant, p, fmt.Sprintf("role kind %s must have assigned_by operator", r.Kind))
		}
	case "interest", "language", "notification":
		if r.AssignedBy != "choice" {
			v.add(CodeRoleGrant, p, fmt.Sprintf("role kind %s must have assigned_by choice", r.Kind))
		}
	case "everyone", "bot":
		if r.AssignedBy != "platform" {
			v.add(CodeRoleGrant, p, fmt.Sprintf("role kind %s must have assigned_by platform", r.Kind))
		}
	case "recognition", "managed":
		checkMultiGrantRoleAssignedBy(v, p, r)
	}
}

func checkMultiGrantRoleAssignedBy(v *validator, p string, r Role) {
	if r.Kind == "recognition" && r.AssignedBy != "operator" && r.AssignedBy != "bot" {
		v.add(CodeRoleGrant, p, "recognition role must have assigned_by operator or bot")
	}
	if r.Kind == "managed" && r.AssignedBy != "platform" && r.AssignedBy != "integration" {
		v.add(CodeRoleGrant, p, "role kind managed must have assigned_by platform or integration")
	}
}

func (v *validator) checkRoleDisplayFields() {
	for _, r := range v.def.Roles {
		checkSingleRoleDisplayFields(v, r)
	}
}

func checkSingleRoleDisplayFields(v *validator, r Role) {
	p := fmt.Sprintf("roles[%s]", r.ID)
	switch r.Kind {
	case "managed":
		if r.Mentionable != nil {
			v.add(CodeRoleGrant, p+".mentionable", "role kind managed must not state mentionable")
		}
	case "everyone", "bot":
		if r.ShownApart != nil {
			v.add(CodeRoleGrant, p+".shown_apart", fmt.Sprintf("role kind %s must not state shown_apart or mentionable", r.Kind))
		}
		if r.Mentionable != nil {
			v.add(CodeRoleGrant, p+".mentionable", fmt.Sprintf("role kind %s must not state shown_apart or mentionable", r.Kind))
		}
	case "notification":
		if r.Mentionable == nil || !*r.Mentionable {
			v.add(CodeRoleGrant, p+".mentionable", "role kind notification must state mentionable true")
		}
	}
}
