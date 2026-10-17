package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/serverdef"
)

// The tests of the moderation handler use a fake interaction and fake calls. The server holds the
// Moderator role and the trail channel mod-log of modTestDefinition; the clock stands at modNow.

const (
	modGuildID     uint64 = 811000000000000001
	modModeratorID uint64 = 821000000000000001
	modMemberID    uint64 = 821000000000000002
	modRoleID      uint64 = 831000000000000001
	modOtherRoleID uint64 = 831000000000000002
	modTrailID     uint64 = 841000000000000001
	modChannelID   uint64 = 841000000000000002
	modMessageID   uint64 = 851000000000000001
	modBotUserID   uint64 = 861000000000000001
)

var modNow = time.Date(2026, 10, 17, 12, 0, 0, 500, time.UTC)

func modTestDefinition() *serverdef.Definition {
	return &serverdef.Definition{
		Roles: []serverdef.Role{
			{ID: "everyone", Name: "@everyone", Kind: "everyone"},
			{ID: "moderator", Name: "Moderator", Kind: "moderator"},
		},
		Channels: []serverdef.Channel{
			{ID: "general", Name: "general"},
			{ID: "mod-log", Name: "mod-log", Staff: true, Trail: true},
		},
	}
}

func modTestRoles() []discord.Role {
	return []discord.Role{
		{ID: snowflake.ID(modGuildID), Name: "@everyone"},
		{ID: snowflake.ID(modRoleID), Name: "Moderator"},
		{ID: snowflake.ID(modOtherRoleID), Name: "Helper"},
	}
}

func modTestChannels() []discord.GuildChannel {
	return []discord.GuildChannel{modTextChannel(modChannelID, "general"), modTextChannel(modTrailID, "mod-log")}
}

// modTextChannel decodes a text channel the way the REST client does; disgo's channel fields are
// not exported.
func modTextChannel(id uint64, name string) discord.GuildChannel {
	var u discord.UnmarshalChannel
	raw := fmt.Sprintf(`{"id":"%d","guild_id":"%d","type":0,"name":%q}`, id, modGuildID, name)
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		panic(err)
	}
	return u.Channel.(discord.GuildChannel)
}

func newModHandler(t *testing.T) *ModerationHandler {
	t.Helper()
	h, err := NewModerationHandler(ModerationConfig{
		Definition: modTestDefinition(), GuildID: modGuildID, Logger: slog.New(slog.DiscardHandler),
		Now: func() time.Time { return modNow },
	})
	if err != nil {
		t.Fatalf("NewModerationHandler: %v", err)
	}
	h.SetServer(modTestRoles(), modTestChannels(), modBotUserID)
	return h
}

// fakeModInteraction records what the handler answers.
type fakeModInteraction struct {
	guildID  uint64
	memberID uint64
	roles    []uint64

	mu      sync.Mutex
	replies []string
	forms   []string
	defers  int
	edits   []string
}

func newModInteraction(roles ...uint64) *fakeModInteraction {
	return &fakeModInteraction{guildID: modGuildID, memberID: modModeratorID, roles: roles}
}

func (f *fakeModInteraction) GuildID() uint64         { return f.guildID }
func (f *fakeModInteraction) MemberID() uint64        { return f.memberID }
func (f *fakeModInteraction) MemberRoleIDs() []uint64 { return f.roles }

func (f *fakeModInteraction) Reply(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies = append(f.replies, text)
	return nil
}

func (f *fakeModInteraction) AskReason(_ context.Context, customID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forms = append(f.forms, customID)
	return nil
}

func (f *fakeModInteraction) Defer(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.defers++
	return nil
}

func (f *fakeModInteraction) Edit(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edits = append(f.edits, text)
	return nil
}

// answers is every text the member saw, replies first.
func (f *fakeModInteraction) answers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append(append([]string(nil), f.replies...), f.edits...)
}

// fakeModCalls is the server side of an act.
type fakeModCalls struct {
	deleteErr, timeoutErr, postErr error

	mu       sync.Mutex
	deletes  []string
	timeouts []string
	lines    []string
}

