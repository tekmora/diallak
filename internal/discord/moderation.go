package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"

	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	// RemoveMessageCommandName is the message command a moderator removes a message with (ADR-0021).
	RemoveMessageCommandName = "Remove message"
	// TimeoutCommandName is the slash command a moderator times a member out with (ADR-0021).
	TimeoutCommandName = "timeout"
	// TimeoutCommandDescription is the description of the timeout command in Discord's command list.
	TimeoutCommandDescription = "Time a member out, with a reason for the moderation trail"

	// MaxTimeoutMinutes is Discord's limit of a timeout: 28 days.
	MaxTimeoutMinutes = 28 * 24 * 60
	// MaxReasonRunes caps the reason of an act; Discord caps the audit log reason at 512 characters.
	MaxReasonRunes = 512

	// DefaultTrailRetention keeps a trail line 90 days (ADR-0021).
	DefaultTrailRetention = 90 * 24 * time.Hour
	// MinTrailRetention and MaxTrailRetention bound the configured retention.
	MinTrailRetention = 24 * time.Hour
	MaxTrailRetention = 365 * 24 * time.Hour

	// removeCustomIDPrefix starts the custom id of the reason form; channel, message and author ids
	// follow it, separated by colons.
	removeCustomIDPrefix = "moderation-remove:"
	// removeReasonInputID is the custom id of the reason field of the form.
	removeReasonInputID = "reason"
	// removeFormTitle heads the reason form.
	removeFormTitle = "Remove message"
	// removeReasonLabel labels the reason field; Discord caps a label at 45 characters.
	removeReasonLabel = "Reason, for the trail and the audit log"

	// TextModerationNotModerator refuses a member without the Moderator role.
	TextModerationNotModerator = "Only moderators can use this command."
	// TextModerationUnavailable refuses every act while the Moderator role or the trail channel is
	// not found on the server.
	TextModerationUnavailable = "This command is unavailable: the Moderator role or the trail channel was not found. Tell the operator."
	// TextModerationBadReason refuses a missing or too long reason.
	TextModerationBadReason = "Nothing was done: give a reason of 1 to 512 characters."
	// TextModerationBadMinutes refuses a timeout outside Discord's limit.
	TextModerationBadMinutes = "Nothing was done: a timeout lasts 1 to 40320 minutes (28 days)."
	// TextModerationNoMember refuses a timeout without a member.
	TextModerationNoMember = "Nothing was done: choose a member of the server."
	// TextModerationBadForm refuses a form that this bot did not send.
	TextModerationBadForm = "Nothing was done: this form does not belong to a moderation command. Run the command again."
	// TextModerationRefused says that Discord refused the act.
	TextModerationRefused = "Discord refused this act. The server owner and members with the Administrator permission cannot be moderated by the bot."
	// TextModerationGone says that the message or member no longer exists.
	TextModerationGone = "Nothing was done: Discord no longer has that message or member."
	// TextModerationFailed says that the act could not be confirmed.
	TextModerationFailed = "The act could not be confirmed. Check the message or member before you try again."
	// TextModerationRemoved confirms a removal.
	TextModerationRemoved = "The message was removed and the trail line written."
	// TextModerationTimedOut confirms a timeout; the end time follows.
	TextModerationTimedOut = "The member is timed out until %s and the trail line written."
	// TextModerationNoLine says that the act was done but its trail line was not written.
	TextModerationNoLine = "The act was done, but its trail line could not be written. Tell the operator."

	// defaultModerationCallTimeout is chosen, not measured.
	defaultModerationCallTimeout = 10 * time.Second
	maxModerationCallTimeout     = 1 * time.Minute

	// trailPageSize is the largest page Get Channel Messages returns.
	trailPageSize = 100
	// maxTrailPages bounds one sweep: 5000 lines.
	maxTrailPages = 50
)

