package serverplan

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	// ActionCreateRole is the candidate action to create a missing role.
	ActionCreateRole = "create-role"
	// ActionSetRolePermissions is the candidate action to update role permissions.
	ActionSetRolePermissions = "set-role-permissions"
	// ActionSetRoleOrder is the candidate action to reorder roles.
	ActionSetRoleOrder = "set-role-order"
	// ActionCreateCategory is the candidate action to create a missing category.
	ActionCreateCategory = "create-category"
	// ActionCreateChannel is the candidate action to create a missing channel.
	ActionCreateChannel = "create-channel"
	// ActionMoveChannel is the candidate action to move a channel to its category.
	ActionMoveChannel = "move-channel"
	// ActionSetAccess is the candidate action to set a channel permission overwrite.
	ActionSetAccess = "set-access"
	// ActionConvertChannel is the candidate action for a channel type mismatch.
	ActionConvertChannel = "convert-channel"
	// ActionResolveName is the candidate action when an entity name is ambiguous.
	ActionResolveName = "resolve-name"
)

const (
	// ReasonAmbiguousName indicates the action cannot proceed due to duplicate names.
	ReasonAmbiguousName = "ambiguous-name"
	// ReasonTypeDiffers indicates channel type conversion is not supported.
	ReasonTypeDiffers = "type-differs"
	// ReasonNotVisible indicates the channel is missing only because it is not visible.
	ReasonNotVisible = "not-visible"
	// ReasonOutsideOwnChanges indicates the unraised bot cannot make this change.
	ReasonOutsideOwnChanges = "outside-own-changes"
	// ReasonMissingFeature indicates the server lacks a required feature.
	ReasonMissingFeature = "missing-feature"
	// ReasonManagedRole indicates managed integration roles cannot be modified.
	ReasonManagedRole = "managed-role"
	// ReasonAboveCaller indicates the caller cannot modify roles at or above its position.
	ReasonAboveCaller = "above-caller"
	// ReasonLacksRight indicates the caller lacks permissions required for the change.
	ReasonLacksRight = "lacks-right"
	// ReasonUnknownBits indicates an entry has bits set that the permission
	// table does not name (entries only; a role-permission change clears such
	// bits and names them).
	ReasonUnknownBits = "unknown-permission-bits"
)

// Change describes an accepted step in the execution plan.
type Change struct {
	Step   int
	Action string
	Object string
	Needs  []string
	Detail string
}

// Refusal describes a candidate action rejected by the planner and the reason why.
type Refusal struct {
	Action string
	Object string
	Needs  []string
	Reason string
	Detail string
}

// Result holds the planned changes, refusals, and whether the caller is raised.
type Result struct {
	Raised  bool
	Changes []Change
	Refused []Refusal
}

// Rights returns the sorted, unique union of permissions needed by changes and refusals.
func (r Result) Rights() []string {
	set := make(map[string]struct{})
	for _, c := range r.Changes {
		for _, n := range c.Needs {
			set[n] = struct{}{}
		}
	}
	for _, ref := range r.Refused {
		for _, n := range ref.Needs {
			set[n] = struct{}{}
		}
	}
	res := make([]string, 0, len(set))
	for n := range set {
		res = append(res, n)
	}
	sort.Strings(res)
	return res
}

// Actions returns the closed list of actions supported by the planner.
func Actions() []string {
	return []string{
		ActionCreateRole,
		ActionSetRolePermissions,
		ActionSetRoleOrder,
		ActionCreateCategory,
		ActionCreateChannel,
		ActionMoveChannel,
		ActionSetAccess,
		ActionConvertChannel,
		ActionResolveName,
	}
}

type candidate struct {
	action       string
	object       string
	needs        []string
	detail       string
	roleID       string
	channelID    string
	isNotVisible bool
}

type planContext struct {
	def           *serverdef.Definition
	snap          *Snapshot
	raised        bool
	botRoleID     string
	callerIdx     int
	callerPerms   map[string]struct{}
	hasAdmin      bool
	diffs         []Difference
	ambiguous     map[string]struct{}
	acceptedRoles []string
	pOrder        []string
	qOrder        []string
	changes       []Change
	refused       []Refusal
}

