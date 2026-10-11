package serverplan

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	// KindRoleMissing indicates a defined role does not exist on the server.
	KindRoleMissing = "role-missing"
	// KindRoleAmbiguous indicates multiple server roles match a single defined role.
	KindRoleAmbiguous = "role-ambiguous"
	// KindRoleName indicates the server caller role has a different name than defined.
	KindRoleName = "role-name"
	// KindRolePermissions indicates a server role's permissions differ from definition.
	KindRolePermissions = "role-permissions"
	// KindRoleOrder indicates the relative ordering of roles differs from definition.
	KindRoleOrder = "role-order"
	// KindCategoryMissing indicates a defined category does not exist on the server.
	KindCategoryMissing = "category-missing"
	// KindCategoryAmbiguous indicates multiple server categories match a single defined category.
	KindCategoryAmbiguous = "category-ambiguous"
	// KindChannelMissing indicates a defined channel is missing when channels are complete.
	KindChannelMissing = "channel-missing"
	// KindChannelNotVisible indicates a channel is missing when channel reading is incomplete.
	KindChannelNotVisible = "channel-not-visible"
	// KindChannelAmbiguous indicates multiple server channels match a single defined channel.
	KindChannelAmbiguous = "channel-ambiguous"
	// KindChannelType indicates a channel's type differs from definition.
	KindChannelType = "channel-type"
	// KindChannelCategory indicates a channel's parent category differs from definition.
	KindChannelCategory = "channel-category"
	// KindAccessMissing indicates a role permission entry is missing on a channel.
	KindAccessMissing = "access-missing"
	// KindAccessDiffers indicates a channel permission entry differs in allow or deny sets.
	KindAccessDiffers = "access-differs"
	// KindAccessExtra indicates an unexpected permission entry exists on a channel.
	KindAccessExtra = "access-extra"
	// KindAccessMember indicates a channel has permission entries naming single members.
	KindAccessMember = "access-member"
	// KindRoleExtra indicates a role exists on the server that is not in the definition.
	KindRoleExtra = "role-extra"
	// KindCategoryExtra indicates a category exists on the server not in the definition.
	KindCategoryExtra = "category-extra"
	// KindChannelExtra indicates a channel exists on the server not in the definition.
	KindChannelExtra = "channel-extra"
	// KindFeatureMissing indicates the server lacks a feature required by the definition.
	KindFeatureMissing = "feature-missing"
	// KindUnknownBits indicates a server role or permission entry has bits set
	// that the permission table does not name.
	KindUnknownBits = "unknown-permission-bits"
)

// Difference describes one discrepancy between the declarative definition and the snapshot.
type Difference struct {
	Kind   string
	Object string
	Detail string
}

// Compare checks snapshot s against definition def and returns all observed differences.
func Compare(def *serverdef.Definition, s *Snapshot) ([]Difference, error) {
	if err := Check(s); err != nil {
		return nil, fmt.Errorf("check snapshot: %w", err)
	}
	findings := serverdef.Validate(def)
	if len(findings) > 0 {
		return nil, fmt.Errorf("validate definition: %d findings", len(findings))
	}

	var diffs []Difference
	diffs = append(diffs, compareRoles(def, s)...)
	if roDiff := compareRoleOrder(def, s); roDiff != nil {
		diffs = append(diffs, *roDiff)
	}
	diffs = append(diffs, compareCategories(def, s)...)
	botRoleID := findBotRoleID(def)
	diffs = append(diffs, compareChannels(def, s, botRoleID)...)
	diffs = append(diffs, compareExtras(def, s)...)
	if featDiff := compareFeatures(def, s); featDiff != nil {
		diffs = append(diffs, *featDiff)
	}
	diffs = append(diffs, compareUnknownBits(s)...)
	return diffs, nil
}

// bitList renders bit numbers as "47, 53".
func bitList(bits []int) string {
	parts := make([]string, 0, len(bits))
	for _, b := range bits {
		parts = append(parts, fmt.Sprintf("%d", b))
	}
	return strings.Join(parts, ", ")
}

