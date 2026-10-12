package discord

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
)

// channelListCutoff is the point in time from which Discord's Get Guild Channels
// leaves out every channel the caller cannot view; a caller with ADMINISTRATOR
// views every channel.
const channelListCutoff = "2026-11-16T00:00:00Z"

// ReadServer reads the current Discord server state into a Snapshot using the bot's credentials.
func ReadServer(ctx context.Context, token string, guildID uint64) (*serverplan.Snapshot, error) {
	return readServerWithOptions(ctx, token, guildID, clientOptions{})
}

func readServerWithOptions(ctx context.Context, token string, guildID uint64, opts clientOptions) (*serverplan.Snapshot, error) {
	if err := checkTokenAndGuild(token, guildID); err != nil {
		return nil, err
	}

	client, err := newRestClient(token, opts.restURL)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	snap, _, err := fetchSnapshotAndIndex(ctx, client, token, guildID, opts)
	return snap, err
}

func fetchSnapshotAndIndex(
	ctx context.Context,
	client *bot.Client,
	token string,
	guildID uint64,
	opts clientOptions,
) (*serverplan.Snapshot, *serverIndex, error) {
	data, err := fetchServerData(ctx, client, token, guildID, opts)
	if err != nil {
		return nil, nil, err
	}
	return buildSnapshotAndIndex(data, guildID, opts.getNow())
}

func buildSnapshotAndIndex(data *serverData, guildID uint64, now time.Time) (*serverplan.Snapshot, *serverIndex, error) {
	snapRoles, everyoneRole, roleByID, err := parseRoles(data.guild.Roles, guildID)
	if err != nil {
		return nil, nil, err
	}

	caller, botRoleID, err := resolveCaller(data.app.Bot, data.member.RoleIDs, data.guild.Roles, everyoneRole, roleByID)
	if err != nil {
		return nil, nil, err
	}

	categories, channels, err := parseChannelsAndCategories(data.channels, roleByID)
	if err != nil {
		return nil, nil, err
	}

	features := make([]string, len(data.guild.Features))
	for i, f := range data.guild.Features {
		features[i] = string(f)
	}

	snap := &serverplan.Snapshot{
		Features:         features,
		Caller:           caller,
		Roles:            snapRoles,
		Categories:       categories,
		Channels:         channels,
		ChannelsComplete: determineChannelsComplete(caller, now),
	}

	if err := serverplan.Check(snap); err != nil {
		return nil, nil, fmt.Errorf("check snapshot: %w", err)
	}

	idx := newServerIndex(guildID, botRoleID)
	for _, r := range data.guild.Roles {
		idx.addRole(r.Name, r.ID)
	}
	for _, ch := range data.channels {
		if ch.Type() == discord.ChannelTypeGuildCategory {
			idx.addCategory(ch.Name(), ch.ID())
		} else {
			idx.addChannel(ch.Name(), ch.ID())
		}
	}

	return snap, idx, nil
}

type serverData struct {
	app      *discord.Application
	guild    *discord.RestGuild
	member   *discord.Member
	channels []discord.GuildChannel
}

func fetchServerData(ctx context.Context, client *bot.Client, token string, guildID uint64, opts clientOptions) (*serverData, error) {
	appCtx, appCancel := context.WithTimeout(ctx, opts.callTimeout())
	app, err := client.Rest.GetCurrentApplication(rest.WithCtx(appCtx))
	appCancel()
	if err != nil {
		return nil, callError("get application", err, token)
	}
	if app == nil || app.Bot == nil {
		return nil, errors.New("get application: the application has no bot user")
	}

	guildCtx, guildCancel := context.WithTimeout(ctx, opts.callTimeout())
	guild, err := client.Rest.GetGuild(snowflake.ID(guildID), false, rest.WithCtx(guildCtx))
	guildCancel()
	if err != nil {
		return nil, callError("get guild", err, token)
	}
	if guild == nil {
		return nil, errors.New("get guild: empty response")
	}

	memberCtx, memberCancel := context.WithTimeout(ctx, opts.callTimeout())
	member, err := client.Rest.GetMember(snowflake.ID(guildID), app.Bot.ID, rest.WithCtx(memberCtx))
	memberCancel()
	if err != nil {
		return nil, callError("get member", err, token)
	}
	if member == nil {
		return nil, errors.New("get member: empty response")
	}

	channelsCtx, channelsCancel := context.WithTimeout(ctx, opts.callTimeout())
	channels, err := client.Rest.GetGuildChannels(snowflake.ID(guildID), rest.WithCtx(channelsCtx))
	channelsCancel()
	if err != nil {
		return nil, callError("get channels", err, token)
	}

	return &serverData{app: app, guild: guild, member: member, channels: channels}, nil
}

// callError names the step of a failed request. A transport failure carries the
// request's address, which holds the server's id and the bot user's id: only the
// cause is kept. The reply of the platform to a refused request holds no address.
func callError(step string, err error, token string) error {
	var transport *url.Error
	if errors.As(err, &transport) {
		err = transport.Err
	}
	return fmt.Errorf("%s: %w", step, sanitizeToken(err, token))
}

