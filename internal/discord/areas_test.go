package discord

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/serverdef"
)

// The tests of the areas handler use a fake interaction and fake role calls. The server holds the
// roles of areasTestRoles; the definition asks two questions: interests (several answers) and a
// language (one answer).

const (
	areasGuildID  uint64 = 810000000000000001
	areasMemberID uint64 = 820000000000000001

	roleHomeLab uint64 = 830000000000000001
	roleMedia   uint64 = 830000000000000002
	roleGerman  uint64 = 830000000000000003
	roleFrench  uint64 = 830000000000000004
	roleStaff   uint64 = 830000000000000005
)

func areasTestDefinition() *serverdef.Definition {
	return &serverdef.Definition{
		Roles: []serverdef.Role{
			{ID: "interest-home-lab", Name: "Home lab"},
			{ID: "interest-media", Name: "Media"},
			{ID: "lang-de", Name: "German"},
			{ID: "lang-fr", Name: "French"},
			{ID: "staff", Name: "Staff"},
		},
		Choices: serverdef.Choices{Questions: []serverdef.Question{
			{ID: "interests", Title: "Your interests", Multiple: true, Options: []serverdef.Option{
				{Title: "Home lab", Role: "interest-home-lab"},
				{Title: "Media", Role: "interest-media"},
			}},
			{ID: "language", Title: "Your language", Options: []serverdef.Option{
				{Title: "German", Role: "lang-de"},
				{Title: "French", Role: "lang-fr"},
			}},
		}},
	}
}

func areasTestRoles() []discord.Role {
	return []discord.Role{
		{ID: snowflake.ID(areasGuildID), Name: "@everyone"},
		{ID: snowflake.ID(roleHomeLab), Name: "Home lab"},
		{ID: snowflake.ID(roleMedia), Name: "Media"},
		{ID: snowflake.ID(roleGerman), Name: "German"},
		{ID: snowflake.ID(roleFrench), Name: "French"},
		{ID: snowflake.ID(roleStaff), Name: "Staff"},
	}
}

// fakeAreasInteraction records what the handler answers.
type fakeAreasInteraction struct {
	guildID  uint64
	memberID uint64
	roles    []uint64
	pending  bool
	ackErr   error

	mu    sync.Mutex
	shows []AreasView
	edits []AreasView
	acks  int
}

func newAreasInteraction(roles ...uint64) *fakeAreasInteraction {
	return &fakeAreasInteraction{guildID: areasGuildID, memberID: areasMemberID, roles: roles}
}

func (f *fakeAreasInteraction) GuildID() uint64         { return f.guildID }
func (f *fakeAreasInteraction) MemberID() uint64        { return f.memberID }
func (f *fakeAreasInteraction) MemberRoleIDs() []uint64 { return f.roles }
func (f *fakeAreasInteraction) Pending() bool           { return f.pending }

func (f *fakeAreasInteraction) Show(_ context.Context, v AreasView) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shows = append(f.shows, v)
	return nil
}

func (f *fakeAreasInteraction) Acknowledge(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acks++
	return f.ackErr
}

func (f *fakeAreasInteraction) Edit(_ context.Context, v AreasView) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edits = append(f.edits, v)
	return nil
}

// fakeRoleCalls is the server side of a change.
type fakeRoleCalls struct {
	roles   []discord.Role
	readErr error
	failOn  map[uint64]bool

	mu      sync.Mutex
	reads   int
	added   []uint64
	removed []uint64
}

func (c *fakeRoleCalls) ReadRoles(_ context.Context) ([]discord.Role, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	return c.roles, c.readErr
}

func (c *fakeRoleCalls) AddRole(_ context.Context, memberID, roleID uint64) error {
	return c.record(&c.added, memberID, roleID)
}

func (c *fakeRoleCalls) RemoveRole(_ context.Context, memberID, roleID uint64) error {
	return c.record(&c.removed, memberID, roleID)
}

func (c *fakeRoleCalls) record(list *[]uint64, memberID, roleID uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	*list = append(*list, roleID)
	if memberID != areasMemberID {
		return fmt.Errorf("call for member %d, want %d", memberID, areasMemberID)
	}
	if c.failOn[roleID] {
		return errors.New("refused by the fake server")
	}
	return nil
}

func (c *fakeRoleCalls) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads + len(c.added) + len(c.removed)
}