var (
	// ErrDiscordRefused marks an act Discord refused with 403.
	ErrDiscordRefused = errors.New("discord refused the act")
	// ErrDiscordGone marks an act on a message or member Discord no longer has (404).
	ErrDiscordGone = errors.New("discord no longer has the target")
)

// ModerationConfig configures the moderation commands.
type ModerationConfig struct {
	Definition  *serverdef.Definition
	GuildID     uint64
	Logger      *slog.Logger
	CallTimeout time.Duration
	// Retention is how long a trail line stays; 0 means DefaultTrailRetention.
	Retention time.Duration
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// ModerationInteraction is one interaction of a moderation command as the handler sees it.
type ModerationInteraction interface {
	GuildID() uint64
	MemberID() uint64
	MemberRoleIDs() []uint64
	// Reply answers the interaction with an ephemeral message.
	Reply(ctx context.Context, text string) error
	// AskReason answers the interaction with the reason form.
	AskReason(ctx context.Context, customID string) error
	// Defer acknowledges the interaction with an ephemeral deferred answer.
	Defer(ctx context.Context) error
	// Edit replaces the deferred answer.
	Edit(ctx context.Context, text string) error
}

// ModerationCalls are the server calls of an act. Errors wrap ErrDiscordRefused or ErrDiscordGone
// where Discord answered 403 or 404.
type ModerationCalls interface {
	DeleteMessage(ctx context.Context, channelID, messageID uint64, reason string) error
	TimeOut(ctx context.Context, memberID uint64, until time.Time, reason string) error
	PostLine(ctx context.Context, channelID uint64, line string) error
}

// MessageTarget is the message a removal acts on.
type MessageTarget struct {
	ChannelID uint64
	MessageID uint64
	AuthorID  uint64
}

// TimeoutRequest is the input of the timeout command.
type TimeoutRequest struct {
	MemberID uint64
	Minutes  int
	Reason   string
}

// moderationServer is what the handler found on the server.
type moderationServer struct {
	moderatorRoleID uint64
	trailChannelID  uint64
	botUserID       uint64
}

// ModerationHandler runs the moderation commands and the sweep of the trail (ADR-0021).
type ModerationHandler struct {
	guildID     uint64
	roleName    string
	trailName   string
	logger      *slog.Logger
	callTimeout time.Duration
	retention   time.Duration
	now         func() time.Time
	server      atomic.Pointer[moderationServer]
}

// NewModerationHandler checks cfg and returns a handler that refuses every act until SetServer
// found the Moderator role and the trail channel.
func NewModerationHandler(cfg ModerationConfig) (*ModerationHandler, error) {
	if cfg.Definition == nil {
		return nil, errors.New("definition is required")
	}
	if cfg.GuildID == 0 {
		return nil, errors.New("guild ID must not be zero")
	}
	if cfg.CallTimeout < 0 || cfg.CallTimeout > maxModerationCallTimeout {
		return nil, fmt.Errorf("call timeout %v out of range [0, %v]", cfg.CallTimeout, maxModerationCallTimeout)
	}
	if cfg.Retention != 0 && (cfg.Retention < MinTrailRetention || cfg.Retention > MaxTrailRetention) {
		return nil, fmt.Errorf("trail retention %v out of range [%v, %v]", cfg.Retention, MinTrailRetention, MaxTrailRetention)
	}
	roleName, trailName, err := moderationNames(cfg.Definition)
	if err != nil {
		return nil, err
	}
	h := &ModerationHandler{
		guildID: cfg.GuildID, roleName: roleName, trailName: trailName,
		logger: cfg.Logger, callTimeout: cfg.CallTimeout, retention: cfg.Retention, now: cfg.Now,
	}
	if h.logger == nil {
		h.logger = slog.New(slog.DiscardHandler)
	}
	if h.callTimeout == 0 {
		h.callTimeout = defaultModerationCallTimeout
	}
	if h.retention == 0 {
		h.retention = DefaultTrailRetention
	}
	if h.now == nil {
		h.now = time.Now
	}
	return h, nil
}

func moderationNames(def *serverdef.Definition) (string, string, error) {
	roleName, trailName := "", ""
	for _, r := range def.Roles {
		if r.Kind == "moderator" {
			roleName = r.Name
		}
	}
	for _, ch := range def.Channels {
		if ch.Trail {
			trailName = ch.Name
		}
	}
	if roleName == "" {
		return "", "", errors.New("the definition holds no moderator role")
	}
	if trailName == "" {
		return "", "", errors.New("the definition holds no trail channel")
	}
	return roleName, trailName, nil
}

// SetServer resolves the Moderator role and the trail channel by name. While either is missing or
// held twice, every act is refused.
func (h *ModerationHandler) SetServer(roles []discord.Role, channels []discord.GuildChannel, botUserID uint64) {
	idx := newServerIndex(h.guildID, 0)
	for _, r := range roles {
		idx.addRole(r.Name, r.ID)
	}
	for _, ch := range channels {
		idx.addChannel(ch.Name(), ch.ID())
	}
	roleID, roleErr := idx.lookupRole(h.roleName)
	trailID, trailErr := idx.lookupChannel(h.trailName)
	if err := errors.Join(roleErr, trailErr); err != nil || botUserID == 0 {
		h.logger.Warn("moderation unavailable", slog.Any("error", err), slog.Bool("bot_user_known", botUserID != 0))
		h.server.Store(nil)
		return
	}
	h.server.Store(&moderationServer{
		moderatorRoleID: uint64(roleID), trailChannelID: uint64(trailID), botUserID: botUserID,
	})
}

// HandleRemoveCommand answers the message command with the reason form. It makes no other call.
func (h *ModerationHandler) HandleRemoveCommand(ctx context.Context, in ModerationInteraction, target MessageTarget) {
	defer h.recoverPanic("HandleRemoveCommand")
	if _, refusal, ok := h.refuseSender(in); ok {
		h.reply(ctx, in, refusal)
		return
	}
	if target.ChannelID == 0 || target.MessageID == 0 || target.AuthorID == 0 {
		h.reply(ctx, in, TextModerationGone)
		return
	}
	customID := fmt.Sprintf("%s%d:%d:%d", removeCustomIDPrefix, target.ChannelID, target.MessageID, target.AuthorID)
	askCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	defer cancel()
	if err := in.AskReason(askCtx, customID); err != nil {
		h.logger.Warn("moderation form failed", slog.String("error_class", classifyError(err)))
	}
}

// HandleRemoveReason removes the message the form names and writes the trail line.
func (h *ModerationHandler) HandleRemoveReason(ctx context.Context, in ModerationInteraction, customID, reason string, calls ModerationCalls) {
	defer h.recoverPanic("HandleRemoveReason")
	srv, refusal, ok := h.refuseSender(in)
	if ok {
		h.reply(ctx, in, refusal)
		return
	}
	target, ok := parseRemoveCustomID(customID)
	if !ok {
		h.reply(ctx, in, TextModerationBadForm)
		return
	}
	reason, ok = checkReason(reason)
	if !ok {
		h.reply(ctx, in, TextModerationBadReason)
		return
	}
	act := func(callCtx context.Context) error {
		return calls.DeleteMessage(callCtx, target.ChannelID, target.MessageID, reason)
	}
	affected := fmt.Sprintf("member %d, channel <#%d>, message %d", target.AuthorID, target.ChannelID, target.MessageID)
	h.run(ctx, in, srv, calls, act, h.line(in, "removed message", affected, reason), TextModerationRemoved)
}

// HandleTimeout times the member out and writes the trail line.
func (h *ModerationHandler) HandleTimeout(ctx context.Context, in ModerationInteraction, req TimeoutRequest, calls ModerationCalls) {
	defer h.recoverPanic("HandleTimeout")
	srv, refusal, ok := h.refuseSender(in)
	if ok {
		h.reply(ctx, in, refusal)
		return
	}
	if req.MemberID == 0 {
		h.reply(ctx, in, TextModerationNoMember)
		return
	}
	if req.Minutes < 1 || req.Minutes > MaxTimeoutMinutes {
		h.reply(ctx, in, TextModerationBadMinutes)
		return
	}
	reason, ok := checkReason(req.Reason)
	if !ok {
		h.reply(ctx, in, TextModerationBadReason)
		return
	}
	until := h.now().UTC().Truncate(time.Second).Add(time.Duration(req.Minutes) * time.Minute)
	act := func(callCtx context.Context) error {
		return calls.TimeOut(callCtx, req.MemberID, until, reason)
	}
	stamp := until.Format(time.RFC3339)
	line := h.line(in, "timed out until "+stamp, fmt.Sprintf("member %d", req.MemberID), reason)
	h.run(ctx, in, srv, calls, act, line, fmt.Sprintf(TextModerationTimedOut, stamp))
}

// run defers the answer, makes the act, writes the line and says the outcome. A failed act
// writes no line.
func (h *ModerationHandler) run(ctx context.Context, in ModerationInteraction, srv *moderationServer,
	calls ModerationCalls, act func(context.Context) error, line, done string) {
	deferCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	err := in.Defer(deferCtx)
	cancel()
	if err != nil {
		h.logger.Warn("moderation defer failed", slog.String("error_class", classifyError(err)))
		return
	}

	actCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	err = act(actCtx)
	cancel()
	if err != nil {
		h.logger.Warn("moderation act failed", slog.String("error_class", classifyError(err)))
		h.edit(ctx, in, actFailureText(err))
		return
	}

	postCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	err = calls.PostLine(postCtx, srv.trailChannelID, line)
	cancel()
	if err != nil {
		h.logger.Error("moderation trail line not written", slog.String("error_class", classifyError(err)))
		h.edit(ctx, in, TextModerationNoLine)
		return
	}
	h.edit(ctx, in, done)
}

func actFailureText(err error) string {
	switch {
	case errors.Is(err, ErrDiscordRefused):
		return TextModerationRefused
	case errors.Is(err, ErrDiscordGone):
		return TextModerationGone
	default:
		return TextModerationFailed
	}
}

// line is one trail line: time, moderator, act, affected member and place, reason.
func (h *ModerationHandler) line(in ModerationInteraction, act, affected, reason string) string {
	return fmt.Sprintf("%s · <@%d> · %s · %s · reason: %s",
		h.now().UTC().Format(time.RFC3339), in.MemberID(), act, affected, oneLine(reason))
}

// oneLine turns every control character into a space, so a reason cannot add lines to the trail.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

// checkReason trims the reason and accepts 1 to MaxReasonRunes characters of valid UTF-8.
func checkReason(reason string) (string, bool) {
	if !utf8.ValidString(reason) {
		return "", false
	}
	reason = strings.TrimSpace(reason)
	n := utf8.RuneCountInString(reason)
	return reason, n >= 1 && n <= MaxReasonRunes
}

func parseRemoveCustomID(customID string) (MessageTarget, bool) {
	rest, ok := strings.CutPrefix(customID, removeCustomIDPrefix)
	if !ok {
		return MessageTarget{}, false
	}
	parts := strings.Split(rest, ":")
	if len(parts) != 3 {
		return MessageTarget{}, false
	}
	var ids [3]uint64
	for i, p := range parts {
		id, err := strconv.ParseUint(p, 10, 64)
		if err != nil || id == 0 {
			return MessageTarget{}, false
		}
		ids[i] = id
	}
	return MessageTarget{ChannelID: ids[0], MessageID: ids[1], AuthorID: ids[2]}, true
}

// refuseSender returns the server read and a refusal for an interaction from another server, while
// the server is not resolved, or from a member without the Moderator role.
func (h *ModerationHandler) refuseSender(in ModerationInteraction) (*moderationServer, string, bool) {
	if in.GuildID() == 0 || in.GuildID() != h.guildID {
		h.logger.Info("moderation refused", slog.String("reason", "other-guild"))
		return nil, TextWrongGuild, true
	}
	srv := h.server.Load()
	if srv == nil {
		h.logger.Info("moderation refused", slog.String("reason", "unavailable"))
		return nil, TextModerationUnavailable, true
	}
	if !slices.Contains(in.MemberRoleIDs(), srv.moderatorRoleID) {
		h.logger.Info("moderation refused", slog.String("reason", "not-moderator"))
		return nil, TextModerationNotModerator, true
	}
	return srv, "", false
}

func (h *ModerationHandler) reply(ctx context.Context, in ModerationInteraction, text string) {
	replyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	defer cancel()
	if err := in.Reply(replyCtx, text); err != nil {
		h.logger.Warn("moderation answer failed", slog.String("error_class", classifyError(err)))
	}
}

func (h *ModerationHandler) edit(ctx context.Context, in ModerationInteraction, text string) {
	editCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	defer cancel()
	if err := in.Edit(editCtx, text); err != nil {
		h.logger.Warn("moderation edit failed", slog.String("error_class", classifyError(err)))
	}
}

func (h *ModerationHandler) recoverPanic(where string) {
	if r := recover(); r != nil {
		h.logger.Error("recovered from panic in "+where, slog.String("panic_type", fmt.Sprintf("%T", r)))
	}
}

// TrailMessage is one message of the trail channel as the sweep sees it.
type TrailMessage struct {
	ID       uint64
	AuthorID uint64
	Sent     time.Time
}

// TrailCalls are the server calls of the sweep. ListLines returns up to trailPageSize messages
// older than before (0 = newest), newest first.
type TrailCalls interface {
	ListLines(ctx context.Context, channelID, before uint64) ([]TrailMessage, error)
	DeleteLine(ctx context.Context, channelID, messageID uint64) error
}

// Sweep deletes the bot's own trail lines older than the retention, and returns how many it
// deleted. It reads at most maxTrailPages pages; other authors' messages stay.
func (h *ModerationHandler) Sweep(ctx context.Context, calls TrailCalls) (int, error) {
	srv := h.server.Load()
	if srv == nil {
		return 0, errors.New("trail channel not resolved")
	}
	cutoff := h.now().Add(-h.retention)
	deleted := 0
	before := uint64(0)
	for range maxTrailPages {
		listCtx, cancel := context.WithTimeout(ctx, h.callTimeout)
		page, err := calls.ListLines(listCtx, srv.trailChannelID, before)
		cancel()
		if err != nil {
			return deleted, fmt.Errorf("list trail lines: %w", err)
		}
		n, err := h.sweepPage(ctx, calls, srv, page, cutoff)
		deleted += n
		if err != nil {
			return deleted, err
		}
		if len(page) < trailPageSize {
			return deleted, nil
		}
		before = oldestID(page)
	}
	h.logger.Warn("trail sweep stopped at its page bound", slog.Int("pages", maxTrailPages))
	return deleted, nil
}

func (h *ModerationHandler) sweepPage(ctx context.Context, calls TrailCalls, srv *moderationServer,
	page []TrailMessage, cutoff time.Time) (int, error) {
	deleted := 0
	for _, m := range page {
		if m.AuthorID != srv.botUserID || !m.Sent.Before(cutoff) {
			continue
		}
		delCtx, cancel := context.WithTimeout(ctx, h.callTimeout)
		err := calls.DeleteLine(delCtx, srv.trailChannelID, m.ID)
		cancel()
		if err != nil && !errors.Is(err, ErrDiscordGone) {
			return deleted, fmt.Errorf("delete trail line: %w", err)
		}
		deleted++
	}
	return deleted, nil
}

func oldestID(page []TrailMessage) uint64 {
	oldest := page[0].ID
	for _, m := range page[1:] {
		oldest = min(oldest, m.ID)
	}
	return oldest
}
