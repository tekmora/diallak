package serverdef

import "fmt"

func (v *validator) checkRoleOrder() {
	orderCounts := make(map[string]int, len(v.def.RoleOrder))
	firstIndex := make(map[string]int, len(v.def.RoleOrder))
	for i, id := range v.def.RoleOrder {
		orderCounts[id]++
		if _, ok := firstIndex[id]; !ok {
			firstIndex[id] = i
		}
	}

	v.checkRoleOrderEntries(orderCounts)
	v.checkRoleOrderPresence(orderCounts)
	v.checkRoleOrderPositions(firstIndex, orderCounts)
}

func (v *validator) checkRoleOrderEntries(orderCounts map[string]int) {
	reportedDuplicates := make(map[string]struct{}, len(orderCounts))
	reportedEveryone := make(map[string]struct{}, 1)
	for _, entry := range v.def.RoleOrder {
		r := findRole(v.def.Roles, entry)
		if r == nil {
			v.add(CodeRef, fmt.Sprintf("role_order[%s]", entry),
				fmt.Sprintf("role_order entry %q is not a role", entry))
			continue
		}
		if orderCounts[entry] > 1 {
			if _, reported := reportedDuplicates[entry]; !reported {
				v.add(CodeRoleOrder, fmt.Sprintf("role_order[%s]", entry),
					fmt.Sprintf("role %q is listed twice in role_order", entry))
				reportedDuplicates[entry] = struct{}{}
			}
		}
		if r.Kind == "everyone" {
			if _, reported := reportedEveryone[entry]; !reported {
				v.add(CodeRoleOrder, fmt.Sprintf("role_order[%s]", entry),
					fmt.Sprintf("role %q of kind everyone must not be listed in role_order", entry))
				reportedEveryone[entry] = struct{}{}
			}
		}
	}
}

func (v *validator) checkRoleOrderPresence(orderCounts map[string]int) {
	for _, r := range v.def.Roles {
		if r.Kind != "everyone" && orderCounts[r.ID] == 0 {
			v.add(CodeRoleOrder, fmt.Sprintf("roles[%s]", r.ID),
				fmt.Sprintf("role %q is not listed in role_order", r.ID))
		}
	}
}

func (v *validator) checkRoleOrderPositions(firstIndex, orderCounts map[string]int) {
	botRole := findRoleByKind(v.def.Roles, "bot")
	if botRole == nil {
		return
	}
	if orderCounts[botRole.ID] == 0 {
		return
	}
	botIdx := firstIndex[botRole.ID]

	for _, r := range v.def.Roles {
		v.checkSingleRolePosition(r, botIdx, firstIndex)
	}
}

func (v *validator) checkSingleRolePosition(r Role, botIdx int, firstIndex map[string]int) {
	if r.Kind == "everyone" {
		return
	}
	rIdx, ok := firstIndex[r.ID]
	if !ok {
		return
	}
	if r.AssignedBy == "operator" && rIdx > botIdx {
		v.add(CodeRoleOrder, fmt.Sprintf("role_order[%s]", r.ID),
			fmt.Sprintf("role %q assigned by operator must stand above the bot role", r.ID))
		return
	}
	if r.AssignedBy == "bot" || r.AssignedBy == "choice" {
		if rIdx < botIdx {
			v.add(CodeRoleOrder, fmt.Sprintf("role_order[%s]", r.ID),
				fmt.Sprintf("role %q assigned by %s must stand below the bot role", r.ID, r.AssignedBy))
		}
	}
}
