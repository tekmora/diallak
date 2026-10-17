package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"
)

const (
	// trailSweepInterval is the wait between two sweeps of the trail: once a day (ADR-0021).
	trailSweepInterval = 24 * time.Hour
	// maxTrailSweeps bounds the sweep loop: more than 270 years of daily sweeps.
	maxTrailSweeps = 100_000
)

// moderationAdapter is an interaction of a moderation command, delivered by the library.
type moderationAdapter struct {
	client      *bot.Client
	interaction discord.Interaction
	respond     func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error
}

func (a *moderationAdapter) GuildID() uint64 {
	return extractGuildID(a.interaction.GuildID())
}

func (a *moderationAdapter) MemberID() uint64 {
	m := a.interaction.Member()
	if m == nil {
		return 0
	}
	return uint64(m.User.ID)
}

func (a *moderationAdapter) MemberRoleIDs() []uint64 {
	m := a.interaction.Member()
	if m == nil {
		return nil
	}
	ids := make([]uint64, len(m.RoleIDs))
	for i, id := range m.RoleIDs {
		ids[i] = uint64(id)
	}
	return ids
}

func (a *moderationAdapter) Reply(ctx context.Context, text string) error {
	mentions := defaultAllowedMentions()
	return a.respond(discord.InteractionResponseTypeCreateMessage, discord.MessageCreate{
		Content:         text,
		Flags:           discord.MessageFlagEphemeral,
		AllowedMentions: &mentions,
	}, rest.WithCtx(ctx))
}

func (a *moderationAdapter) AskReason(ctx context.Context, customID string) error {
	minLength := 1
	return a.respond(discord.InteractionResponseTypeModal, discord.ModalCreate{
		CustomID: customID,
		Title:    removeFormTitle,
		Components: []discord.LayoutComponent{
			discord.NewLabel(removeReasonLabel, discord.TextInputComponent{
				CustomID:  removeReasonInputID,
				Style:     discord.TextInputStyleParagraph,
				MinLength: &minLength,
				MaxLength: MaxReasonRunes,
				Required:  true,
			}),
		},
	}, rest.WithCtx(ctx))
}

func (a *moderationAdapter) Defer(ctx context.Context) error {
	return a.respond(discord.InteractionResponseTypeDeferredCreateMessage, buildDeferPayload(), rest.WithCtx(ctx))
}

func (a *moderationAdapter) Edit(ctx context.Context, text string) error {
	mentions := defaultAllowedMentions()
	_, err := a.client.Rest.UpdateInteractionResponse(
		a.interaction.ApplicationID(),
		a.interaction.Token(),
		discord.MessageUpdate{Content: &text, AllowedMentions: &mentions},
		rest.WithCtx(ctx),
	)
	return err
}

// restModerationCalls makes the acts and the trail calls against the server. The reason of an act
// goes to the audit log.
type restModerationCalls struct {
	rest    rest.Rest
	guildID snowflake.ID
}

func (c restModerationCalls) DeleteMessage(ctx context.Context, channelID, messageID uint64, reason string) error {
	err := c.rest.DeleteMessage(snowflake.ID(channelID), snowflake.ID(messageID), rest.WithCtx(ctx), rest.WithReason(reason))
	return discordCallError("delete message", err)
}

func (c restModerationCalls) TimeOut(ctx context.Context, memberID uint64, until time.Time, reason string) error {
	_, err := c.rest.UpdateMember(c.guildID, snowflake.ID(memberID), discord.MemberUpdate{
		CommunicationDisabledUntil: omit.NewPtr(until),
	}, rest.WithCtx(ctx), rest.WithReason(reason))
	return discordCallError("modify guild member", err)
}

func (c restModerationCalls) PostLine(ctx context.Context, channelID uint64, line string) error {
	mentions := defaultAllowedMentions()
	_, err := c.rest.CreateMessage(snowflake.ID(channelID), discord.MessageCreate{
		Content:         line,
		AllowedMentions: &mentions,
	}, rest.WithCtx(ctx))
	return discordCallError("create message", err)
}

func (c restModerationCalls) ListLines(ctx context.Context, channelID, before uint64) ([]TrailMessage, error) {
	messages, err := c.rest.GetMessages(snowflake.ID(channelID), 0, snowflake.ID(before), 0, trailPageSize, rest.WithCtx(ctx))
	if err != nil {
		return nil, discordCallError("get channel messages", err)
	}
	lines := make([]TrailMessage, len(messages))
	for i, m := range messages {
		lines[i] = TrailMessage{ID: uint64(m.ID), AuthorID: uint64(m.Author.ID), Sent: m.CreatedAt}
	}
	return lines, nil
}

func (c restModerationCalls) DeleteLine(ctx context.Context, channelID, messageID uint64) error {
	err := c.rest.DeleteMessage(snowflake.ID(channelID), snowflake.ID(messageID), rest.WithCtx(ctx))
	return discordCallError("delete trail line", err)
}