// accessBitsDetail renders the unknown bits of an entry by side, or "" for none.
func accessBitsDetail(acc *Access) string {
	var parts []string
	if len(acc.UnknownAllow) > 0 {
		parts = append(parts, "allow "+bitList(acc.UnknownAllow))
	}
	if len(acc.UnknownDeny) > 0 {
		parts = append(parts, "deny "+bitList(acc.UnknownDeny))
	}
	return strings.Join(parts, "; ")
}

// compareUnknownBits reports every role and every permission entry of the
// server that has a bit set the permission table does not name. The objects
// carry the server's names, because the finding does not depend on the
// definition: a role or channel the definition does not hold is reported too.
func compareUnknownBits(s *Snapshot) []Difference {
	var diffs []Difference
	for i := range s.Roles {
		sr := &s.Roles[i]
		if len(sr.UnknownBits) > 0 {
			diffs = append(diffs, Difference{
				Kind:   KindUnknownBits,
				Object: fmt.Sprintf("server.roles[%s]", sr.Name),
				Detail: "bits " + bitList(sr.UnknownBits),
			})
		}
	}
	for i := range s.Channels {
		sch := &s.Channels[i]
		for j := range sch.Access {
			if detail := accessBitsDetail(&sch.Access[j]); detail != "" {
				diffs = append(diffs, Difference{
					Kind:   KindUnknownBits,
					Object: fmt.Sprintf("server.channels[%s].access[%s]", sch.Name, sch.Access[j].Role),
					Detail: detail,
				})
			}
		}
	}
	return diffs
}

func findBotRoleID(def *serverdef.Definition) string {
	for i := range def.Roles {
		if def.Roles[i].Kind == "bot" {
			return def.Roles[i].ID
		}
	}
	return ""
}

func matchEveryoneRole(roles []Role) []Role {
	var matches []Role
	for i := range roles {
		if roles[i].Everyone {
			matches = append(matches, roles[i])
		}
	}
	return matches
}

func matchBotRole(roles []Role, callerRole string) []Role {
	var matches []Role
	for i := range roles {
		if roles[i].Name == callerRole {
			matches = append(matches, roles[i])
		}
	}
	return matches
}

func matchNamedRole(roles []Role, name, callerRole string) []Role {
	var matches []Role
	for i := range roles {
		r := &roles[i]
		if !r.Everyone && r.Name != callerRole && r.Name == name {
			matches = append(matches, *r)
		}
	}
	return matches
}

func matchRoleInSnapshot(defRole *serverdef.Role, s *Snapshot) []Role {
	switch defRole.Kind {
	case "everyone":
		return matchEveryoneRole(s.Roles)
	case "bot":
		return matchBotRole(s.Roles, s.Caller.Role)
	default:
		return matchNamedRole(s.Roles, defRole.Name, s.Caller.Role)
	}
}

func permDiffDetail(defPerms, snapPerms []string) (string, bool) {
	snapMap := make(map[string]struct{}, len(snapPerms))
	for _, p := range snapPerms {
		snapMap[p] = struct{}{}
	}
	defMap := make(map[string]struct{}, len(defPerms))
	for _, p := range defPerms {
		defMap[p] = struct{}{}
	}

	var lacks []string
	for _, p := range defPerms {
		if _, ok := snapMap[p]; !ok {
			lacks = append(lacks, p)
		}
	}
	var beyond []string
	for _, p := range snapPerms {
		if _, ok := defMap[p]; !ok {
			beyond = append(beyond, p)
		}
	}
	if len(lacks) == 0 && len(beyond) == 0 {
		return "", false
	}
	sort.Strings(lacks)
	sort.Strings(beyond)

	if len(lacks) > 0 && len(beyond) > 0 {
		return fmt.Sprintf("lacks %s; beyond %s", strings.Join(lacks, ", "), strings.Join(beyond, ", ")), true
	}
	if len(lacks) > 0 {
		return fmt.Sprintf("lacks %s", strings.Join(lacks, ", ")), true
	}
	return fmt.Sprintf("beyond %s", strings.Join(beyond, ", ")), true
}