func (f *fakeModCalls) DeleteMessage(_ context.Context, channelID, messageID uint64, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, fmt.Sprintf("%d/%d %s", channelID, messageID, reason))
	return f.deleteErr
}

func (f *fakeModCalls) TimeOut(_ context.Context, memberID uint64, until time.Time, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.timeouts = append(f.timeouts, fmt.Sprintf("%d %s %s", memberID, until.Format(time.RFC3339Nano), reason))
	return f.timeoutErr
}

func (f *fakeModCalls) PostLine(_ context.Context, channelID uint64, line string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lines = append(f.lines, fmt.Sprintf("%d: %s", channelID, line))
	return f.postErr
}

func (f *fakeModCalls) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.deletes) + len(f.timeouts) + len(f.lines)
}

func removeTarget() MessageTarget {
	return MessageTarget{ChannelID: modChannelID, MessageID: modMessageID, AuthorID: modMemberID}
}

func removeCustomID() string {
	return fmt.Sprintf("moderation-remove:%d:%d:%d", modChannelID, modMessageID, modMemberID)
}

func TestModeration_Positive_RemoveAsksForAReasonThenDeletesAndWritesTheLine(t *testing.T) {
	h := newModHandler(t)
	cmd := newModInteraction(modRoleID)
	h.HandleRemoveCommand(context.Background(), cmd, removeTarget())
	if len(cmd.forms) != 1 || cmd.forms[0] != removeCustomID() || len(cmd.replies) != 0 {
		t.Fatalf("forms = %q, replies = %q; want one form %q", cmd.forms, cmd.replies, removeCustomID())
	}

	submit := newModInteraction(modRoleID)
	calls := &fakeModCalls{}
	h.HandleRemoveReason(context.Background(), submit, cmd.forms[0], "  Spam link  ", calls)

	wantDelete := fmt.Sprintf("%d/%d Spam link", modChannelID, modMessageID)
	if len(calls.deletes) != 1 || calls.deletes[0] != wantDelete {
		t.Errorf("deletes = %q, want %q", calls.deletes, wantDelete)
	}
	wantLine := fmt.Sprintf("%d: 2026-10-17T12:00:00Z · <@%d> · removed message · member %d, channel <#%d>, message %d · reason: Spam link",
		modTrailID, modModeratorID, modMemberID, modChannelID, modMessageID)
	if len(calls.lines) != 1 || calls.lines[0] != wantLine {
		t.Errorf("lines = %q\nwant  %q", calls.lines, wantLine)
	}
	if submit.defers != 1 || len(submit.edits) != 1 || submit.edits[0] != TextModerationRemoved {
		t.Errorf("defers = %d, edits = %q; want one defer and %q", submit.defers, submit.edits, TextModerationRemoved)
	}
}

func TestModeration_Positive_TimeoutCallsDiscordAndWritesTheLine(t *testing.T) {
	h := newModHandler(t)
	in := newModInteraction(modOtherRoleID, modRoleID)
	calls := &fakeModCalls{}
	h.HandleTimeout(context.Background(), in, TimeoutRequest{MemberID: modMemberID, Minutes: 90, Reason: "Insults"}, calls)

	wantTimeout := fmt.Sprintf("%d 2026-10-17T13:30:00Z Insults", modMemberID)
	if len(calls.timeouts) != 1 || calls.timeouts[0] != wantTimeout {
		t.Errorf("timeouts = %q, want %q", calls.timeouts, wantTimeout)
	}
	wantLine := fmt.Sprintf("%d: 2026-10-17T12:00:00Z · <@%d> · timed out until 2026-10-17T13:30:00Z · member %d · reason: Insults",
		modTrailID, modModeratorID, modMemberID)
	if len(calls.lines) != 1 || calls.lines[0] != wantLine {
		t.Errorf("lines = %q\nwant  %q", calls.lines, wantLine)
	}
	want := fmt.Sprintf(TextModerationTimedOut, "2026-10-17T13:30:00Z")
	if in.defers != 1 || len(in.edits) != 1 || in.edits[0] != want {
		t.Errorf("defers = %d, edits = %q; want one defer and %q", in.defers, in.edits, want)
	}
}