// Raised reports whether the caller has more permissions than the definition grants the bot role.
func Raised(def *serverdef.Definition, s *Snapshot) bool {
	var botRole *serverdef.Role
	for i := range def.Roles {
		if def.Roles[i].Kind == "bot" {
			botRole = &def.Roles[i]
			break
		}
	}
	if botRole == nil {
		return false
	}
	defPerms := make(map[string]struct{}, len(botRole.Permissions))
	for _, p := range botRole.Permissions {
		defPerms[p] = struct{}{}
	}
	for i := range s.Roles {
		if s.Roles[i].Name == s.Caller.Role {
			for _, p := range s.Roles[i].Permissions {
				if _, ok := defPerms[p]; !ok {
					return true
				}
			}
			break
		}
	}
	return false
}

// Plan constructs an ordered plan of changes and refusals based on definition and snapshot.
func Plan(def *serverdef.Definition, s *Snapshot) (Result, error) {
	diffs, err := Compare(def, s)
	if err != nil {
		return Result{}, err
	}

	ctx := newPlanContext(def, s, diffs)
	ctx.generateAndEvaluateCandidates()

	return Result{
		Raised:  ctx.raised,
		Changes: ctx.changes,
		Refused: ctx.refused,
	}, nil
}

func newPlanContext(def *serverdef.Definition, s *Snapshot, diffs []Difference) *planContext {
	botID := findBotRoleID(def)
	callerIdx := -1
	for i := range s.Roles {
		if s.Roles[i].Name == s.Caller.Role {
			callerIdx = i
			break
		}
	}
	callerPerms := make(map[string]struct{}, len(s.Caller.Permissions))
	for _, p := range s.Caller.Permissions {
		callerPerms[p] = struct{}{}
	}
	_, hasAdmin := callerPerms["ADMINISTRATOR"]

	ambiguous := make(map[string]struct{}, len(diffs))
	for _, d := range diffs {
		if d.Kind == KindRoleAmbiguous || d.Kind == KindCategoryAmbiguous || d.Kind == KindChannelAmbiguous {
			ambiguous[d.Object] = struct{}{}
		}
	}

	p, q := computeRoleOrderPQ(def, s, botID)

	return &planContext{
		def:         def,
		snap:        s,
		raised:      Raised(def, s),
		botRoleID:   botID,
		callerIdx:   callerIdx,
		callerPerms: callerPerms,
		hasAdmin:    hasAdmin,
		diffs:       diffs,
		ambiguous:   ambiguous,
		pOrder:      p,
		qOrder:      q,
	}
}

func computeRoleOrderPQ(def *serverdef.Definition, s *Snapshot, botRoleID string) ([]string, []string) {
	matchedMap := getMatchedDefRoleMap(def, s)
	var p []string
	pSet := make(map[string]struct{}, len(s.Roles))
	for i := range s.Roles {
		sr := &s.Roles[i]
		if sr.Everyone || sr.Name == s.Caller.Role {
			continue
		}
		if id, ok := matchedMap[sr.Name]; ok && id != botRoleID {
			p = append(p, id)
			pSet[id] = struct{}{}
		}
	}

	var q []string
	for _, id := range def.RoleOrder {
		if id != botRoleID {
			if _, ok := pSet[id]; ok {
				q = append(q, id)
			}
		}
	}
	return p, q
}

func (ctx *planContext) generateAndEvaluateCandidates() {
	ctx.addRoleMissingCandidates()
	ctx.addRolePermissionsCandidates()
	ctx.addRoleOrderCandidate()
	ctx.addCategoryMissingCandidates()
	ctx.addChannelCandidates()
	ctx.addMoveChannelCandidates()
	ctx.addAccessCandidates()
	ctx.addConvertAndResolveCandidates()
}

func (ctx *planContext) evaluateCandidate(c candidate) {
	if reason, detail := ctx.checkRefusal(c); reason != "" {
		ctx.refused = append(ctx.refused, Refusal{
			Action: c.action,
			Object: c.object,
			Needs:  c.needs,
			Reason: reason,
			Detail: detail,
		})
		return
	}

	step := len(ctx.changes) + 1
	ctx.changes = append(ctx.changes, Change{
		Step:   step,
		Action: c.action,
		Object: c.object,
		Needs:  c.needs,
		Detail: c.detail,
	})

	if c.action == ActionCreateRole && c.roleID != "" {
		ctx.acceptedRoles = append(ctx.acceptedRoles, c.roleID)
	}
}