func newTestAreasHandler(t *testing.T, def *serverdef.Definition, logs *bytes.Buffer) *AreasHandler {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	if logs != nil {
		logger = slog.New(slog.NewJSONHandler(logs, nil))
	}
	h, err := NewAreasHandler(AreasConfig{Definition: def, GuildID: areasGuildID, Logger: logger})
	if err != nil {
		t.Fatalf("NewAreasHandler: %v", err)
	}
	h.SetRoles(areasTestRoles())
	return h
}

// menuDefaults returns, per menu, the values of the selected options.
func menuDefaults(v AreasView) map[string][]string {
	out := make(map[string][]string, len(v.Menus))
	for _, m := range v.Menus {
		out[m.CustomID] = []string{}
		for _, o := range m.Options {
			if o.Default {
				out[m.CustomID] = append(out[m.CustomID], o.Value)
			}
		}
	}
	return out
}

func onlyEdit(t *testing.T, in *fakeAreasInteraction) AreasView {
	t.Helper()
	if in.acks != 1 || len(in.edits) != 1 || len(in.shows) != 0 {
		t.Fatalf("acknowledged %d, edited %d, shown %d times; want one acknowledgement and one edit",
			in.acks, len(in.edits), len(in.shows))
	}
	return in.edits[0]
}

func onlyShow(t *testing.T, in *fakeAreasInteraction) AreasView {
	t.Helper()
	if in.acks != 0 || len(in.edits) != 0 || len(in.shows) != 1 {
		t.Fatalf("acknowledged %d, edited %d, shown %d times; want one answer and nothing else",
			in.acks, len(in.edits), len(in.shows))
	}
	return in.shows[0]
}

func TestAreasCommand_Positive_ShowsEveryMenuWithTheHeldRolesSelected(t *testing.T) {
	h := newTestAreasHandler(t, areasTestDefinition(), nil)
	in := newAreasInteraction(roleMedia, roleStaff)

	h.HandleCommand(context.Background(), in)

	v := onlyShow(t, in)
	if v.Text != TextAreasIntro {
		t.Errorf("text = %q, want the intro", v.Text)
	}
	if len(v.Menus) != 2 {
		t.Fatalf("menus = %+v, want interests and language", v.Menus)
	}
	if v.Menus[0].CustomID != "areas:interests" || v.Menus[0].MaxValues != 2 || len(v.Menus[0].Options) != 2 {
		t.Errorf("first menu = %+v, want areas:interests with two options and two values", v.Menus[0])
	}
	if v.Menus[1].CustomID != "areas:language" || v.Menus[1].MaxValues != 1 {
		t.Errorf("second menu = %+v, want areas:language with one value", v.Menus[1])
	}
	got := menuDefaults(v)
	if fmt.Sprint(got["areas:interests"]) != "[interest-media]" || len(got["areas:language"]) != 0 {
		t.Errorf("selected = %v, want only interest-media", got)
	}
}

func TestAreasChange_Positive_GivesAndTakesOnlyTheDifference(t *testing.T) {
	var logs bytes.Buffer
	h := newTestAreasHandler(t, areasTestDefinition(), &logs)
	in := newAreasInteraction(roleHomeLab, roleGerman)
	calls := &fakeRoleCalls{roles: areasTestRoles()}

	h.HandleChange(context.Background(), in, "areas:interests", []string{"interest-media"}, calls)

	v := onlyEdit(t, in)
	if fmt.Sprint(calls.added) != fmt.Sprint([]uint64{roleMedia}) ||
		fmt.Sprint(calls.removed) != fmt.Sprint([]uint64{roleHomeLab}) {
		t.Errorf("added %v, removed %v; want Media given and Home lab taken", calls.added, calls.removed)
	}
	if v.Text != "Given: Media.\nTaken: Home lab." {
		t.Errorf("status = %q", v.Text)
	}
	got := menuDefaults(v)
	if fmt.Sprint(got["areas:interests"]) != "[interest-media]" || fmt.Sprint(got["areas:language"]) != "[lang-de]" {
		t.Errorf("selected after the change = %v", got)
	}
	out := logs.String()
	if !strings.Contains(out, `"role":"interest-media","action":"given","outcome":"ok"`) ||
		!strings.Contains(out, `"role":"interest-home-lab","action":"taken","outcome":"ok"`) {
		t.Errorf("logs lack role, action and outcome: %s", out)
	}
	if strings.Contains(out, strconv.FormatUint(areasMemberID, 10)) {
		t.Errorf("logs hold the member id: %s", out)
	}
}