func compareRoles(def *serverdef.Definition, s *Snapshot) []Difference {
	var diffs []Difference
	for i := range def.Roles {
		r := &def.Roles[i]
		obj := fmt.Sprintf("roles[%s]", r.ID)
		matches := matchRoleInSnapshot(r, s)
		switch len(matches) {
		case 0:
			diffs = append(diffs, Difference{Kind: KindRoleMissing, Object: obj})
		case 1:
			sr := matches[0]
			if r.Kind == "bot" && sr.Name != r.Name {
				diffs = append(diffs, Difference{
					Kind:   KindRoleName,
					Object: obj,
					Detail: fmt.Sprintf("server: %s; definition: %s", sr.Name, r.Name),
				})
			}
			if detail, diff := permDiffDetail(r.Permissions, sr.Permissions); diff {
				diffs = append(diffs, Difference{Kind: KindRolePermissions, Object: obj, Detail: detail})
			}
		default:
			diffs = append(diffs, Difference{Kind: KindRoleAmbiguous, Object: obj, Detail: fmt.Sprintf("%d", len(matches))})
		}
	}
	return diffs
}

func getMatchedDefRoleMap(def *serverdef.Definition, s *Snapshot) map[string]string {
	matched := make(map[string]string, len(def.Roles))
	for i := range def.Roles {
		dr := &def.Roles[i]
		if dr.Kind == "everyone" {
			continue
		}
		matches := matchRoleInSnapshot(dr, s)
		if len(matches) == 1 {
			// Uniquely matched definition role
			if dr.Kind == "bot" {
				matched[s.Caller.Role] = dr.ID
			} else {
				matched[matches[0].Name] = dr.ID
			}
		}
	}
	return matched
}

func compareRoleOrder(def *serverdef.Definition, s *Snapshot) *Difference {
	matchedMap := getMatchedDefRoleMap(def, s)
	var o []string
	oSet := make(map[string]struct{}, len(s.Roles))
	for i := range s.Roles {
		sr := &s.Roles[i]
		if sr.Everyone {
			continue
		}
		if id, ok := matchedMap[sr.Name]; ok {
			o = append(o, id)
			oSet[id] = struct{}{}
		}
	}

	var t []string
	for _, id := range def.RoleOrder {
		if _, ok := oSet[id]; ok {
			t = append(t, id)
		}
	}

	if !slices.Equal(o, t) {
		return &Difference{
			Kind:   KindRoleOrder,
			Object: "role_order",
			Detail: fmt.Sprintf("server: %s; definition: %s", strings.Join(o, ", "), strings.Join(t, ", ")),
		}
	}
	return nil
}

func compareCategories(def *serverdef.Definition, s *Snapshot) []Difference {
	var diffs []Difference
	for i := range def.Categories {
		c := &def.Categories[i]
		obj := fmt.Sprintf("categories[%s]", c.ID)
		count := 0
		for j := range s.Categories {
			if s.Categories[j].Name == c.Name {
				count++
			}
		}
		if count == 0 {
			diffs = append(diffs, Difference{Kind: KindCategoryMissing, Object: obj})
		} else if count >= 2 {
			diffs = append(diffs, Difference{Kind: KindCategoryAmbiguous, Object: obj, Detail: fmt.Sprintf("%d", count)})
		}
	}
	return diffs
}

func compareChannels(def *serverdef.Definition, s *Snapshot, botRoleID string) []Difference {
	var diffs []Difference
	catNameMap := make(map[string]string, len(def.Categories))
	for i := range def.Categories {
		catNameMap[def.Categories[i].ID] = def.Categories[i].Name
	}

	for i := range def.Channels {
		ch := &def.Channels[i]
		obj := fmt.Sprintf("channels[%s]", ch.ID)
		var matches []Channel
		for j := range s.Channels {
			if s.Channels[j].Name == ch.Name {
				matches = append(matches, s.Channels[j])
			}
		}
		switch len(matches) {
		case 0:
			diffs = append(diffs, handleMissingChannel(def, s, ch, obj, botRoleID))
		case 1:
			diffs = append(diffs, compareSingleChannel(def, s, ch, &matches[0], catNameMap)...)
		default:
			diffs = append(diffs, Difference{Kind: KindChannelAmbiguous, Object: obj, Detail: fmt.Sprintf("%d", len(matches))})
		}
	}
	return diffs
}

