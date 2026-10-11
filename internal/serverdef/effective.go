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

// Effective calculates what a member who holds only roleID can do in channelID. It is what the
// role is granted there, and nothing when the role cannot see the channel.
func Effective(def *Definition, roleID, channelID string) map[string]struct{} {
	result := granted(def, roleID, channelID)
	if _, canView := result["VIEW_CHANNEL"]; !canView {
		return make(map[string]struct{})
	}
	return result
}

// granted calculates the permissions roleID is granted within channelID, whether or not the role
// can see the channel. It follows Discord's documented order: the permissions of the role every
// member holds and of the role itself, then the channel's entry for every member, then the entry
// for the role, each applied as deny before allow. An administrator is granted every permission.
//
// The rules that refuse a right read this set and not Effective: a member can hold a second role
// that makes the channel visible, and such a member can do at most what the roles are granted
// one by one.
func granted(def *Definition, roleID, channelID string) map[string]struct{} {
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
		all := make(map[string]struct{}, len(permissionsTable))
		for i := range permissionsTable {
			all[permissionsTable[i].name] = struct{}{}
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
	return result
}