func (ctx *planContext) checkRefusal(c candidate) (string, string) {
	if c.action == ActionResolveName || ctx.namesAmbiguous(c) {
		return ReasonAmbiguousName, ""
	}
	if c.action == ActionConvertChannel {
		return ReasonTypeDiffers, ""
	}
	if c.isNotVisible {
		return ReasonNotVisible, ""
	}
	if !ctx.raised && !ctx.isAllowedInOwnMode(c) {
		return ReasonOutsideOwnChanges, ""
	}
	if r, d := ctx.checkMissingFeature(c); r != "" {
		return r, d
	}
	if r, d := ctx.checkManagedRole(c); r != "" {
		return r, d
	}
	if r, d := ctx.checkUnknownBits(c); r != "" {
		return r, d
	}
	if r, d := ctx.checkAboveCaller(c); r != "" {
		return r, d
	}
	if r, d := ctx.checkLacksRight(c); r != "" {
		return r, d
	}
	return "", ""
}

func (ctx *planContext) defChannel(id string) *serverdef.Channel {
	for i := range ctx.def.Channels {
		if ctx.def.Channels[i].ID == id {
			return &ctx.def.Channels[i]
		}
	}
	return nil
}

func (ctx *planContext) isAmbiguous(object string) bool {
	_, ok := ctx.ambiguous[object]
	return ok
}

// namesAmbiguous reports whether a candidate rests on a name the server holds
// more than once: its own role or channel, the category a channel is created in
// or moved to, or a role named by an entry of a channel that is created.
func (ctx *planContext) namesAmbiguous(c candidate) bool {
	if c.roleID != "" && ctx.isAmbiguous(fmt.Sprintf("roles[%s]", c.roleID)) {
		return true
	}
	ch := ctx.defChannel(c.channelID)
	if ch == nil {
		return false
	}
	if ctx.isAmbiguous(fmt.Sprintf("channels[%s]", ch.ID)) {
		return true
	}
	if c.action != ActionCreateChannel && c.action != ActionMoveChannel {
		return false
	}
	if ctx.isAmbiguous(fmt.Sprintf("categories[%s]", ch.Category)) {
		return true
	}
	return c.action == ActionCreateChannel && ctx.entryRoleAmbiguous(ch)
}

func (ctx *planContext) entryRoleAmbiguous(ch *serverdef.Channel) bool {
	for i := range ch.Access {
		if ctx.isAmbiguous(fmt.Sprintf("roles[%s]", ch.Access[i].Role)) {
			return true
		}
	}
	return false
}

func (ctx *planContext) isAllowedInOwnMode(c candidate) bool {
	if c.action != ActionSetAccess {
		return false
	}
	ch := ctx.defChannel(c.channelID)
	if ch == nil || ch.Area == "" {
		return false
	}
	for i := range ctx.def.Areas {
		if ctx.def.Areas[i].ID == ch.Area {
			return ctx.def.Areas[i].Role == c.roleID
		}
	}
	return false
}

func (ctx *planContext) checkMissingFeature(c candidate) (string, string) {
	if c.action != ActionCreateChannel {
		return "", ""
	}
	ch := ctx.defChannel(c.channelID)
	if ch != nil && ch.Type == "announcement" {
		hasNews := false
		for _, f := range ctx.snap.Features {
			if f == "NEWS" {
				hasNews = true
				break
			}
		}
		if !hasNews {
			return ReasonMissingFeature, ""
		}
	}
	return "", ""
}

func (ctx *planContext) checkManagedRole(c candidate) (string, string) {
	if c.action != ActionSetRolePermissions || c.roleID == "" {
		return "", ""
	}
	for i := range ctx.snap.Roles {
		sr := &ctx.snap.Roles[i]
		if sr.Name == getRoleName(ctx.def, c.roleID) && sr.Managed {
			return ReasonManagedRole, ""
		}
	}
	return "", ""
}

