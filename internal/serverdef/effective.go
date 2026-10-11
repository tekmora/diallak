package serverdef

func findRole(roles []Role, id string) *Role {
	for i := range roles {
		if roles[i].ID == id {
			return &roles[i]
		}
	}
	return nil
}

func findChannel(channels []Channel, id string) *Channel {
	for i := range channels {
		if channels[i].ID == id {
			return &channels[i]
		}
	}
	return nil
}

func findAccess(accessList []Access, roleID string) *Access {
	for i := range accessList {
		if accessList[i].Role == roleID {
			return &accessList[i]
		}
	}
	return nil
}

// everyoneID returns the id of the role every member holds, which is the role of kind everyone.
// It returns the empty string when the definition has no such role.
func everyoneID(def *Definition) string {
	for i := range def.Roles {
		if def.Roles[i].Kind == "everyone" {
			return def.Roles[i].ID
		}
	}
	return ""
}

func computeBasePermissions(def *Definition, everyone, roleID string) map[string]struct{} {
	base := make(map[string]struct{})
	if everyone != "" {
		if everyoneRole := findRole(def.Roles, everyone); everyoneRole != nil {
			for _, p := range everyoneRole.Permissions {
				base[p] = struct{}{}
			}
		}
	}
	if roleID != everyone {
		r := findRole(def.Roles, roleID)
		if r != nil {
			for _, p := range r.Permissions {
				base[p] = struct{}{}
			}
		}
	}
	return base
}

func applyAccessOverwrite(perms map[string]struct{}, acc *Access) {
	if acc == nil {
		return
	}
	for _, p := range acc.Deny {
		delete(perms, p)
	}
	for _, p := range acc.Allow {
		perms[p] = struct{}{}
	}
}

// Effective calculates the effective permissions of roleID within channelID for a member who holds
// only that role. It follows Discord's documented order: the permissions of the role every member
// holds and of the role itself, then the channel's entry for every member, then the entry for the
// role, each applied as deny before allow. An administrator has every permission, and a channel
// that cannot be seen leaves none.
func Effective(def *Definition, roleID, channelID string) map[string]struct{} {
	if def == nil {
		return make(map[string]struct{})
	}
	ch := findChannel(def.Channels, channelID)
	if ch == nil {
		return make(map[string]struct{})
	}

	everyone := everyoneID(def)
	result := computeBasePermissions(def, everyone, roleID)
	if _, isAdmin := result["ADMINISTRATOR"]; isAdmin {
		all := make(map[string]struct{}, len(vocabulary))
		for _, p := range vocabulary {
			all[p] = struct{}{}
		}
		return all
	}

	if everyone != "" {
		applyAccessOverwrite(result, findAccess(ch.Access, everyone))
	}

	if roleID != everyone {
		roleAcc := findAccess(ch.Access, roleID)
		applyAccessOverwrite(result, roleAcc)
	}

	if _, canView := result["VIEW_CHANNEL"]; !canView {
		return make(map[string]struct{})
	}
	return result
}