// refusedCase is an interaction the handler refuses before any call, with the text it answers.
type refusedCase struct {
	name string
	run  func(h *ModerationHandler, in *fakeModInteraction, calls *fakeModCalls)
	in   *fakeModInteraction
	want string
}

func runRefused(t *testing.T, cases []refusedCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newModHandler(t)
			calls := &fakeModCalls{}
			c.run(h, c.in, calls)
			if got := c.in.answers(); len(got) != 1 || got[0] != c.want {
				t.Errorf("answers = %q, want only %q", got, c.want)
			}
			if n := calls.count(); n != 0 || len(c.in.forms) != 0 || c.in.defers != 0 {
				t.Errorf("%d calls, forms %q, %d defers; want none", n, c.in.forms, c.in.defers)
			}
		})
	}
}

func removeCommand(h *ModerationHandler, in *fakeModInteraction, _ *fakeModCalls) {
	h.HandleRemoveCommand(context.Background(), in, removeTarget())
}

func removeReason(reason string) func(*ModerationHandler, *fakeModInteraction, *fakeModCalls) {
	return func(h *ModerationHandler, in *fakeModInteraction, calls *fakeModCalls) {
		h.HandleRemoveReason(context.Background(), in, removeCustomID(), reason, calls)
	}
}

func timeout(minutes int, reason string) func(*ModerationHandler, *fakeModInteraction, *fakeModCalls) {
	return func(h *ModerationHandler, in *fakeModInteraction, calls *fakeModCalls) {
		h.HandleTimeout(context.Background(), in, TimeoutRequest{MemberID: modMemberID, Minutes: minutes, Reason: reason}, calls)
	}
}

func TestModeration_Negative_CallerWithoutTheRoleIsRefused(t *testing.T) {
	runRefused(t, []refusedCase{
		{"remove command", removeCommand, newModInteraction(modOtherRoleID), TextModerationNotModerator},
		{"remove reason", removeReason("Spam"), newModInteraction(), TextModerationNotModerator},
		{"timeout", timeout(10, "Spam"), newModInteraction(modOtherRoleID), TextModerationNotModerator},
	})
}

func TestModeration_Negative_MissingReasonIsRefused(t *testing.T) {
	runRefused(t, []refusedCase{
		{"remove blank", removeReason(" \n\t "), newModInteraction(modRoleID), TextModerationBadReason},
		{"remove empty", removeReason(""), newModInteraction(modRoleID), TextModerationBadReason},
		{"timeout blank", timeout(10, "   "), newModInteraction(modRoleID), TextModerationBadReason},
		{"timeout too long", timeout(10, strings.Repeat("ä", MaxReasonRunes+1)), newModInteraction(modRoleID), TextModerationBadReason},
		{"timeout invalid text", timeout(10, "spam \xff"), newModInteraction(modRoleID), TextModerationBadReason},
	})
}

func TestModeration_Negative_TimeoutOutsideTheLimitIsRefused(t *testing.T) {
	runRefused(t, []refusedCase{
		{"over 28 days", timeout(MaxTimeoutMinutes+1, "Spam"), newModInteraction(modRoleID), TextModerationBadMinutes},
		{"zero", timeout(0, "Spam"), newModInteraction(modRoleID), TextModerationBadMinutes},
		{"negative", timeout(-5, "Spam"), newModInteraction(modRoleID), TextModerationBadMinutes},
	})
}