// checkUnknownBits refuses an entry change that has bits set the permission table
// does not name (entries only; a role-permission change clears such bits and names them).
func (ctx *planContext) checkUnknownBits(c candidate) (string, string) {
	if c.action != ActionSetAccess || c.roleID == "" {
		return "", ""
	}
	sr := ctx.snapRole(c.roleID)
	if sr == nil {
		return "", ""
	}
	ch := ctx.defChannel(c.channelID)
	if ch == nil {
		return "", ""
	}
	if detail := entryBitsDetail(ctx.snap, ch.Name, sr.Name); detail != "" {
		return ReasonUnknownBits, detail
	}
	return "", ""
}

// roleClearsUnlistedBitsDetail returns a detail note when the matched server role
// has unlisted bits. The change sends the definition's whole value; the plan text
// and so the plan digest name what is cleared.
func (ctx *planContext) roleClearsUnlistedBitsDetail(roleID string) string {
	sr := ctx.snapRole(roleID)
	if sr != nil && len(sr.UnknownBits) > 0 {
		return "clears unlisted bits " + bitList(sr.UnknownBits)
	}
	return ""
}

// entryBitsDetail returns the unknown bits of the first entry for the named
// role on a server channel of the given name that has any, or "" for none.
func entryBitsDetail(s *Snapshot, channelName, roleName string) string {
	for i := range s.Channels {
		sch := &s.Channels[i]
		if sch.Name != channelName {
			continue
		}
		for j := range sch.Access {
			if sch.Access[j].Role != roleName {
				continue
			}
			if detail := accessBitsDetail(&sch.Access[j]); detail != "" {
				return detail
			}
		}
	}
	return ""
}

// snapRole returns the one server role a role of the definition is matched to,
// or nil when there is none or more than one.
func (ctx *planContext) snapRole(roleID string) *Role {
	for i := range ctx.def.Roles {
		if ctx.def.Roles[i].ID != roleID {
			continue
		}
		matched := matchRoleInSnapshot(&ctx.def.Roles[i], ctx.snap)
		if len(matched) != 1 {
			return nil
		}
		return &matched[0]
	}
	return nil
}

func getRoleName(def *serverdef.Definition, roleID string) string {
	for i := range def.Roles {
		if def.Roles[i].ID == roleID {
			return def.Roles[i].Name
		}
	}
	return ""
}

func (ctx *planContext) checkAboveCaller(c candidate) (string, string) {
	if c.action == ActionSetRolePermissions && c.roleID != "" {
		return checkRoleAboveCaller(ctx, c.roleID)
	}
	if c.action == ActionSetRoleOrder {
		return checkRoleOrderAboveCaller(ctx)
	}
	return "", ""
}

func checkRoleAboveCaller(ctx *planContext, roleID string) (string, string) {
	roleName := getRoleName(ctx.def, roleID)
	for i := range ctx.snap.Roles {
		if ctx.snap.Roles[i].Name == roleName {
			if i < ctx.callerIdx {
				return ReasonAboveCaller, ""
			}
			break
		}
	}
	return "", ""
}

func checkRoleOrderAboveCaller(ctx *planContext) (string, string) {
	if ctx.callerIdx <= 0 {
		return "", ""
	}
	var pAbove []string
	matchedMap := getMatchedDefRoleMap(ctx.def, ctx.snap)
	for i := 0; i < ctx.callerIdx; i++ {
		sr := &ctx.snap.Roles[i]
		if sr.Everyone || sr.Name == ctx.snap.Caller.Role {
			continue
		}
		if id, ok := matchedMap[sr.Name]; ok && id != ctx.botRoleID {
			pAbove = append(pAbove, id)
		}
	}
	// The order the change would set, created roles included: a created role
	// cannot be placed above a role that stands above the caller.
	target := ctx.buildRoleOrderDetail()
	k := len(pAbove)
	if len(target) < k || !slices.Equal(target[:k], pAbove) {
		return ReasonAboveCaller, ""
	}
	return "", ""
}