func TestAreasChange_Negative_ChoiceOutsideTheMenuIsRefusedWithoutAnyCall(t *testing.T) {
	cases := []struct {
		name     string
		customID string
		values   []string
	}{
		{"role outside the choices", "areas:interests", []string{"staff"}},
		{"option of another menu", "areas:interests", []string{"lang-de"}},
		{"option twice", "areas:interests", []string{"interest-media", "interest-media"}},
		{"two answers to a one-answer question", "areas:language", []string{"lang-de", "lang-fr"}},
		{"menu that does not exist", "areas:staff", []string{"staff"}},
		{"custom id of another component", "interests", []string{"interest-media"}},
		{"one valid option next to a role outside", "areas:interests", []string{"interest-media", "staff"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			h := newTestAreasHandler(t, areasTestDefinition(), &logs)
			in := newAreasInteraction()
			calls := &fakeRoleCalls{roles: areasTestRoles()}

			h.HandleChange(context.Background(), in, tc.customID, tc.values, calls)

			if v := onlyShow(t, in); v.Text != TextAreasRefused || len(v.Menus) != 0 {
				t.Errorf("answer = %+v, want the refusal", v)
			}
			if n := calls.callCount(); n != 0 {
				t.Errorf("%d calls to the server, want none", n)
			}
			if !strings.Contains(logs.String(), `"reason":"not-an-option"`) {
				t.Errorf("logs lack the refusal: %s", logs.String())
			}
		})
	}
}

func TestAreas_Negative_OtherGuildPendingAndMissingMemberAreRefused(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*fakeAreasInteraction)
		want   string
	}{
		{"other guild", func(in *fakeAreasInteraction) { in.guildID = areasGuildID + 1 }, TextWrongGuild},
		{"no guild", func(in *fakeAreasInteraction) { in.guildID = 0 }, TextWrongGuild},
		{"pending screening", func(in *fakeAreasInteraction) { in.pending = true }, TextAreasPending},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestAreasHandler(t, areasTestDefinition(), nil)
			cmd := newAreasInteraction()
			tc.modify(cmd)
			h.HandleCommand(context.Background(), cmd)
			if v := onlyShow(t, cmd); v.Text != tc.want || len(v.Menus) != 0 {
				t.Errorf("command answer = %+v, want %q without menus", v, tc.want)
			}

			change := newAreasInteraction()
			tc.modify(change)
			calls := &fakeRoleCalls{roles: areasTestRoles()}
			h.HandleChange(context.Background(), change, "areas:interests", []string{"interest-media"}, calls)
			if v := onlyShow(t, change); v.Text != tc.want {
				t.Errorf("change answer = %q, want %q", v.Text, tc.want)
			}
			if n := calls.callCount(); n != 0 {
				t.Errorf("%d calls to the server, want none", n)
			}
		})
	}

	t.Run("change without a member", func(t *testing.T) {
		h := newTestAreasHandler(t, areasTestDefinition(), nil)
		in := newAreasInteraction()
		in.memberID = 0
		calls := &fakeRoleCalls{roles: areasTestRoles()}
		h.HandleChange(context.Background(), in, "areas:interests", []string{"interest-media"}, calls)
		if v := onlyShow(t, in); v.Text != TextAreasRefused {
			t.Errorf("answer = %q, want the refusal", v.Text)
		}
		if n := calls.callCount(); n != 0 {
			t.Errorf("%d calls to the server, want none", n)
		}
	})
}

