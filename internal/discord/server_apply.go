package discord

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
)

// ApplyInput holds what a run applies and the digest of the plan a person has read.
type ApplyInput struct {
	Definition       *serverdef.Definition
	DefinitionDigest string
	PlanDigest       string
}

// ApplyServer applies the planned server changes to the Discord guild using the bot's credentials.
func ApplyServer(ctx context.Context, token string, guildID uint64, in ApplyInput, out io.Writer) error {
	return applyServerWithOptions(ctx, token, guildID, in, out, clientOptions{})
}

func applyServerWithOptions(
	ctx context.Context,
	token string,
	guildID uint64,
	in ApplyInput,
	out io.Writer,
	opts clientOptions,
) error {
	if err := checkTokenAndGuild(token, guildID); err != nil {
		return err
	}
	if in.Definition == nil || in.DefinitionDigest == "" || in.PlanDigest == "" {
		return errors.New("definition, definition digest and plan digest are required")
	}

	client, err := newRestClient(token, opts.restURL)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	snap, idx, err := fetchSnapshotAndIndex(ctx, client, token, guildID, opts)
	if err != nil {
		return err
	}

	result, err := serverplan.Plan(in.Definition, snap)
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}

	if err := verifyApplyGates(result, in); err != nil {
		return err
	}

	if len(result.Changes) == 0 {
		if _, err := fmt.Fprintf(out, "changes: 0\napplied: 0 of 0\n"); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		return nil
	}

	return executePlan(ctx, client, guildID, in, result.Changes, idx, out, opts)
}

func verifyApplyGates(result serverplan.Result, in ApplyInput) error {
	gotDigest, err := serverplan.Digest(result, in.DefinitionDigest)
	if err != nil {
		return fmt.Errorf("digest: %w", err)
	}
	if gotDigest != in.PlanDigest {
		return fmt.Errorf("the plan is not the one that was shown: its digest is now %s; run -server=plan and read it again", gotDigest)
	}

	if len(result.Refused) > 0 {
		return fmt.Errorf("the plan holds %d refused changes; a plan with a refusal is not applied", len(result.Refused))
	}

	for _, c := range result.Changes {
		if err := validateChangeObject(in.Definition, c); err != nil {
			return err
		}
	}
	return nil
}

func validateChangeObject(def *serverdef.Definition, c serverplan.Change) error {
	switch c.Action {
	case "create-role", "set-role-permissions", "set-role-display":
		return validateRoleChange(def, c)
	case "set-role-order":
		return validateRoleOrderChange(c)
	case "create-category":
		return validateCategoryChange(def, c)
	case "create-channel", "move-channel":
		return validateChannelChange(def, c)
	case "set-access":
		return validateAccessChange(def, c)
	case serverplan.ActionSetVerificationLevel:
		return validateVerificationChange(def, c)
	case serverplan.ActionCreateFilterRule, serverplan.ActionSetFilterRule:
		return validateFilterRuleChange(def, c)
	default:
		return fmt.Errorf("step %d %s %s: unsupported action", c.Step, c.Action, c.Object)
	}
}

func validateRoleChange(def *serverdef.Definition, c serverplan.Change) error {
	id, err := extractBracketedID(c.Object, "roles[")
	if err != nil {
		return fmt.Errorf("step %d %s %s: %w", c.Step, c.Action, c.Object, err)
	}
	if _, ok := findRoleInDef(def, id); !ok {
		return fmt.Errorf("step %d %s %s: role %q not found in definition", c.Step, c.Action, c.Object, id)
	}
	return nil
}

func validateRoleOrderChange(c serverplan.Change) error {
	if c.Object != "role_order" {
		return fmt.Errorf("step %d %s %s: unexpected object for set-role-order", c.Step, c.Action, c.Object)
	}
	return nil
}

func validateCategoryChange(def *serverdef.Definition, c serverplan.Change) error {
	id, err := extractBracketedID(c.Object, "categories[")
	if err != nil {
		return fmt.Errorf("step %d %s %s: %w", c.Step, c.Action, c.Object, err)
	}
	if _, ok := findCategoryInDef(def, id); !ok {
		return fmt.Errorf("step %d %s %s: category %q not found in definition", c.Step, c.Action, c.Object, id)
	}
	return nil
}

