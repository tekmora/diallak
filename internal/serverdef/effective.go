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

func computeBasePermissions(def *Definition, roleID string) map[string]struct{} {
	base := make(map[string]struct{})
	everyoneRole := findRole(def.Roles, "everyone")
	if everyoneRole != nil {
		for _, p := range everyoneRole.Permissions {
			base[p] = struct{}{}
		}
	}
	if roleID != "everyone" {
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

// Effective calculates the effective permissions of roleID within channelID.
// It follows Discord's documented permission overwrite hierarchy.
func Effective(def *Definition, roleID, channelID string) map[string]struct{} {
	if def == nil {
		return make(map[string]struct{})
	}
	ch := findChannel(def.Channels, channelID)
	if ch == nil {
		return make(map[string]struct{})
	}

	result := computeBasePermissions(def, roleID)
	if _, isAdmin := result["ADMINISTRATOR"]; isAdmin {
		all := make(map[string]struct{}, len(vocabulary))
		for _, p := range vocabulary {
			all[p] = struct{}{}
		}
		return all
	}

	everyoneAcc := findAccess(ch.Access, "everyone")
	applyAccessOverwrite(result, everyoneAcc)

	if roleID != "everyone" {
		roleAcc := findAccess(ch.Access, roleID)
		applyAccessOverwrite(result, roleAcc)
	}

	if _, canView := result["VIEW_CHANNEL"]; !canView {
		return make(map[string]struct{})
	}
	return result
}
