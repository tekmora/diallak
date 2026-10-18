package discord

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/server"
)

// The server's ids of the roles in these tests. Honorary supporter and Supporter for a year are
// on the server too; the function must never touch them.
const (
	supRoleSupporter uint64 = 830000000000000001
	supRoleKoFi      uint64 = 830000000000000002
	supRolePatreon   uint64 = 830000000000000003
	supRoleHonorary  uint64 = 830000000000000004
	supRoleYear      uint64 = 830000000000000005
	supMemberA       uint64 = 840000000000000001
	supMemberB       uint64 = 840000000000000002
)

var supStart = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// supClock is a clock the test moves by hand.
type supClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *supClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *supClock) set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

// supCalls records the role changes and fails them while fail is set.
type supCalls struct {
	mu      sync.Mutex
	added   [][2]uint64
	removed [][2]uint64
	fail    bool
}

func (c *supCalls) AddRole(_ context.Context, memberID, roleID uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return errors.New("refused")
	}
	c.added = append(c.added, [2]uint64{memberID, roleID})
	return nil
}

func (c *supCalls) RemoveRole(_ context.Context, memberID, roleID uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return errors.New("refused")
	}
	c.removed = append(c.removed, [2]uint64{memberID, roleID})
	return nil
}

func supDefinition(t *testing.T) *serverdef.Definition {
	t.Helper()
	def, err := server.Definition()
	if err != nil {
		t.Fatalf("server.Definition(): %v", err)
	}
	return def
}

// supServerRoles are the server's roles by the names the definition gives them.
func supServerRoles(t *testing.T) []discord.Role {
	t.Helper()
	names := map[string]uint64{}
	for _, r := range supDefinition(t).Roles {
		switch r.ID {
		case "supporter":
			names[r.Name] = supRoleSupporter
		case "supporter-ko-fi":
			names[r.Name] = supRoleKoFi
		case "supporter-patreon":
			names[r.Name] = supRolePatreon
		case "honorary-supporter":
			names[r.Name] = supRoleHonorary
		case "supporter-12-months":
			names[r.Name] = supRoleYear
		}
	}
	roles := make([]discord.Role, 0, len(names))
	for name, id := range names {
		roles = append(roles, discord.Role{ID: snowflake.ID(id), Name: name})
	}
	return roles
}

func newTestSupporter(t *testing.T, clock *supClock) *SupporterHandler {
	t.Helper()
	h, err := NewSupporterHandler(SupporterConfig{Definition: supDefinition(t), GuildID: areasGuildID, Now: clock.Now})
	if err != nil {
		t.Fatalf("NewSupporterHandler: %v", err)
	}
	if err := h.SetRoles(supServerRoles(t)); err != nil {
		t.Fatalf("SetRoles: %v", err)
	}
	return h
}

func TestSupporter_Positive_PlatformRoleGivesSupporter(t *testing.T) {
	for name, platform := range map[string]uint64{"Ko-fi": supRoleKoFi, "Patreon": supRolePatreon} {
		t.Run(name, func(t *testing.T) {
			h := newTestSupporter(t, &supClock{now: supStart})
			calls := &supCalls{}
			h.Observe(context.Background(), SupporterMember{ID: supMemberA, RoleIDs: []uint64{platform}}, calls)
			if len(calls.added) != 1 || calls.added[0] != [2]uint64{supMemberA, supRoleSupporter} {
				t.Fatalf("added = %v, want Supporter for the member", calls.added)
			}
			if len(calls.removed) != 0 || h.Lapsed() != 0 {
				t.Fatalf("removed = %v, lapsed = %d; want nothing taken and no grace period", calls.removed, h.Lapsed())
			}
		})
	}
}

// ADR-0024: a member who has held neither platform role for the grace period loses Supporter, and
// only Supporter.
func TestSupporter_Positive_SupporterTakenAfterTheGracePeriod(t *testing.T) {
	clock := &supClock{now: supStart}
	h := newTestSupporter(t, clock)
	calls := &supCalls{}
	h.Observe(context.Background(), SupporterMember{ID: supMemberA,
		RoleIDs: []uint64{supRoleSupporter, supRoleHonorary, supRoleYear}}, calls)
	clock.set(supStart.Add(DefaultSupporterGrace))
	if n := h.RemoveLapsed(context.Background(), calls); n != 1 {
		t.Fatalf("RemoveLapsed = %d, want 1", n)
	}
	if len(calls.removed) != 1 || calls.removed[0] != [2]uint64{supMemberA, supRoleSupporter} {
		t.Fatalf("removed = %v, want only Supporter of the member", calls.removed)
	}
	if len(calls.added) != 0 || h.Lapsed() != 0 {
		t.Fatalf("added = %v, lapsed = %d; want nothing given and the grace period ended", calls.added, h.Lapsed())
	}
}