func validateChannelChange(def *serverdef.Definition, c serverplan.Change) error {
	id, err := extractBracketedID(c.Object, "channels[")
	if err != nil {
		return fmt.Errorf("step %d %s %s: %w", c.Step, c.Action, c.Object, err)
	}
	if _, ok := findChannelInDef(def, id); !ok {
		return fmt.Errorf("step %d %s %s: channel %q not found in definition", c.Step, c.Action, c.Object, id)
	}
	return nil
}

func validateAccessChange(def *serverdef.Definition, c serverplan.Change) error {
	chID, roleID, err := extractAccessObject(c.Object)
	if err != nil {
		return fmt.Errorf("step %d %s %s: %w", c.Step, c.Action, c.Object, err)
	}
	ch, ok := findChannelInDef(def, chID)
	if !ok {
		return fmt.Errorf("step %d %s %s: channel %q not found in definition", c.Step, c.Action, c.Object, chID)
	}
	if _, ok := findAccessInDef(ch, roleID); !ok {
		return fmt.Errorf("step %d %s %s: access for role %q not found in definition", c.Step, c.Action, c.Object, roleID)
	}
	return nil
}

func extractBracketedID(obj, prefix string) (string, error) {
	if !strings.HasPrefix(obj, prefix) || !strings.HasSuffix(obj, "]") {
		return "", fmt.Errorf("invalid object %q: expected prefix %q and suffix ']'", obj, prefix)
	}
	id := obj[len(prefix) : len(obj)-1]
	if id == "" {
		return "", fmt.Errorf("empty identifier in %q", obj)
	}
	return id, nil
}

func extractAccessObject(obj string) (string, string, error) {
	const prefix = "channels["
	const mid = "].access["
	if !strings.HasPrefix(obj, prefix) || !strings.HasSuffix(obj, "]") || !strings.Contains(obj, mid) {
		return "", "", fmt.Errorf("invalid access object %q", obj)
	}
	rem := obj[len(prefix):]
	parts := strings.SplitN(rem, mid, 2)
	if len(parts) != 2 || !strings.HasSuffix(parts[1], "]") {
		return "", "", fmt.Errorf("invalid access object %q", obj)
	}
	chID := parts[0]
	roleID := strings.TrimSuffix(parts[1], "]")
	if chID == "" || roleID == "" {
		return "", "", fmt.Errorf("empty channel or role in %q", obj)
	}
	return chID, roleID, nil
}

func findRoleInDef(def *serverdef.Definition, id string) (*serverdef.Role, bool) {
	for i := range def.Roles {
		if def.Roles[i].ID == id {
			return &def.Roles[i], true
		}
	}
	return nil, false
}

func findCategoryInDef(def *serverdef.Definition, id string) (*serverdef.Category, bool) {
	for i := range def.Categories {
		if def.Categories[i].ID == id {
			return &def.Categories[i], true
		}
	}
	return nil, false
}

func findChannelInDef(def *serverdef.Definition, id string) (*serverdef.Channel, bool) {
	for i := range def.Channels {
		if def.Channels[i].ID == id {
			return &def.Channels[i], true
		}
	}
	return nil, false
}

func findAccessInDef(ch *serverdef.Channel, roleID string) (*serverdef.Access, bool) {
	for i := range ch.Access {
		if ch.Access[i].Role == roleID {
			return &ch.Access[i], true
		}
	}
	return nil, false
}

func resolveRoleID(r *serverdef.Role, idx *serverIndex) (snowflake.ID, error) {
	if r.Kind == "everyone" {
		return idx.everyoneRoleID, nil
	}
	if r.Kind == "bot" {
		return idx.botRoleID, nil
	}
	return idx.lookupRole(r.Name)
}

