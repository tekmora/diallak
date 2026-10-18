package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"

	"github.com/tekmora/diallak/internal/serverdef"
)

// The roles of the supporter function (ADR-0024): the bot gives the first and follows the two
// platform roles, which the funding integrations give and take.
const (
	supporterRole        = "supporter"
	supporterKoFiRole    = "supporter-ko-fi"
	supporterPatreonRole = "supporter-patreon"

	supporterAuditReason = "Supporter follows the funding platform roles (ADR-0024)"
)

const (
	// DefaultSupporterGrace is how long a member keeps Supporter after holding neither platform
	// role. It is chosen, not measured: Patreon syncs once a day, and three days also cover a
	// payment that is retried (ADR-0024).
	DefaultSupporterGrace = 72 * time.Hour
	// MinSupporterGrace is one day, Patreon's sync interval (ADR-0024).
	MinSupporterGrace = 24 * time.Hour
	// MaxSupporterGrace is the longest grace period accepted.
	MaxSupporterGrace = 90 * 24 * time.Hour

	// maxLapsedMembers bounds the members held in memory with a running grace period. A member
	// beyond it keeps Supporter until a later event or restart finds room: late, never early.
	maxLapsedMembers = 100_000
	// defaultSupporterCallTimeout is chosen, not measured.
	defaultSupporterCallTimeout = 10 * time.Second
	maxSupporterCallTimeout     = 2 * time.Minute
)

// SupporterMember is what the function reads of a member: the id and the role ids, nothing else
// (ADR-0027).
type SupporterMember struct {
	ID      uint64
	RoleIDs []uint64
}

// SupporterCalls are the calls the function makes against the server.
type SupporterCalls interface {
	AddRole(ctx context.Context, memberID, roleID uint64) error
	RemoveRole(ctx context.Context, memberID, roleID uint64) error
}

// SupporterConfig configures a SupporterHandler.
type SupporterConfig struct {
	Definition *serverdef.Definition
	GuildID    uint64
	// Grace is how long a member keeps Supporter after holding neither platform role. Zero means
	// DefaultSupporterGrace.
	Grace  time.Duration
	Logger *slog.Logger
	// CallTimeout is the deadline of each call to Discord. Zero means the default.
	CallTimeout time.Duration
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
}

// supporterRoleIDs are the server's ids of the three roles the function reads.
type supporterRoleIDs struct {
	supporter, koFi, patreon uint64
}

// SupporterHandler gives Supporter to a member who holds a platform role and takes it once the
// member has held neither for the grace period (ADR-0024). It stores nothing: the time a member
// was first seen without a platform role lives in memory only, and a restart starts it again.
type SupporterHandler struct {
	names       [3]string
	grace       time.Duration
	callTimeout time.Duration
	logger      *slog.Logger
	now         func() time.Time

	mu     sync.Mutex
	roles  *supporterRoleIDs
	lapsed map[uint64]time.Time
}

// NewSupporterHandler resolves the names of the three roles from the definition.
func NewSupporterHandler(cfg SupporterConfig) (*SupporterHandler, error) {
	if err := checkSupporterConfig(cfg); err != nil {
		return nil, err
	}
	names, err := supporterRoleNames(cfg.Definition)
	if err != nil {
		return nil, err
	}
	h := &SupporterHandler{
		names: names, grace: cfg.Grace, callTimeout: cfg.CallTimeout, logger: cfg.Logger, now: cfg.Now,
		lapsed: make(map[uint64]time.Time),
	}
	if h.grace == 0 {
		h.grace = DefaultSupporterGrace
	}
	if h.callTimeout == 0 {
		h.callTimeout = defaultSupporterCallTimeout
	}
	if h.logger == nil {
		h.logger = slog.New(slog.DiscardHandler)
	}
	if h.now == nil {
		h.now = time.Now
	}
	return h, nil
}

func checkSupporterConfig(cfg SupporterConfig) error {
	if cfg.Definition == nil {
		return errors.New("definition is required")
	}
	if cfg.GuildID == 0 {
		return errors.New("guild ID must not be zero")
	}
	if cfg.Grace != 0 && (cfg.Grace < MinSupporterGrace || cfg.Grace > MaxSupporterGrace) {
		return fmt.Errorf("grace %v out of range [%v, %v]", cfg.Grace, MinSupporterGrace, MaxSupporterGrace)
	}
	if cfg.CallTimeout < 0 || cfg.CallTimeout > maxSupporterCallTimeout {
		return fmt.Errorf("call timeout %v out of range [0, %v]", cfg.CallTimeout, maxSupporterCallTimeout)
	}
	return nil
}