// Boundary: one nanosecond before the grace period ends nothing is taken; at its end it is.
func TestSupporter_Boundary_NothingTakenBeforeTheGracePeriodEnds(t *testing.T) {
	clock := &supClock{now: supStart}
	h := newTestSupporter(t, clock)
	calls := &supCalls{}
	h.Observe(context.Background(), SupporterMember{ID: supMemberA, RoleIDs: []uint64{supRoleSupporter}}, calls)
	for _, at := range []time.Duration{0, time.Hour, DefaultSupporterGrace - time.Nanosecond} {
		clock.set(supStart.Add(at))
		if n := h.RemoveLapsed(context.Background(), calls); n != 0 {
			t.Fatalf("RemoveLapsed at %v = %d, want 0 before the grace period ends", at, n)
		}
	}
	// A second event while the period runs does not start it again.
	h.Observe(context.Background(), SupporterMember{ID: supMemberA, RoleIDs: []uint64{supRoleSupporter}}, calls)
	clock.set(supStart.Add(DefaultSupporterGrace))
	if n := h.RemoveLapsed(context.Background(), calls); n != 1 {
		t.Fatalf("RemoveLapsed at the end = %d, want 1", n)
	}
}

func TestSupporter_Boundary_ConfiguredGraceIsHonoured(t *testing.T) {
	clock := &supClock{now: supStart}
	h, err := NewSupporterHandler(SupporterConfig{
		Definition: supDefinition(t), GuildID: areasGuildID, Grace: MinSupporterGrace, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("NewSupporterHandler: %v", err)
	}
	if err := h.SetRoles(supServerRoles(t)); err != nil {
		t.Fatalf("SetRoles: %v", err)
	}
	calls := &supCalls{}
	h.Observe(context.Background(), SupporterMember{ID: supMemberA, RoleIDs: []uint64{supRoleSupporter}}, calls)
	clock.set(supStart.Add(MinSupporterGrace - time.Nanosecond))
	if n := h.RemoveLapsed(context.Background(), calls); n != 0 {
		t.Fatalf("RemoveLapsed before one day = %d, want 0", n)
	}
	clock.set(supStart.Add(MinSupporterGrace))
	if n := h.RemoveLapsed(context.Background(), calls); n != 1 {
		t.Fatalf("RemoveLapsed after one day = %d, want 1", n)
	}
}

// A platform role that comes back during the grace period ends it; a member who leaves is dropped.
func TestSupporter_Negative_ReturnAndLeaveEndTheGracePeriod(t *testing.T) {
	clock := &supClock{now: supStart}
	h := newTestSupporter(t, clock)
	calls := &supCalls{}
	ctx := context.Background()
	h.Observe(ctx, SupporterMember{ID: supMemberA, RoleIDs: []uint64{supRoleSupporter}}, calls)
	h.Observe(ctx, SupporterMember{ID: supMemberB, RoleIDs: []uint64{supRoleSupporter}}, calls)
	h.Observe(ctx, SupporterMember{ID: supMemberA, RoleIDs: []uint64{supRoleSupporter, supRolePatreon}}, calls)
	h.Forget(supMemberB)
	clock.set(supStart.Add(MaxSupporterGrace))
	if n := h.RemoveLapsed(ctx, calls); n != 0 || len(calls.removed) != 0 {
		t.Fatalf("RemoveLapsed = %d, removed = %v; want nothing taken", n, calls.removed)
	}
	if len(calls.added) != 0 {
		t.Fatalf("added = %v, want nothing given to a member who holds Supporter", calls.added)
	}
}

// Members who hold none of the roles, or only roles given by hand, are left alone.
func TestSupporter_Negative_OtherMembersAreLeftAlone(t *testing.T) {
	clock := &supClock{now: supStart}
	h := newTestSupporter(t, clock)
	calls := &supCalls{}
	for _, roles := range [][]uint64{nil, {supRoleHonorary}, {supRoleYear}, {supRoleHonorary, supRoleYear}} {
		h.Observe(context.Background(), SupporterMember{ID: supMemberA, RoleIDs: roles}, calls)
	}
	clock.set(supStart.Add(MaxSupporterGrace))
	h.RemoveLapsed(context.Background(), calls)
	if len(calls.added)+len(calls.removed) != 0 || h.Lapsed() != 0 {
		t.Fatalf("added = %v, removed = %v, lapsed = %d; want no change", calls.added, calls.removed, h.Lapsed())
	}
}

// A failed removal keeps the member for the next check; a failed grant is retried by the next event.
func TestSupporter_Negative_FailedRemovalIsRetried(t *testing.T) {
	clock := &supClock{now: supStart}
	h := newTestSupporter(t, clock)
	calls := &supCalls{fail: true}
	h.Observe(context.Background(), SupporterMember{ID: supMemberA, RoleIDs: []uint64{supRoleSupporter}}, calls)
	clock.set(supStart.Add(DefaultSupporterGrace))
	if n := h.RemoveLapsed(context.Background(), calls); n != 0 || h.Lapsed() != 1 {
		t.Fatalf("RemoveLapsed = %d, lapsed = %d; want 0 and the member kept", n, h.Lapsed())
	}
	calls.fail = false
	if n := h.RemoveLapsed(context.Background(), calls); n != 1 {
		t.Fatalf("RemoveLapsed after the failure = %d, want 1", n)
	}
}

// Before the server's roles are read the function changes nothing.
func TestSupporter_Negative_NothingBeforeTheRolesAreRead(t *testing.T) {
	h, err := NewSupporterHandler(SupporterConfig{Definition: supDefinition(t), GuildID: areasGuildID})
	if err != nil {
		t.Fatalf("NewSupporterHandler: %v", err)
	}
	calls := &supCalls{}
	h.Observe(context.Background(), SupporterMember{ID: supMemberA, RoleIDs: []uint64{supRoleKoFi}}, calls)
	if n := h.RemoveLapsed(context.Background(), calls); n != 0 || len(calls.added) != 0 {
		t.Fatalf("RemoveLapsed = %d, added = %v; want no change", n, calls.added)
	}
}

func TestSupporter_Negative_MissingOrDoubleRoleStopsTheStart(t *testing.T) {
	h, err := NewSupporterHandler(SupporterConfig{Definition: supDefinition(t), GuildID: areasGuildID})
	if err != nil {
		t.Fatalf("NewSupporterHandler: %v", err)
	}
	roles := supServerRoles(t)
	for i, r := range roles {
		if uint64(r.ID) == supRolePatreon {
			missing := append(append([]discord.Role(nil), roles[:i]...), roles[i+1:]...)
			if err := h.SetRoles(missing); err == nil || !strings.Contains(err.Error(), "not found") {
				t.Errorf("SetRoles without the Patreon role = %v, want not found", err)
			}
			double := append(append([]discord.Role(nil), roles...), discord.Role{ID: 1, Name: r.Name})
			if err := h.SetRoles(double); err == nil || !strings.Contains(err.Error(), "more than once") {
				t.Errorf("SetRoles with the Patreon role twice = %v, want held more than once", err)
			}
		}
	}
}

func TestNewSupporterHandler_Negative_InvalidConfig(t *testing.T) {
	def := supDefinition(t)
	cases := map[string]SupporterConfig{
		"no definition":     {GuildID: areasGuildID},
		"no guild":          {Definition: def},
		"grace too short":   {Definition: def, GuildID: areasGuildID, Grace: MinSupporterGrace - time.Nanosecond},
		"grace too long":    {Definition: def, GuildID: areasGuildID, Grace: MaxSupporterGrace + time.Nanosecond},
		"negative timeout":  {Definition: def, GuildID: areasGuildID, CallTimeout: -time.Second},
		"timeout too long":  {Definition: def, GuildID: areasGuildID, CallTimeout: maxSupporterCallTimeout + time.Second},
		"no supporter role": {Definition: &serverdef.Definition{}, GuildID: areasGuildID},
	}
	for name, cfg := range cases {
		if _, err := NewSupporterHandler(cfg); err == nil {
			t.Errorf("%s: NewSupporterHandler = nil error, want an error", name)
		}
	}
	h, err := NewSupporterHandler(SupporterConfig{Definition: def, GuildID: areasGuildID})
	if err != nil || h.Grace() != DefaultSupporterGrace {
		t.Fatalf("NewSupporterHandler default = %v, %v; want grace %v", h, err, DefaultSupporterGrace)
	}
}

// The configuration accepts only grace periods the function accepts.
func TestSupporterGrace_Boundary_ConfigBoundsWithinTheFunction(t *testing.T) {
	if config.MinSupporterGrace < MinSupporterGrace || config.MaxSupporterGrace > MaxSupporterGrace {
		t.Fatalf("config bounds [%v, %v] outside the function's [%v, %v]",
			config.MinSupporterGrace, config.MaxSupporterGrace, MinSupporterGrace, MaxSupporterGrace)
	}
}