// discordCallError names the step of a failed call and marks the answers 403 and 404, which the
// moderator is told apart from other failures.
func discordCallError(step string, err error) error {
	if err == nil {
		return nil
	}
	var restErr *rest.Error
	if errors.As(err, &restErr) && restErr.Response != nil {
		switch restErr.Response.StatusCode {
		case http.StatusForbidden:
			err = errors.Join(ErrDiscordRefused, err)
		case http.StatusNotFound:
			err = errors.Join(ErrDiscordGone, err)
		}
	}
	return callError(step, err, "")
}

// removeMessageTarget returns the message a Remove message command names.
func removeMessageTarget(data discord.MessageCommandInteractionData) MessageTarget {
	m := data.TargetMessage()
	return MessageTarget{ChannelID: uint64(m.ChannelID), MessageID: uint64(data.TargetID()), AuthorID: uint64(m.Author.ID)}
}

// timeoutRequest returns the options of a timeout command.
func timeoutRequest(data discord.SlashCommandInteractionData) TimeoutRequest {
	return TimeoutRequest{
		MemberID: uint64(data.Snowflake(OptionTimeoutMember)),
		Minutes:  data.Int(OptionTimeoutMinutes),
		Reason:   data.String(OptionTimeoutReason),
	}
}

// removeReasonForm returns the custom id and the reason of a submitted reason form.
func removeReasonForm(i discord.ModalSubmitInteraction) (string, string, bool) {
	if !strings.HasPrefix(i.Data.CustomID, removeCustomIDPrefix) {
		return "", "", false
	}
	return i.Data.CustomID, i.Data.Text(removeReasonInputID), true
}

const (
	// OptionTimeoutMember, OptionTimeoutMinutes and OptionTimeoutReason are the options of the
	// timeout command.
	OptionTimeoutMember  = "member"
	OptionTimeoutMinutes = "minutes"
	OptionTimeoutReason  = "reason"
)

func buildRemoveMessageCommandCreate() discord.ApplicationCommandCreate {
	return discord.MessageCommandCreate{Name: RemoveMessageCommandName}
}

func buildTimeoutCommandCreate() discord.ApplicationCommandCreate {
	minMinutes, maxMinutes := 1, MaxTimeoutMinutes
	minReason, maxReason := 1, MaxReasonRunes
	return discord.SlashCommandCreate{
		Name:        TimeoutCommandName,
		Description: TimeoutCommandDescription,
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionUser{Name: OptionTimeoutMember, Description: "The member to time out", Required: true},
			discord.ApplicationCommandOptionInt{
				Name: OptionTimeoutMinutes, Description: "Length in minutes, 1 to 40320 (28 days)", Required: true,
				MinValue: &minMinutes, MaxValue: &maxMinutes,
			},
			discord.ApplicationCommandOptionString{
				Name: OptionTimeoutReason, Description: "Why, for the trail and the audit log", Required: true,
				MinLength: &minReason, MaxLength: &maxReason,
			},
		},
	}
}

// readModerationServer reads the bot user, the roles and the channels, and gives them to the
// moderation handler.
func readModerationServer(ctx context.Context, restClient rest.Rest, guildID uint64, m *ModerationHandler, timeout time.Duration) error {
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	app, err := restClient.GetCurrentApplication(rest.WithCtx(readCtx))
	if err != nil {
		return callError("lookup current application", err, "")
	}
	if app == nil || app.Bot == nil || app.Bot.ID == 0 {
		return errors.New("lookup current application: no bot user")
	}
	roles, err := restClient.GetRoles(snowflake.ID(guildID), rest.WithCtx(readCtx))
	if err != nil {
		return callError("get roles", err, "")
	}
	channels, err := restClient.GetGuildChannels(snowflake.ID(guildID), rest.WithCtx(readCtx))
	if err != nil {
		return callError("get guild channels", err, "")
	}
	m.SetServer(roles, channels, uint64(app.Bot.ID))
	return nil
}

// sweepTrail reads the server and sweeps the trail once a day until ctx ends. A failed round is
// logged and the next one tries again.
func sweepTrail(ctx context.Context, restClient rest.Rest, cfg BotConfig, opts clientOptions) {
	calls := restModerationCalls{rest: restClient, guildID: snowflake.ID(cfg.GuildID)}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for range maxTrailSweeps {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if err := readModerationServer(ctx, restClient, cfg.GuildID, cfg.Moderation, opts.callTimeout()); err != nil {
			cfg.Logger.Warn("moderation server read failed", slog.Any("error", err))
		} else if n, err := cfg.Moderation.Sweep(ctx, calls); err != nil {
			cfg.Logger.Warn("trail sweep failed", slog.Int("deleted", n), slog.Any("error", err))
		} else {
			cfg.Logger.Info("trail swept", slog.Int("deleted", n))
		}
		timer.Reset(opts.sweepInterval())
	}
}

// startModeration reads the server once before the gateway opens, so the first command needs no
// read, and fails the start when it cannot.
func startModeration(ctx context.Context, client *bot.Client, cfg BotConfig, opts clientOptions) error {
	if err := readModerationServer(ctx, client.Rest, cfg.GuildID, cfg.Moderation, opts.callTimeout()); err != nil {
		return fmt.Errorf("read the server for the moderation commands: %w", sanitizeToken(err, cfg.Token))
	}
	return nil
}