func (ctx *planContext) checkLacksRight(c candidate) (string, string) {
	if ctx.hasAdmin || len(c.needs) == 0 {
		return "", ""
	}
	var lacking []string
	for _, n := range c.needs {
		if _, ok := ctx.callerPerms[n]; !ok {
			lacking = append(lacking, n)
		}
	}
	if len(lacking) > 0 {
		sort.Strings(lacking)
		return ReasonLacksRight, strings.Join(lacking, ", ")
	}
	return "", ""
}

func sortedUnique(items ...string) []string {
	set := make(map[string]struct{}, len(items))
	for _, it := range items {
		if it != "" {
			set[it] = struct{}{}
		}
	}
	res := make([]string, 0, len(set))
	for it := range set {
		res = append(res, it)
	}
	sort.Strings(res)
	return res
}

func (ctx *planContext) addRoleMissingCandidates() {
	missingSet := make(map[string]struct{})
	for _, d := range ctx.diffs {
		if d.Kind == KindRoleMissing {
			id := extractBracketID(d.Object)
			missingSet[id] = struct{}{}
		}
	}

	for _, id := range ctx.def.RoleOrder {
		if _, ok := missingSet[id]; ok {
			var defRole *serverdef.Role
			for i := range ctx.def.Roles {
				if ctx.def.Roles[i].ID == id {
					defRole = &ctx.def.Roles[i]
					break
				}
			}
			needs := sortedUnique(append([]string{"MANAGE_ROLES"}, defRole.Permissions...)...)
			ctx.evaluateCandidate(candidate{
				action: ActionCreateRole,
				object: fmt.Sprintf("roles[%s]", id),
				needs:  needs,
				roleID: id,
			})
		}
	}
}

func (ctx *planContext) addRolePermissionsCandidates() {
	for _, d := range ctx.diffs {
		if d.Kind == KindRolePermissions {
			id := extractBracketID(d.Object)
			if id == ctx.botRoleID {
				continue
			}
			var defRole *serverdef.Role
			for i := range ctx.def.Roles {
				if ctx.def.Roles[i].ID == id {
					defRole = &ctx.def.Roles[i]
					break
				}
			}
			lacks := findLackingPerms(defRole, ctx.snap)
			needs := sortedUnique(append([]string{"MANAGE_ROLES"}, lacks...)...)
			ctx.evaluateCandidate(candidate{
				action: ActionSetRolePermissions,
				object: d.Object,
				needs:  needs,
				detail: ctx.roleClearsUnlistedBitsDetail(id),
				roleID: id,
			})
		}
	}
}

func findLackingPerms(defRole *serverdef.Role, s *Snapshot) []string {
	matched := matchRoleInSnapshot(defRole, s)
	if len(matched) != 1 {
		return nil
	}
	snapPerms := make(map[string]struct{}, len(matched[0].Permissions))
	for _, p := range matched[0].Permissions {
		snapPerms[p] = struct{}{}
	}
	var lacks []string
	for _, p := range defRole.Permissions {
		if _, ok := snapPerms[p]; !ok {
			lacks = append(lacks, p)
		}
	}
	return lacks
}

func (ctx *planContext) addRoleOrderCandidate() {
	wanted := len(ctx.acceptedRoles) > 0 || !slices.Equal(ctx.pOrder, ctx.qOrder)
	if !wanted {
		return
	}

	detailIDs := ctx.buildRoleOrderDetail()
	ctx.evaluateCandidate(candidate{
		action: ActionSetRoleOrder,
		object: "role_order",
		needs:  []string{"MANAGE_ROLES"},
		detail: strings.Join(detailIDs, ", "),
	})
}

func (ctx *planContext) buildRoleOrderDetail() []string {
	allowedSet := make(map[string]struct{}, len(ctx.pOrder)+len(ctx.acceptedRoles))
	for _, id := range ctx.pOrder {
		allowedSet[id] = struct{}{}
	}
	for _, id := range ctx.acceptedRoles {
		allowedSet[id] = struct{}{}
	}
	var detailIDs []string
	for _, id := range ctx.def.RoleOrder {
		if id != ctx.botRoleID {
			if _, ok := allowedSet[id]; ok {
				detailIDs = append(detailIDs, id)
			}
		}
	}
	return detailIDs
}