func TestModeration_Negative_SenderAndFormAreChecked(t *testing.T) {
	other := newModInteraction(modRoleID)
	other.guildID = modGuildID + 1
	noMember := func(h *ModerationHandler, in *fakeModInteraction, calls *fakeModCalls) {
		h.HandleTimeout(context.Background(), in, TimeoutRequest{Minutes: 10, Reason: "Spam"}, calls)
	}
	badForm := func(id string) func(*ModerationHandler, *fakeModInteraction, *fakeModCalls) {
		return func(h *ModerationHandler, in *fakeModInteraction, calls *fakeModCalls) {
			h.HandleRemoveReason(context.Background(), in, id, "Spam", calls)
		}
	}
	runRefused(t, []refusedCase{
		{"other server", removeCommand, other, TextWrongGuild},
		{"no member", noMember, newModInteraction(modRoleID), TextModerationNoMember},
		{"form of another command", badForm("areas:interests"), newModInteraction(modRoleID), TextModerationBadForm},
		{"form with two ids", badForm("moderation-remove:1:2"), newModInteraction(modRoleID), TextModerationBadForm},
		{"form with four ids", badForm("moderation-remove:1:2:3:4"), newModInteraction(modRoleID), TextModerationBadForm},
		{"form with a zero id", badForm("moderation-remove:1:0:3"), newModInteraction(modRoleID), TextModerationBadForm},
		{"form with a word", badForm("moderation-remove:1:x:3"), newModInteraction(modRoleID), TextModerationBadForm},
	})
}

func TestModeration_Negative_ServerNotResolvedRefusesEveryCommand(t *testing.T) {
	for name, set := range map[string]func(h *ModerationHandler){
		"never read":  func(*ModerationHandler) {},
		"no role":     func(h *ModerationHandler) { h.SetServer(modTestRoles()[:1], modTestChannels(), modBotUserID) },
		"no trail":    func(h *ModerationHandler) { h.SetServer(modTestRoles(), modTestChannels()[:1], modBotUserID) },
		"no bot user": func(h *ModerationHandler) { h.SetServer(modTestRoles(), modTestChannels(), 0) },
		"trail held twice": func(h *ModerationHandler) {
			h.SetServer(modTestRoles(), append(modTestChannels(), modTextChannel(9, "mod-log")), modBotUserID)
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, err := NewModerationHandler(ModerationConfig{Definition: modTestDefinition(), GuildID: modGuildID})
			if err != nil {
				t.Fatalf("NewModerationHandler: %v", err)
			}
			set(h)
			in, calls := newModInteraction(modRoleID), &fakeModCalls{}
			h.HandleTimeout(context.Background(), in, TimeoutRequest{MemberID: modMemberID, Minutes: 10, Reason: "Spam"}, calls)
			if got := in.answers(); len(got) != 1 || got[0] != TextModerationUnavailable || calls.count() != 0 {
				t.Errorf("answers = %q after %d calls, want only %q", got, calls.count(), TextModerationUnavailable)
			}
		})
	}
}

func TestModeration_Negative_DiscordRefusesTheActAndNoLineIsWritten(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"refused":  {fmt.Errorf("modify guild member: %w", ErrDiscordRefused), TextModerationRefused},
		"gone":     {fmt.Errorf("modify guild member: %w", ErrDiscordGone), TextModerationGone},
		"failed":   {errors.New("connection reset"), TextModerationFailed},
		"deadline": {context.DeadlineExceeded, TextModerationFailed},
	} {
		t.Run(name, func(t *testing.T) {
			h := newModHandler(t)
			in := newModInteraction(modRoleID)
			calls := &fakeModCalls{timeoutErr: c.err, deleteErr: c.err}
			h.HandleTimeout(context.Background(), in, TimeoutRequest{MemberID: modMemberID, Minutes: 10, Reason: "Spam"}, calls)
			h.HandleRemoveReason(context.Background(), in, removeCustomID(), "Spam", calls)
			if len(calls.lines) != 0 {
				t.Errorf("lines = %q, want none", calls.lines)
			}
			if len(in.edits) != 2 || in.edits[0] != c.want || in.edits[1] != c.want {
				t.Errorf("edits = %q, want %q twice", in.edits, c.want)
			}
		})
	}
}