func handleMissingChannel(def *serverdef.Definition, s *Snapshot, ch *serverdef.Channel, obj, botRoleID string) Difference {
	if s.ChannelsComplete {
		return Difference{Kind: KindChannelMissing, Object: obj}
	}
	eff := serverdef.Effective(def, botRoleID, ch.ID)
	if _, ok := eff["VIEW_CHANNEL"]; !ok {
		return Difference{Kind: KindChannelNotVisible, Object: obj, Detail: "hidden from the bot by the definition"}
	}
	return Difference{Kind: KindChannelNotVisible, Object: obj, Detail: "missing or hidden"}
}

func compareSingleChannel(def *serverdef.Definition, s *Snapshot, ch *serverdef.Channel, sch *Channel, catNames map[string]string) []Difference {
	var diffs []Difference
	obj := fmt.Sprintf("channels[%s]", ch.ID)
	if sch.Type != ch.Type {
		diffs = append(diffs, Difference{
			Kind:   KindChannelType,
			Object: obj,
			Detail: fmt.Sprintf("server: %s; definition: %s", sch.Type, ch.Type),
		})
	}
	defCatName := catNames[ch.Category]
	if sch.Category != defCatName {
		diffs = append(diffs, Difference{
			Kind:   KindChannelCategory,
			Object: obj,
			Detail: fmt.Sprintf("server: %s; definition: %s", sch.Category, defCatName),
		})
	}
	diffs = append(diffs, compareAccessEntries(def, s, ch, sch)...)
	if sch.MemberEntries > 0 {
		diffs = append(diffs, memberEntriesDifference(obj, sch.MemberEntries))
	}
	return diffs
}

// memberEntriesDifference reports the entries a channel holds for single members.
// The definition gives access to roles only, so every such entry is a difference.
func memberEntriesDifference(object string, entries int) Difference {
	return Difference{
		Kind:   KindAccessMember,
		Object: object,
		Detail: fmt.Sprintf("entries for single members: %d", entries),
	}
}

func setEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	mA := make(map[string]struct{}, len(a))
	for _, v := range a {
		mA[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := mA[v]; !ok {
			return false
		}
	}
	return true
}

func compareAccessEntries(def *serverdef.Definition, s *Snapshot, ch *serverdef.Channel, sch *Channel) []Difference {
	roleIDMap := make(map[string]*serverdef.Role, len(def.Roles))
	for i := range def.Roles {
		roleIDMap[def.Roles[i].ID] = &def.Roles[i]
	}

	snapAccessByRole := make(map[string]*Access, len(sch.Access))
	for i := range sch.Access {
		acc := &sch.Access[i]
		if len(acc.Allow) > 0 || len(acc.Deny) > 0 {
			snapAccessByRole[acc.Role] = acc
		}
	}

	diffs, claimedRoles := checkDefinedAccessEntries(s, ch, roleIDMap, snapAccessByRole)
	diffs = append(diffs, checkExtraAccessEntries(ch, sch, claimedRoles)...)
	return diffs
}

func checkDefinedAccessEntries(s *Snapshot, ch *serverdef.Channel, roleMap map[string]*serverdef.Role, snapAccess map[string]*Access) ([]Difference, map[string]struct{}) {
	var diffs []Difference
	claimedRoles := make(map[string]struct{}, len(ch.Access))
	for i := range ch.Access {
		defAcc := &ch.Access[i]
		accessObj := fmt.Sprintf("channels[%s].access[%s]", ch.ID, defAcc.Role)
		matchedSRs := matchRoleInSnapshot(roleMap[defAcc.Role], s)
		if len(matchedSRs) != 1 {
			diffs = append(diffs, Difference{Kind: KindAccessMissing, Object: accessObj})
			continue
		}
		snapRoleName := matchedSRs[0].Name
		claimedRoles[snapRoleName] = struct{}{}
		snapAcc, hasEntry := snapAccess[snapRoleName]
		if !hasEntry {
			diffs = append(diffs, Difference{Kind: KindAccessMissing, Object: accessObj})
			continue
		}
		diffs = append(diffs, checkAccessDiffers(accessObj, defAcc, snapAcc)...)
	}
	return diffs, claimedRoles
}