func (ctx *planContext) addCategoryMissingCandidates() {
	for _, d := range ctx.diffs {
		if d.Kind == KindCategoryMissing {
			ctx.evaluateCandidate(candidate{
				action: ActionCreateCategory,
				object: d.Object,
				needs:  []string{"MANAGE_CHANNELS"},
			})
		}
	}
}

func (ctx *planContext) addChannelCandidates() {
	for _, d := range ctx.diffs {
		if d.Kind == KindChannelMissing || (d.Kind == KindChannelNotVisible && d.Detail == "missing or hidden") {
			chID := extractBracketID(d.Object)
			ch := ctx.defChannel(chID)
			allPerms := []string{"MANAGE_CHANNELS"}
			for _, acc := range ch.Access {
				allPerms = append(allPerms, acc.Allow...)
				allPerms = append(allPerms, acc.Deny...)
			}
			ctx.evaluateCandidate(candidate{
				action:       ActionCreateChannel,
				object:       fmt.Sprintf("channels[%s]", chID),
				needs:        sortedUnique(allPerms...),
				channelID:    chID,
				isNotVisible: d.Kind == KindChannelNotVisible,
			})
		}
	}
}

func (ctx *planContext) addMoveChannelCandidates() {
	for _, d := range ctx.diffs {
		if d.Kind == KindChannelCategory {
			chID := extractBracketID(d.Object)
			ctx.evaluateCandidate(candidate{
				action:    ActionMoveChannel,
				object:    fmt.Sprintf("channels[%s]", chID),
				needs:     []string{"MANAGE_CHANNELS"},
				channelID: chID,
			})
		}
	}
}

func findDefChannelAccess(def *serverdef.Definition, chID, roleID string) *serverdef.Access {
	for i := range def.Channels {
		ch := &def.Channels[i]
		if ch.ID == chID {
			for j := range ch.Access {
				if ch.Access[j].Role == roleID {
					return &ch.Access[j]
				}
			}
			break
		}
	}
	return nil
}

func candidateForAccessDiff(def *serverdef.Definition, d Difference) candidate {
	chID, roleID := parseAccessObject(d.Object)
	acc := findDefChannelAccess(def, chID, roleID)
	allPerms := []string{"MANAGE_ROLES"}
	if acc != nil {
		allPerms = append(allPerms, acc.Allow...)
		allPerms = append(allPerms, acc.Deny...)
	}
	return candidate{
		action:    ActionSetAccess,
		object:    d.Object,
		needs:     sortedUnique(allPerms...),
		channelID: chID,
		roleID:    roleID,
	}
}

func (ctx *planContext) addAccessCandidates() {
	for _, d := range ctx.diffs {
		if d.Kind == KindAccessMissing || d.Kind == KindAccessDiffers {
			ctx.evaluateCandidate(candidateForAccessDiff(ctx.def, d))
		}
	}
}

func (ctx *planContext) addConvertAndResolveCandidates() {
	for _, d := range ctx.diffs {
		if d.Kind == KindChannelType {
			chID := extractBracketID(d.Object)
			ctx.evaluateCandidate(candidate{
				action:    ActionConvertChannel,
				object:    d.Object,
				channelID: chID,
			})
		}
		if d.Kind == KindRoleAmbiguous || d.Kind == KindCategoryAmbiguous || d.Kind == KindChannelAmbiguous {
			ctx.evaluateCandidate(candidate{
				action: ActionResolveName,
				object: d.Object,
			})
		}
	}
}

func extractBracketID(s string) string {
	start := strings.Index(s, "[")
	end := strings.Index(s, "]")
	if start != -1 && end != -1 && end > start+1 {
		return s[start+1 : end]
	}
	return ""
}

func parseAccessObject(s string) (string, string) {
	// e.g. channels[home-lab].access[interest-home-lab]
	chStart := strings.Index(s, "channels[")
	chEnd := strings.Index(s, "].access[")
	accEnd := strings.LastIndex(s, "]")
	if chStart != -1 && chEnd != -1 && accEnd > chEnd+9 {
		chID := s[chStart+9 : chEnd]
		roleID := s[chEnd+9 : accEnd]
		return chID, roleID
	}
	return "", ""
}