func TestModeration_Negative_LineThatFailsIsSaid(t *testing.T) {
	h := newModHandler(t)
	in := newModInteraction(modRoleID)
	calls := &fakeModCalls{postErr: errors.New("connection reset")}
	h.HandleTimeout(context.Background(), in, TimeoutRequest{MemberID: modMemberID, Minutes: 10, Reason: "Spam"}, calls)
	if len(calls.timeouts) != 1 || len(in.edits) != 1 || in.edits[0] != TextModerationNoLine {
		t.Errorf("timeouts = %q, edits = %q; want the act and %q", calls.timeouts, in.edits, TextModerationNoLine)
	}
}

func TestModeration_Boundary_TimeoutLimitsAndReasonLength(t *testing.T) {
	long := strings.Repeat("ä", MaxReasonRunes)
	for name, c := range map[string]struct {
		minutes   int
		reason    string
		wantUntil string
	}{
		"one minute":       {1, "x", "2026-10-17T12:01:00Z"},
		"28 days":          {MaxTimeoutMinutes, "x", "2026-11-14T12:00:00Z"},
		"512 characters":   {10, long, "2026-10-17T12:10:00Z"},
		"reason with tabs": {10, "a\tb", "2026-10-17T12:10:00Z"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newModHandler(t)
			in, calls := newModInteraction(modRoleID), &fakeModCalls{}
			h.HandleTimeout(context.Background(), in, TimeoutRequest{MemberID: modMemberID, Minutes: c.minutes, Reason: c.reason}, calls)
			want := fmt.Sprintf("%d %s %s", modMemberID, c.wantUntil, c.reason)
			if len(calls.timeouts) != 1 || calls.timeouts[0] != want || len(calls.lines) != 1 {
				t.Errorf("timeouts = %q, lines = %d; want %q and one line", calls.timeouts, len(calls.lines), want)
			}
		})
	}
}

func TestModeration_Boundary_LineHoldsTheReasonOnOneLine(t *testing.T) {
	h := newModHandler(t)
	in, calls := newModInteraction(modRoleID), &fakeModCalls{}
	h.HandleRemoveReason(context.Background(), in, removeCustomID(), "first\nsecond\r\nthird", calls)
	if len(calls.lines) != 1 || !strings.HasSuffix(calls.lines[0], "· reason: first second  third") {
		t.Errorf("lines = %q, want the reason on one line", calls.lines)
	}
	if len(calls.deletes) != 1 || !strings.HasSuffix(calls.deletes[0], " first\nsecond\r\nthird") {
		t.Errorf("deletes = %q, want the reason as given for the audit log", calls.deletes)
	}
}

func TestNewModerationHandler_Negative_ConfigurationIsChecked(t *testing.T) {
	noRole := modTestDefinition()
	noRole.Roles = noRole.Roles[:1]
	noTrail := modTestDefinition()
	noTrail.Channels[1].Trail = false
	for name, cfg := range map[string]ModerationConfig{
		"no definition":       {GuildID: modGuildID},
		"no guild":            {Definition: modTestDefinition()},
		"no moderator role":   {Definition: noRole, GuildID: modGuildID},
		"no trail channel":    {Definition: noTrail, GuildID: modGuildID},
		"retention too short": {Definition: modTestDefinition(), GuildID: modGuildID, Retention: MinTrailRetention - time.Second},
		"retention too long":  {Definition: modTestDefinition(), GuildID: modGuildID, Retention: MaxTrailRetention + time.Second},
		"negative timeout":    {Definition: modTestDefinition(), GuildID: modGuildID, CallTimeout: -time.Second},
	} {
		if _, err := NewModerationHandler(cfg); err == nil {
			t.Errorf("%s: NewModerationHandler succeeded, want an error", name)
		}
	}
}

func TestModerationConfig_Boundary_ConfigBoundsLieInsideTheHandlerBounds(t *testing.T) {
	if config.MinTrailRetention < MinTrailRetention || config.MaxTrailRetention > MaxTrailRetention {
		t.Errorf("config bounds [%v, %v] outside the handler bounds [%v, %v]",
			config.MinTrailRetention, config.MaxTrailRetention, MinTrailRetention, MaxTrailRetention)
	}
}