func checkExtraAccessEntries(ch *serverdef.Channel, sch *Channel, claimed map[string]struct{}) []Difference {
	var diffs []Difference
	for i := range sch.Access {
		snapAcc := &sch.Access[i]
		if len(snapAcc.Allow) == 0 && len(snapAcc.Deny) == 0 {
			continue
		}
		if _, ok := claimed[snapAcc.Role]; !ok {
			diffs = append(diffs, Difference{
				Kind:   KindAccessExtra,
				Object: fmt.Sprintf("channels[%s].access[%s]", ch.ID, snapAcc.Role),
			})
		}
	}
	return diffs
}

func checkAccessDiffers(accessObj string, defAcc *serverdef.Access, snapAcc *Access) []Difference {
	allowDiff := !setEqual(defAcc.Allow, snapAcc.Allow)
	denyDiff := !setEqual(defAcc.Deny, snapAcc.Deny)
	if !allowDiff && !denyDiff {
		return nil
	}
	var parts []string
	if allowDiff {
		parts = append(parts, "allow")
	}
	if denyDiff {
		parts = append(parts, "deny")
	}
	return []Difference{
		{
			Kind:   KindAccessDiffers,
			Object: accessObj,
			Detail: strings.Join(parts, ", "),
		},
	}
}

func isExtraRole(def *serverdef.Definition, sr *Role, callerRole string) bool {
	if sr.Everyone || sr.Name == callerRole {
		return false
	}
	for i := range def.Roles {
		dr := &def.Roles[i]
		if dr.Kind != "everyone" && dr.Kind != "bot" && dr.Name == sr.Name {
			return false
		}
	}
	return true
}

func compareRoleExtras(def *serverdef.Definition, s *Snapshot) []Difference {
	var diffs []Difference
	for i := range s.Roles {
		sr := &s.Roles[i]
		if isExtraRole(def, sr, s.Caller.Role) {
			detail := ""
			if sr.Managed {
				detail = "managed"
			}
			diffs = append(diffs, Difference{
				Kind:   KindRoleExtra,
				Object: fmt.Sprintf("server.roles[%s]", sr.Name),
				Detail: detail,
			})
		}
	}
	return diffs
}

func compareCategoryExtras(def *serverdef.Definition, s *Snapshot) []Difference {
	var diffs []Difference
	for i := range s.Categories {
		sc := &s.Categories[i]
		found := false
		for j := range def.Categories {
			if def.Categories[j].Name == sc.Name {
				found = true
				break
			}
		}
		if !found {
			diffs = append(diffs, Difference{
				Kind:   KindCategoryExtra,
				Object: fmt.Sprintf("server.categories[%s]", sc.Name),
			})
		}
	}
	return diffs
}

func compareChannelExtras(def *serverdef.Definition, s *Snapshot) []Difference {
	var diffs []Difference
	for i := range s.Channels {
		sch := &s.Channels[i]
		found := false
		for j := range def.Channels {
			if def.Channels[j].Name == sch.Name {
				found = true
				break
			}
		}
		if !found {
			diffs = append(diffs, Difference{
				Kind:   KindChannelExtra,
				Object: fmt.Sprintf("server.channels[%s]", sch.Name),
			})
			if sch.MemberEntries > 0 {
				diffs = append(diffs, memberEntriesDifference(fmt.Sprintf("server.channels[%s]", sch.Name), sch.MemberEntries))
			}
		}
	}
	return diffs
}

func compareExtras(def *serverdef.Definition, s *Snapshot) []Difference {
	var diffs []Difference
	diffs = append(diffs, compareRoleExtras(def, s)...)
	diffs = append(diffs, compareCategoryExtras(def, s)...)
	diffs = append(diffs, compareChannelExtras(def, s)...)
	return diffs
}

func compareFeatures(def *serverdef.Definition, s *Snapshot) *Difference {
	hasAnnouncement := false
	for i := range def.Channels {
		if def.Channels[i].Type == "announcement" {
			hasAnnouncement = true
			break
		}
	}
	if !hasAnnouncement {
		return nil
	}
	hasNews := false
	for _, f := range s.Features {
		if f == "NEWS" {
			hasNews = true
			break
		}
	}
	if !hasNews {
		return &Difference{
			Kind:   KindFeatureMissing,
			Object: "features[NEWS]",
		}
	}
	return nil
}