func executePlan(
	ctx context.Context,
	client *bot.Client,
	guildID uint64,
	in ApplyInput,
	changes []serverplan.Change,
	idx *serverIndex,
	out io.Writer,
	opts clientOptions,
) error {
	if _, err := fmt.Fprintf(out, "changes: %d\n", len(changes)); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	applied := 0
	timeout := opts.getApplyCallTimeout()

	for _, c := range changes {
		if err := ctx.Err(); err != nil {
			if _, wErr := fmt.Fprintf(out, "applied: %d of %d\n", applied, len(changes)); wErr != nil {
				return fmt.Errorf("write output: %w", wErr)
			}
			return err
		}

		err := executeSingleChange(ctx, client, guildID, in, c, idx, timeout)
		if err != nil {
			if _, wErr := fmt.Fprintf(out, "  %d. %s %s: failed\napplied: %d of %d\n", c.Step, c.Action, c.Object, applied, len(changes)); wErr != nil {
				return fmt.Errorf("write output: %w", wErr)
			}
			return fmt.Errorf("step %d %s %s: %w", c.Step, c.Action, c.Object, err)
		}

		applied++
		if _, err := fmt.Fprintf(out, "  %d. %s %s: done\n", c.Step, c.Action, c.Object); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	}

	if _, err := fmt.Fprintf(out, "applied: %d of %d\n", applied, len(changes)); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func executeSingleChange(
	ctx context.Context,
	client *bot.Client,
	guildID uint64,
	in ApplyInput,
	c serverplan.Change,
	idx *serverIndex,
	timeout time.Duration,
) error {
	auditReason := "diallak apply " + in.PlanDigest
	if c.Action == "set-role-order" {
		// two requests, each under its own deadline
		return applySetRoleOrder(ctx, client, guildID, in.Definition, c, idx, auditReason, timeout)
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch c.Action {
	case "create-role":
		return applyCreateRole(callCtx, client, guildID, in.Definition, c.Object, idx, auditReason)
	case "set-role-permissions":
		return applySetRolePermissions(callCtx, client, guildID, in.Definition, c.Object, idx, auditReason)
	case "set-role-display":
		return applySetRoleDisplay(callCtx, client, guildID, in.Definition, c.Object, idx, auditReason)
	case "create-category":
		return applyCreateCategory(callCtx, client, guildID, in.Definition, c.Object, idx, auditReason)
	case "create-channel":
		return applyCreateChannel(callCtx, client, guildID, in.Definition, c.Object, idx, auditReason)
	case "move-channel":
		return applyMoveChannel(callCtx, client, in.Definition, c.Object, idx, auditReason)
	case "set-access":
		return applySetAccess(callCtx, client, in.Definition, c.Object, idx, auditReason)
	case serverplan.ActionSetVerificationLevel:
		return applySetVerificationLevel(callCtx, client, guildID, in.Definition, auditReason)
	case serverplan.ActionCreateFilterRule:
		return applyCreateFilterRule(callCtx, client, guildID, in.Definition, c.Object, idx, auditReason)
	case serverplan.ActionSetFilterRule:
		return applySetFilterRule(callCtx, client, guildID, in.Definition, c.Object, idx, auditReason)
	default:
		return fmt.Errorf("unsupported action %q", c.Action)
	}
}

func applyCreateRole(
	ctx context.Context,
	client *bot.Client,
	guildID uint64,
	def *serverdef.Definition,
	object string,
	idx *serverIndex,
	reason string,
) error {
	id, err := extractBracketedID(object, "roles[")
	if err != nil {
		return err
	}
	defRole, ok := findRoleInDef(def, id)
	if !ok {
		return fmt.Errorf("role %q not found in definition", id)
	}
	if defRole.Kind == "managed" {
		return fmt.Errorf("role %q is managed by another party; the tool does not change it", id)
	}

	mask, err := serverdef.PermissionMask(defRole.Permissions)
	if err != nil {
		return fmt.Errorf("role %q permissions: %w", id, err)
	}
	perms := maskToPermissions(mask)
	roleCreate := discord.RoleCreate{
		Name:        defRole.Name,
		Permissions: &perms,
		Hoist:       defRole.ShownApart != nil && *defRole.ShownApart,
		Mentionable: defRole.Mentionable != nil && *defRole.Mentionable,
	}

	created, err := client.Rest.CreateRole(snowflake.ID(guildID), roleCreate, rest.WithCtx(ctx), rest.WithReason(reason))
	if err != nil {
		return callError("create role", err, client.Token)
	}
	if created == nil || created.ID == 0 {
		return errors.New("create role: returned role id is 0")
	}
	idx.addRole(defRole.Name, created.ID)
	return nil
}

func applySetRolePermissions(
	ctx context.Context,
	client *bot.Client,
	guildID uint64,
	def *serverdef.Definition,
	object string,
	idx *serverIndex,
	reason string,
) error {
	id, err := extractBracketedID(object, "roles[")
	if err != nil {
		return err
	}
	defRole, ok := findRoleInDef(def, id)
	if !ok {
		return fmt.Errorf("role %q not found in definition", id)
	}
	if defRole.Kind == "managed" {
		return fmt.Errorf("role %q is managed by another party; the tool does not change it", id)
	}

	roleID, err := resolveRoleID(defRole, idx)
	if err != nil {
		return err
	}

	mask, err := serverdef.PermissionMask(defRole.Permissions)
	if err != nil {
		return fmt.Errorf("role %q permissions: %w", id, err)
	}
	perms := maskToPermissions(mask)
	roleUpdate := discord.RoleUpdate{
		Permissions: &perms,
	}

	_, err = client.Rest.UpdateRole(snowflake.ID(guildID), roleID, roleUpdate, rest.WithCtx(ctx), rest.WithReason(reason))
	if err != nil {
		return callError("set role permissions", err, client.Token)
	}
	return nil
}

func applySetRoleDisplay(
	ctx context.Context,
	client *bot.Client,
	guildID uint64,
	def *serverdef.Definition,
	object string,
	idx *serverIndex,
	reason string,
) error {
	id, err := extractBracketedID(object, "roles[")
	if err != nil {
		return err
	}
	defRole, ok := findRoleInDef(def, id)
	if !ok {
		return fmt.Errorf("role %q not found in definition", id)
	}
	if defRole.Kind == "managed" {
		return fmt.Errorf("role %q is managed by another party; the tool does not change it", id)
	}

	roleID, err := resolveRoleID(defRole, idx)
	if err != nil {
		return err
	}

	roleUpdate := discord.RoleUpdate{
		Hoist:       defRole.ShownApart,
		Mentionable: defRole.Mentionable,
	}

	_, err = client.Rest.UpdateRole(snowflake.ID(guildID), roleID, roleUpdate, rest.WithCtx(ctx), rest.WithReason(reason))
	if err != nil {
		return callError("set role display", err, client.Token)
	}
	return nil
}

func applyCreateCategory(
	ctx context.Context,
	client *bot.Client,
	guildID uint64,
	def *serverdef.Definition,
	object string,
	idx *serverIndex,
	reason string,
) error {
	id, err := extractBracketedID(object, "categories[")
	if err != nil {
		return err
	}
	defCat, ok := findCategoryInDef(def, id)
	if !ok {
		return fmt.Errorf("category %q not found in definition", id)
	}

	catCreate := discord.GuildCategoryChannelCreate{
		Name: defCat.Name,
	}
	created, err := client.Rest.CreateGuildChannel(snowflake.ID(guildID), catCreate, rest.WithCtx(ctx), rest.WithReason(reason))
	if err != nil {
		return callError("create category", err, client.Token)
	}
	if created == nil || created.ID() == 0 {
		return errors.New("create category: returned category id is 0")
	}
	idx.addCategory(defCat.Name, created.ID())
	return nil
}

func applyCreateChannel(
	ctx context.Context,
	client *bot.Client,
	guildID uint64,
	def *serverdef.Definition,
	object string,
	idx *serverIndex,
	reason string,
) error {
	id, err := extractBracketedID(object, "channels[")
	if err != nil {
		return err
	}
	defCh, ok := findChannelInDef(def, id)
	if !ok {
		return fmt.Errorf("channel %q not found in definition", id)
	}

	var parentID snowflake.ID
	if defCh.Category != "" {
		catDef, ok := findCategoryInDef(def, defCh.Category)
		if !ok {
			return fmt.Errorf("category %q not found in definition", defCh.Category)
		}
		catID, err := idx.lookupCategory(catDef.Name)
		if err != nil {
			return err
		}
		parentID = catID
	}

	overwrites, err := buildPermissionOverwrites(def, defCh.Access, idx)
	if err != nil {
		return err
	}

	chCreate, err := buildGuildChannelCreate(defCh, parentID, overwrites)
	if err != nil {
		return err
	}

	created, err := client.Rest.CreateGuildChannel(snowflake.ID(guildID), chCreate, rest.WithCtx(ctx), rest.WithReason(reason))
	if err != nil {
		return callError("create channel", err, client.Token)
	}
	if created == nil || created.ID() == 0 {
		return errors.New("create channel: returned channel id is 0")
	}
	idx.addChannel(defCh.Name, created.ID())
	return nil
}

func buildPermissionOverwrites(
	def *serverdef.Definition,
	accessList []serverdef.Access,
	idx *serverIndex,
) ([]discord.PermissionOverwrite, error) {
	res := make([]discord.PermissionOverwrite, 0, len(accessList))
	for _, a := range accessList {
		rDef, ok := findRoleInDef(def, a.Role)
		if !ok {
			return nil, fmt.Errorf("role %q not found in definition", a.Role)
		}
		rID, err := resolveRoleID(rDef, idx)
		if err != nil {
			return nil, err
		}
		allowMask, err := serverdef.PermissionMask(a.Allow)
		if err != nil {
			return nil, fmt.Errorf("role %q allow: %w", a.Role, err)
		}
		denyMask, err := serverdef.PermissionMask(a.Deny)
		if err != nil {
			return nil, fmt.Errorf("role %q deny: %w", a.Role, err)
		}
		res = append(res, discord.RolePermissionOverwrite{
			RoleID: rID,
			Allow:  maskToPermissions(allowMask),
			Deny:   maskToPermissions(denyMask),
		})
	}
	return res, nil
}

func buildGuildChannelCreate(
	defCh *serverdef.Channel,
	parentID snowflake.ID,
	overwrites []discord.PermissionOverwrite,
) (discord.GuildChannelCreate, error) {
	switch defCh.Type {
	case "text":
		return discord.GuildTextChannelCreate{
			Name:                 defCh.Name,
			ParentID:             parentID,
			PermissionOverwrites: overwrites,
		}, nil
	case "voice":
		return discord.GuildVoiceChannelCreate{
			Name:                 defCh.Name,
			ParentID:             parentID,
			PermissionOverwrites: overwrites,
		}, nil
	case "announcement":
		return discord.GuildNewsChannelCreate{
			Name:                 defCh.Name,
			ParentID:             parentID,
			PermissionOverwrites: overwrites,
		}, nil
	case "forum":
		return discord.GuildForumChannelCreate{
			Name:                 defCh.Name,
			ParentID:             parentID,
			PermissionOverwrites: overwrites,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported channel type %q", defCh.Type)
	}
}

func applyMoveChannel(
	ctx context.Context,
	client *bot.Client,
	def *serverdef.Definition,
	object string,
	idx *serverIndex,
	reason string,
) error {
	id, err := extractBracketedID(object, "channels[")
	if err != nil {
		return err
	}
	defCh, ok := findChannelInDef(def, id)
	if !ok {
		return fmt.Errorf("channel %q not found in definition", id)
	}

	chID, err := idx.lookupChannel(defCh.Name)
	if err != nil {
		return err
	}

	// Without a parent the request body would be empty and change nothing.
	if defCh.Category == "" {
		return fmt.Errorf("channel %q has no category in the definition", id)
	}
	catDef, ok := findCategoryInDef(def, defCh.Category)
	if !ok {
		return fmt.Errorf("category %q not found in definition", defCh.Category)
	}
	catID, err := idx.lookupCategory(catDef.Name)
	if err != nil {
		return err
	}

	update, err := buildGuildChannelUpdate(defCh.Type, &catID)
	if err != nil {
		return err
	}

	_, err = client.Rest.UpdateChannel(chID, update, rest.WithCtx(ctx), rest.WithReason(reason))
	if err != nil {
		return callError("move channel", err, client.Token)
	}
	return nil
}

func buildGuildChannelUpdate(chType string, parentID *snowflake.ID) (discord.ChannelUpdate, error) {
	switch chType {
	case "text":
		return discord.GuildTextChannelUpdate{ParentID: parentID}, nil
	case "voice":
		return discord.GuildVoiceChannelUpdate{ParentID: parentID}, nil
	case "announcement":
		return discord.GuildNewsChannelUpdate{ParentID: parentID}, nil
	case "forum":
		return discord.GuildForumChannelUpdate{ParentID: parentID}, nil
	default:
		return nil, fmt.Errorf("unsupported channel type %q", chType)
	}
}

func applySetAccess(
	ctx context.Context,
	client *bot.Client,
	def *serverdef.Definition,
	object string,
	idx *serverIndex,
	reason string,
) error {
	chID, roleID, err := extractAccessObject(object)
	if err != nil {
		return err
	}
	defCh, ok := findChannelInDef(def, chID)
	if !ok {
		return fmt.Errorf("channel %q not found in definition", chID)
	}
	defRole, ok := findRoleInDef(def, roleID)
	if !ok {
		return fmt.Errorf("role %q not found in definition", roleID)
	}
	access, ok := findAccessInDef(defCh, roleID)
	if !ok {
		return fmt.Errorf("access for role %q not found in definition", roleID)
	}

	channelSnowflakeID, err := idx.lookupChannel(defCh.Name)
	if err != nil {
		return err
	}
	roleSnowflakeID, err := resolveRoleID(defRole, idx)
	if err != nil {
		return err
	}

	allowMask, err := serverdef.PermissionMask(access.Allow)
	if err != nil {
		return fmt.Errorf("role %q allow: %w", roleID, err)
	}
	denyMask, err := serverdef.PermissionMask(access.Deny)
	if err != nil {
		return fmt.Errorf("role %q deny: %w", roleID, err)
	}

	allowPerm := maskToPermissions(allowMask)
	denyPerm := maskToPermissions(denyMask)
	owUpdate := discord.RolePermissionOverwriteUpdate{
		Allow: &allowPerm,
		Deny:  &denyPerm,
	}

	err = client.Rest.UpdatePermissionOverwrite(channelSnowflakeID, roleSnowflakeID, owUpdate, rest.WithCtx(ctx), rest.WithReason(reason))
	if err != nil {
		return callError("set access", err, client.Token)
	}
	return nil
}

func fetchAndSortRoles(
	ctx context.Context,
	client *bot.Client,
	guildID uint64,
	botRoleID snowflake.ID,
	timeout time.Duration,
) ([]discord.Role, int, error) {
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rawRoles, err := client.Rest.GetRoles(snowflake.ID(guildID), rest.WithCtx(readCtx))
	if err != nil {
		return nil, -1, callError("read roles", err, client.Token)
	}

	sortedRoles := make([]discord.Role, len(rawRoles))
	copy(sortedRoles, rawRoles)
	sortRolesDescending(sortedRoles)

	for i, r := range sortedRoles {
		if r.ID == botRoleID {
			return sortedRoles, i, nil
		}
	}
	return nil, -1, errors.New("the roles changed during the run")
}

func buildRolePositionUpdates(roles []discord.Role) []discord.RolePositionUpdate {
	n := len(roles)
	updates := make([]discord.RolePositionUpdate, n)
	for i := 0; i < n; i++ {
		wantedPos := n - i
		updates[i] = discord.RolePositionUpdate{
			ID:       roles[i].ID,
			Position: &wantedPos,
		}
	}
	return updates
}

func applySetRoleOrder(
	ctx context.Context,
	client *bot.Client,
	guildID uint64,
	def *serverdef.Definition,
	c serverplan.Change,
	idx *serverIndex,
	reason string,
	timeout time.Duration,
) error {
	sortedRoles, callerIdx, err := fetchAndSortRoles(ctx, client, guildID, idx.botRoleID, timeout)
	if err != nil {
		return err
	}

	targetSnowflakes, err := resolveTargetRoles(def, c.Detail, idx)
	if err != nil {
		return err
	}

	if err := verifyRolesPresent(sortedRoles, targetSnowflakes); err != nil {
		return err
	}

	below, newBelow, err := computeBelowAndNewBelow(sortedRoles, callerIdx, targetSnowflakes, guildID)
	if err != nil {
		return err
	}

	n := len(below)
	callerPos := sortedRoles[callerIdx].Position
	if n >= callerPos {
		return fmt.Errorf("not enough positions below the bot's role: %d roles stand below position %d", n, callerPos)
	}

	if rolesAlreadyOrdered(below, newBelow, n) {
		return nil
	}

	writeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	updates := buildRolePositionUpdates(newBelow)
	_, err = client.Rest.UpdateRolePositions(snowflake.ID(guildID), updates, rest.WithCtx(writeCtx), rest.WithReason(reason))
	if err != nil {
		return callError("set role order", err, client.Token)
	}
	return nil
}

func resolveTargetRoles(
	def *serverdef.Definition,
	detail string,
	idx *serverIndex,
) ([]snowflake.ID, error) {
	rawList := strings.Split(detail, ",")
	ids := make([]snowflake.ID, 0, len(rawList))
	for _, raw := range rawList {
		tID := strings.TrimSpace(raw)
		if tID == "" {
			continue
		}
		defRole, ok := findRoleInDef(def, tID)
		if !ok {
			return nil, fmt.Errorf("target role %q not found in definition", tID)
		}
		sID, err := resolveRoleID(defRole, idx)
		if err != nil {
			return nil, err
		}
		ids = append(ids, sID)
	}
	return ids, nil
}

func verifyRolesPresent(sortedRoles []discord.Role, targetSnowflakes []snowflake.ID) error {
	serverRoleSet := make(map[snowflake.ID]struct{}, len(sortedRoles))
	for _, r := range sortedRoles {
		serverRoleSet[r.ID] = struct{}{}
	}
	for _, id := range targetSnowflakes {
		if _, ok := serverRoleSet[id]; !ok {
			return errors.New("the roles changed during the run")
		}
	}
	return nil
}

func verifyRolesAbove(
	sortedRoles []discord.Role,
	callerIdx int,
	targetSnowflakes []snowflake.ID,
	targetSet map[snowflake.ID]struct{},
) (int, error) {
	var aboveTargetIDs []snowflake.ID
	for i := 0; i < callerIdx; i++ {
		r := sortedRoles[i]
		if _, ok := targetSet[r.ID]; ok {
			aboveTargetIDs = append(aboveTargetIDs, r.ID)
		}
	}
	k := len(aboveTargetIDs)
	for i := 0; i < k; i++ {
		if aboveTargetIDs[i] != targetSnowflakes[i] {
			return 0, errors.New("a role above the bot's role is not where the plan saw it")
		}
	}
	return k, nil
}

func collectBelowRoles(sortedRoles []discord.Role, callerIdx int, guildID uint64) []discord.Role {
	var below []discord.Role
	for i := callerIdx + 1; i < len(sortedRoles); i++ {
		r := sortedRoles[i]
		if r.ID != snowflake.ID(guildID) {
			below = append(below, r)
		}
	}
	return below
}

func reorderBelowRoles(
	below []discord.Role,
	remainingTargets []snowflake.ID,
	targetSet map[snowflake.ID]struct{},
) []discord.Role {
	belowTargetSet := make(map[snowflake.ID]struct{})
	for _, r := range below {
		if _, ok := targetSet[r.ID]; ok {
			belowTargetSet[r.ID] = struct{}{}
		}
	}
	var targetOrderInBelow []snowflake.ID
	for _, id := range remainingTargets {
		if _, ok := belowTargetSet[id]; ok {
			targetOrderInBelow = append(targetOrderInBelow, id)
		}
	}

	roleByID := make(map[snowflake.ID]discord.Role, len(below))
	for _, r := range below {
		roleByID[r.ID] = r
	}

	newBelow := make([]discord.Role, len(below))
	copy(newBelow, below)
	tIdx := 0
	for i, r := range below {
		if _, isTarget := belowTargetSet[r.ID]; isTarget {
			newBelow[i] = roleByID[targetOrderInBelow[tIdx]]
			tIdx++
		}
	}
	return newBelow
}

func computeBelowAndNewBelow(
	sortedRoles []discord.Role,
	callerIdx int,
	targetSnowflakes []snowflake.ID,
	guildID uint64,
) ([]discord.Role, []discord.Role, error) {
	targetSet := make(map[snowflake.ID]struct{}, len(targetSnowflakes))
	for _, id := range targetSnowflakes {
		targetSet[id] = struct{}{}
	}

	k, err := verifyRolesAbove(sortedRoles, callerIdx, targetSnowflakes, targetSet)
	if err != nil {
		return nil, nil, err
	}

	below := collectBelowRoles(sortedRoles, callerIdx, guildID)
	newBelow := reorderBelowRoles(below, targetSnowflakes[k:], targetSet)
	return below, newBelow, nil
}

func rolesAlreadyOrdered(below []discord.Role, newBelow []discord.Role, n int) bool {
	for i := 0; i < n; i++ {
		wantedPos := n - i
		if below[i].ID != newBelow[i].ID || below[i].Position != wantedPos {
			return false
		}
	}
	return true
}

// maskToPermissions converts an unsigned uint64 bitmask into Disgo's signed int64 Permissions.
func maskToPermissions(mask uint64) discord.Permissions {
	return discord.Permissions(mask) // #nosec G115 -- Discord permission bits are bitmasks represented as int64 in disgo.
}