func TestAreasChange_Negative_FailedCallIsReportedAndNotRepeated(t *testing.T) {
	var logs bytes.Buffer
	h := newTestAreasHandler(t, areasTestDefinition(), &logs)
	in := newAreasInteraction()
	calls := &fakeRoleCalls{roles: areasTestRoles(), failOn: map[uint64]bool{roleMedia: true}}

	h.HandleChange(context.Background(), in, "areas:interests", []string{"interest-home-lab", "interest-media"}, calls)

	v := onlyEdit(t, in)
	if fmt.Sprint(calls.added) != fmt.Sprint([]uint64{roleHomeLab, roleMedia}) {
		t.Errorf("added %v, want one call for each role", calls.added)
	}
	if v.Text != "Given: Home lab.\nNot applied: Media. Run /areas again to see what you hold." {
		t.Errorf("status = %q", v.Text)
	}
	if got := menuDefaults(v); fmt.Sprint(got["areas:interests"]) != "[interest-home-lab]" {
		t.Errorf("selected = %v, want only the role that was given", got)
	}
	if !strings.Contains(logs.String(), `"role":"interest-media","action":"given","outcome":"failure"`) {
		t.Errorf("logs lack the failure: %s", logs.String())
	}
}

func TestAreasChange_Negative_UnreadRolesApplyNothing(t *testing.T) {
	h := newTestAreasHandler(t, areasTestDefinition(), nil)
	in := newAreasInteraction()
	calls := &fakeRoleCalls{readErr: errors.New("read refused")}

	h.HandleChange(context.Background(), in, "areas:interests", []string{"interest-media"}, calls)

	if v := onlyEdit(t, in); v.Text != TextAreasUnread || len(v.Menus) != 0 {
		t.Errorf("edit = %+v, want the unread text", v)
	}
	if len(calls.added)+len(calls.removed) != 0 {
		t.Errorf("added %v, removed %v; want no role call", calls.added, calls.removed)
	}
}

func TestAreasChange_Negative_FailedAcknowledgementMakesNoCall(t *testing.T) {
	h := newTestAreasHandler(t, areasTestDefinition(), nil)
	in := newAreasInteraction()
	in.ackErr = errors.New("interaction expired")
	calls := &fakeRoleCalls{roles: areasTestRoles()}

	h.HandleChange(context.Background(), in, "areas:interests", []string{"interest-media"}, calls)

	if n := calls.callCount(); n != 0 {
		t.Errorf("%d calls after a failed acknowledgement, want none", n)
	}
	if len(in.edits) != 0 {
		t.Errorf("edited %d times after a failed acknowledgement, want none", len(in.edits))
	}
}

func TestAreas_Boundary_MemberHoldingEveryOption(t *testing.T) {
	held := []uint64{roleHomeLab, roleMedia, roleGerman}
	h := newTestAreasHandler(t, areasTestDefinition(), nil)

	cmd := newAreasInteraction(held...)
	h.HandleCommand(context.Background(), cmd)
	if got := menuDefaults(onlyShow(t, cmd)); fmt.Sprint(got["areas:interests"]) != "[interest-home-lab interest-media]" {
		t.Errorf("selected = %v, want both interests", got)
	}

	same := newAreasInteraction(held...)
	calls := &fakeRoleCalls{roles: areasTestRoles()}
	h.HandleChange(context.Background(), same, "areas:interests", []string{"interest-media", "interest-home-lab"}, calls)
	if v := onlyEdit(t, same); v.Text != textAreasNoChange {
		t.Errorf("status = %q, want %q", v.Text, textAreasNoChange)
	}
	if len(calls.added)+len(calls.removed) != 0 {
		t.Errorf("added %v, removed %v; want no role call for an unchanged choice", calls.added, calls.removed)
	}

	empty := newAreasInteraction(held...)
	calls = &fakeRoleCalls{roles: areasTestRoles()}
	h.HandleChange(context.Background(), empty, "areas:interests", nil, calls)
	if v := onlyEdit(t, empty); v.Text != "Taken: Home lab, Media." {
		t.Errorf("status = %q", v.Text)
	}
	if fmt.Sprint(calls.removed) != fmt.Sprint([]uint64{roleHomeLab, roleMedia}) || len(calls.added) != 0 {
		t.Errorf("added %v, removed %v; want both interests taken, the language kept", calls.added, calls.removed)
	}
}