func sortRolesDescending(roles []discord.Role) {
	sort.Slice(roles, func(i, j int) bool {
		if roles[i].Position != roles[j].Position {
			return roles[i].Position > roles[j].Position
		}
		return roles[i].ID < roles[j].ID
	})
}

func parseRoles(rawRoles []discord.Role, guildID uint64) ([]serverplan.Role, *discord.Role, map[snowflake.ID]discord.Role, error) {
	rolesCopy := make([]discord.Role, len(rawRoles))
	copy(rolesCopy, rawRoles)
	sortRolesDescending(rolesCopy)

	roleByID := make(map[snowflake.ID]discord.Role, len(rawRoles))
	var everyoneRole *discord.Role
	for i := range rawRoles {
		roleByID[rawRoles[i].ID] = rawRoles[i]
		if rawRoles[i].ID == snowflake.ID(guildID) {
			everyoneRole = &rawRoles[i]
		}
	}
	if everyoneRole == nil {
		return nil, nil, nil, errors.New("everyone role not found on guild")
	}

	snapRoles := make([]serverplan.Role, 0, len(rolesCopy))
	for _, r := range rolesCopy {
		if r.ID == snowflake.ID(guildID) {
			continue
		}
		permNames, unknown := serverdef.PermissionNames(permissionsToMask(r.Permissions))
		snapRoles = append(snapRoles, serverplan.Role{
			Name:        r.Name,
			Everyone:    false,
			Managed:     r.Managed,
			ShownApart:  r.Hoist,
			Mentionable: r.Mentionable,
			Permissions: permNames,
			UnknownBits: unknownBits(unknown),
		})
	}

	everyonePerms, unknown := serverdef.PermissionNames(permissionsToMask(everyoneRole.Permissions))
	snapRoles = append(snapRoles, serverplan.Role{
		Name:        everyoneRole.Name,
		Everyone:    true,
		Managed:     everyoneRole.Managed,
		ShownApart:  everyoneRole.Hoist,
		Mentionable: everyoneRole.Mentionable,
		Permissions: everyonePerms,
		UnknownBits: unknownBits(unknown),
	})

	return snapRoles, everyoneRole, roleByID, nil
}

func resolveCaller(
	appBot *discord.User,
	memberRoleIDs []snowflake.ID,
	guildRoles []discord.Role,
	everyoneRole *discord.Role,
	roleByID map[snowflake.ID]discord.Role,
) (serverplan.Caller, snowflake.ID, error) {
	var botRole *discord.Role
	for i := range guildRoles {
		r := &guildRoles[i]
		if r.Managed && r.Tags != nil && r.Tags.BotID != nil && *r.Tags.BotID == appBot.ID {
			botRole = r
			break
		}
	}
	if botRole == nil || !slices.Contains(memberRoleIDs, botRole.ID) {
		return serverplan.Caller{}, 0, errors.New("the bot has no role of its own on this server")
	}

	callerMask := permissionsToMask(everyoneRole.Permissions)
	for _, rID := range memberRoleIDs {
		r, ok := roleByID[rID]
		if !ok {
			return serverplan.Caller{}, 0, errors.New("the bot holds a role that the server's role list does not have; the server changed during the read")
		}
		callerMask |= permissionsToMask(r.Permissions)
	}
	// A bit the table does not name gives the caller no right the plan knows;
	// the snapshot reports such bits per role.
	callerPerms, _ := serverdef.PermissionNames(callerMask)

	return serverplan.Caller{
		Role:        botRole.Name,
		Position:    botRole.Position,
		Permissions: callerPerms,
	}, botRole.ID, nil
}

type serverIndex struct {
	everyoneRoleID snowflake.ID
	botRoleID      snowflake.ID
	roles          map[string][]snowflake.ID
	categories     map[string][]snowflake.ID
	channels       map[string][]snowflake.ID
}

func newServerIndex(guildID uint64, botRoleID snowflake.ID) *serverIndex {
	return &serverIndex{
		everyoneRoleID: snowflake.ID(guildID),
		botRoleID:      botRoleID,
		roles:          make(map[string][]snowflake.ID),
		categories:     make(map[string][]snowflake.ID),
		channels:       make(map[string][]snowflake.ID),
	}
}

func (idx *serverIndex) addRole(name string, id snowflake.ID) {
	idx.roles[name] = append(idx.roles[name], id)
}

func (idx *serverIndex) addCategory(name string, id snowflake.ID) {
	idx.categories[name] = append(idx.categories[name], id)
}

func (idx *serverIndex) addChannel(name string, id snowflake.ID) {
	idx.channels[name] = append(idx.channels[name], id)
}