// supporterRoleNames returns the server names of supporter, supporter-ko-fi and supporter-patreon.
func supporterRoleNames(def *serverdef.Definition) ([3]string, error) {
	ids := [3]string{supporterRole, supporterKoFiRole, supporterPatreonRole}
	var names [3]string
	for i, id := range ids {
		for _, r := range def.Roles {
			if r.ID == id {
				names[i] = r.Name
			}
		}
		if names[i] == "" {
			return names, fmt.Errorf("definition has no role %q", id)
		}
	}
	return names, nil
}

// Grace is the configured grace period.
func (h *SupporterHandler) Grace() time.Duration { return h.grace }

// SetRoles takes the server's roles and finds the three by name. A role that is missing or held
// more than once stops the start: the function never guesses which role a member holds.
func (h *SupporterHandler) SetRoles(roles []discord.Role) error {
	idx := newServerIndex(0, 0)
	for _, r := range roles {
		idx.addRole(r.Name, r.ID)
	}
	var ids [3]uint64
	for i, name := range h.names {
		id, err := idx.lookupRole(name)
		if err != nil {
			return fmt.Errorf("supporter roles: %w", err)
		}
		ids[i] = uint64(id)
	}
	h.mu.Lock()
	h.roles = &supporterRoleIDs{supporter: ids[0], koFi: ids[1], patreon: ids[2]}
	h.mu.Unlock()
	return nil
}

// supporterAction is what one member's roles call for.
type supporterAction int

const (
	supporterNone supporterAction = iota
	supporterGive
)

// Observe reads one member's roles, as a member event or the start sweep delivers them. A member
// with a platform role gets Supporter and leaves the grace period; a member who holds Supporter
// without one enters it, and nothing is taken here.
func (h *SupporterHandler) Observe(ctx context.Context, m SupporterMember, calls SupporterCalls) {
	action, roleID := h.record(m)
	if action == supporterGive {
		h.call(ctx, calls, m.ID, roleID, true)
	}
}

// record updates the grace state of m and reports whether Supporter must be given.
func (h *SupporterHandler) record(m SupporterMember) (supporterAction, uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.roles == nil {
		return supporterNone, 0
	}
	held := roleSet(m.RoleIDs)
	platform := held[h.roles.koFi] || held[h.roles.patreon]
	switch {
	case platform:
		delete(h.lapsed, m.ID)
		if !held[h.roles.supporter] {
			return supporterGive, h.roles.supporter
		}
	case held[h.roles.supporter]:
		if _, running := h.lapsed[m.ID]; !running && len(h.lapsed) < maxLapsedMembers {
			h.lapsed[m.ID] = h.now()
		}
	default:
		delete(h.lapsed, m.ID)
	}
	return supporterNone, 0
}

// Forget drops a member who left the server.
func (h *SupporterHandler) Forget(memberID uint64) {
	h.mu.Lock()
	delete(h.lapsed, memberID)
	h.mu.Unlock()
}

// Lapsed is the number of members with a running grace period.
func (h *SupporterHandler) Lapsed() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.lapsed)
}

// RemoveLapsed takes Supporter from every member whose grace period has passed, and returns how
// many it took. A member whose call fails stays for the next run.
func (h *SupporterHandler) RemoveLapsed(ctx context.Context, calls SupporterCalls) int {
	due, roleID := h.due()
	removed := 0
	for _, id := range due {
		if ctx.Err() != nil {
			break
		}
		if h.call(ctx, calls, id, roleID, false) {
			h.Forget(id)
			removed++
		}
	}
	return removed
}

// due lists the members whose grace period has passed at the current time.
func (h *SupporterHandler) due() ([]uint64, uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.roles == nil {
		return nil, 0
	}
	now := h.now()
	var due []uint64
	for id, since := range h.lapsed {
		if now.Sub(since) >= h.grace {
			due = append(due, id)
		}
	}
	return due, h.roles.supporter
}

// call gives or takes Supporter and logs the action and the outcome, never the member.
func (h *SupporterHandler) call(ctx context.Context, calls SupporterCalls, memberID, roleID uint64, give bool) bool {
	callCtx, cancel := context.WithTimeout(ctx, h.callTimeout)
	defer cancel()
	action := "taken"
	var err error
	if give {
		action = "given"
		err = calls.AddRole(callCtx, memberID, roleID)
	} else {
		err = calls.RemoveRole(callCtx, memberID, roleID)
	}
	if err != nil {
		h.logger.Warn("supporter role change failed", slog.String("action", action),
			slog.String("outcome", "failure"), slog.String("error_class", classifyError(err)))
		return false
	}
	h.logger.Info("supporter role changed", slog.String("action", action), slog.String("outcome", "ok"))
	return true
}