func TestAreas_Boundary_TwentyFiveOptionsTakeTwentyFiveValuesAndRefuseOneMore(t *testing.T) {
	// The definition admits at most 25 options per question, the limit of a Discord select menu
	// (internal/serverdef/validate_choices.go).
	def := &serverdef.Definition{Choices: serverdef.Choices{Questions: []serverdef.Question{
		{ID: "many", Title: "Many", Multiple: true},
	}}}
	roles := []discord.Role{}
	values := []string{}
	for i := range 25 {
		id := fmt.Sprintf("role-%02d", i)
		def.Roles = append(def.Roles, serverdef.Role{ID: id, Name: "Role " + id})
		def.Choices.Questions[0].Options = append(def.Choices.Questions[0].Options,
			serverdef.Option{Title: id, Role: id})
		roles = append(roles, discord.Role{ID: snowflake.ID(900 + i), Name: "Role " + id})
		values = append(values, id)
	}
	h, err := NewAreasHandler(AreasConfig{Definition: def, GuildID: areasGuildID})
	if err != nil {
		t.Fatalf("NewAreasHandler: %v", err)
	}
	h.SetRoles(roles)

	in := newAreasInteraction()
	calls := &fakeRoleCalls{roles: roles}
	h.HandleChange(context.Background(), in, "areas:many", values, calls)
	v := onlyEdit(t, in)
	if len(calls.added) != 25 || len(v.Menus) != 1 || v.Menus[0].MaxValues != 25 {
		t.Errorf("added %d roles, menus %d, max values %v; want 25 roles given in one menu of 25 values",
			len(calls.added), len(v.Menus), v.Menus)
	}

	over := newAreasInteraction()
	calls = &fakeRoleCalls{roles: roles}
	h.HandleChange(context.Background(), over, "areas:many", append(values, "role-00"), calls)
	if onlyShow(t, over).Text != TextAreasRefused || calls.callCount() != 0 {
		t.Errorf("26 values were not refused before any call")
	}
}

func TestAreas_Boundary_MissingOrDuplicatedRoleLeavesItsOptionOut(t *testing.T) {
	h := newTestAreasHandler(t, areasTestDefinition(), nil)
	roles := []discord.Role{
		{ID: snowflake.ID(roleHomeLab), Name: "Home lab"},
		{ID: snowflake.ID(roleGerman), Name: "German"},
		{ID: snowflake.ID(roleFrench), Name: "French"},
		{ID: snowflake.ID(roleStaff), Name: "French"},
	}
	h.SetRoles(roles)

	cmd := newAreasInteraction()
	h.HandleCommand(context.Background(), cmd)
	v := onlyShow(t, cmd)
	if len(v.Menus) != 2 || len(v.Menus[0].Options) != 1 || v.Menus[0].MaxValues != 1 ||
		len(v.Menus[1].Options) != 1 || v.Menus[1].Options[0].Value != "lang-de" {
		t.Errorf("menus = %+v, want Media and French left out", v.Menus)
	}

	in := newAreasInteraction()
	calls := &fakeRoleCalls{roles: roles}
	h.HandleChange(context.Background(), in, "areas:interests", []string{"interest-media"}, calls)
	if got := onlyEdit(t, in).Text; got != "Not applied: Media. Run /areas again to see what you hold." {
		t.Errorf("status = %q", got)
	}
	if len(calls.added)+len(calls.removed) != 0 {
		t.Errorf("added %v, removed %v; want no call for a role the server lacks", calls.added, calls.removed)
	}

	h.SetRoles(nil)
	none := newAreasInteraction()
	h.HandleCommand(context.Background(), none)
	if got := onlyShow(t, none); got.Text != TextAreasNone || len(got.Menus) != 0 {
		t.Errorf("answer without any role = %+v, want %q", got, TextAreasNone)
	}
}

func TestNewAreasHandler_RefusesAnIncompleteConfig(t *testing.T) {
	def := areasTestDefinition()
	cases := []struct {
		name string
		cfg  AreasConfig
		ok   bool
	}{
		{"complete", AreasConfig{Definition: def, GuildID: areasGuildID}, true},
		{"longest timeout", AreasConfig{Definition: def, GuildID: areasGuildID, CallTimeout: maxAreasCallTimeout}, true},
		{"no definition", AreasConfig{GuildID: areasGuildID}, false},
		{"no guild", AreasConfig{Definition: def}, false},
		{"timeout over the maximum", AreasConfig{Definition: def, GuildID: areasGuildID,
			CallTimeout: maxAreasCallTimeout + time.Nanosecond}, false},
		{"negative timeout", AreasConfig{Definition: def, GuildID: areasGuildID, CallTimeout: -time.Second}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAreasHandler(tc.cfg)
			if (err == nil) != tc.ok {
				t.Errorf("err = %v, want ok %v", err, tc.ok)
			}
		})
	}
}