func (idx *serverIndex) lookupRole(name string) (snowflake.ID, error) {
	ids := idx.roles[name]
	if len(ids) == 0 {
		return 0, fmt.Errorf("role %q not found", name)
	}
	if len(ids) > 1 {
		return 0, fmt.Errorf("role %q is held more than once", name)
	}
	return ids[0], nil
}

func (idx *serverIndex) lookupCategory(name string) (snowflake.ID, error) {
	ids := idx.categories[name]
	if len(ids) == 0 {
		return 0, fmt.Errorf("category %q not found", name)
	}
	if len(ids) > 1 {
		return 0, fmt.Errorf("category %q is held more than once", name)
	}
	return ids[0], nil
}

func (idx *serverIndex) lookupChannel(name string) (snowflake.ID, error) {
	ids := idx.channels[name]
	if len(ids) == 0 {
		return 0, fmt.Errorf("channel %q not found", name)
	}
	if len(ids) > 1 {
		return 0, fmt.Errorf("channel %q is held more than once", name)
	}
	return ids[0], nil
}

func parseChannelsAndCategories(
	guildChannels []discord.GuildChannel,
	roleByID map[snowflake.ID]discord.Role,
) ([]serverplan.Category, []serverplan.Channel, error) {
	catMap := make(map[snowflake.ID]string)
	var categories []serverplan.Category
	for _, ch := range guildChannels {
		if ch.Type() == discord.ChannelTypeGuildCategory {
			catMap[ch.ID()] = ch.Name()
			categories = append(categories, serverplan.Category{Name: ch.Name()})
		}
	}

	var channels []serverplan.Channel
	for _, ch := range guildChannels {
		if ch.Type() == discord.ChannelTypeGuildCategory {
			continue
		}
		chType := mapChannelType(ch.Type())
		catName := ""
		if ch.ParentID() != nil {
			parentName, ok := catMap[*ch.ParentID()]
			if !ok {
				return nil, nil, fmt.Errorf("channel %q has unknown parent category", ch.Name())
			}
			catName = parentName
		}

		access, memberEntries, err := parseOverwrites(ch.Name(), ch.PermissionOverwrites(), roleByID)
		if err != nil {
			return nil, nil, err
		}

		channels = append(channels, serverplan.Channel{
			Name:          ch.Name(),
			Type:          chType,
			Category:      catName,
			Access:        access,
			MemberEntries: memberEntries,
		})
	}
	return categories, channels, nil
}

func mapChannelType(t discord.ChannelType) string {
	switch t {
	case discord.ChannelTypeGuildText:
		return "text"
	case discord.ChannelTypeGuildVoice:
		return "voice"
	case discord.ChannelTypeGuildNews:
		return "announcement"
	case discord.ChannelTypeGuildForum:
		return "forum"
	default:
		return "other"
	}
}

func parseOverwrites(
	channelName string,
	overwrites discord.PermissionOverwrites,
	roleByID map[snowflake.ID]discord.Role,
) ([]serverplan.Access, int, error) {
	var access []serverplan.Access
	var memberEntries int
	for _, po := range overwrites {
		switch po.Type() {
		case discord.PermissionOverwriteTypeMember:
			memberEntries++
		case discord.PermissionOverwriteTypeRole:
			ro, ok := po.(discord.RolePermissionOverwrite)
			if !ok {
				continue
			}
			role, exists := roleByID[ro.RoleID]
			if !exists {
				return nil, 0, fmt.Errorf("channel %q has overwrite for unknown role", channelName)
			}
			allow, unkAllow := serverdef.PermissionNames(permissionsToMask(ro.Allow))
			deny, unkDeny := serverdef.PermissionNames(permissionsToMask(ro.Deny))
			access = append(access, serverplan.Access{
				Role:         role.Name,
				Allow:        allow,
				Deny:         deny,
				UnknownAllow: unknownBits(unkAllow),
				UnknownDeny:  unknownBits(unkDeny),
			})
		}
	}
	return access, memberEntries, nil
}

func determineChannelsComplete(caller serverplan.Caller, now time.Time) bool {
	if slices.Contains(caller.Permissions, "ADMINISTRATOR") {
		return true
	}
	cutoff, err := time.Parse(time.RFC3339, channelListCutoff)
	if err != nil {
		return false
	}
	return now.Before(cutoff)
}

// unknownBits lists the numbers of the bits set in unknown, in rising order.
// The read keeps such a bit as a number: Discord's reference describes a
// permission value as an integer of open length that new permissions are added
// to, so a bit the table does not name is no reason to stop.
func unknownBits(unknown uint64) []int {
	var bits []int
	for b := 0; b < 64; b++ {
		if unknown&(uint64(1)<<b) != 0 {
			bits = append(bits, b)
		}
	}
	return bits
}

// permissionsToMask converts Disgo's signed int64 permission bitmask into an unsigned uint64.
func permissionsToMask(p discord.Permissions) uint64 {
	return uint64(p) // #nosec G115 -- Discord permission bits are bitmasks represented as int64 in disgo.
}
