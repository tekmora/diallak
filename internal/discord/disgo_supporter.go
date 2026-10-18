package discord

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

const (
	// memberPageSize is the largest page List Guild Members returns.
	memberPageSize = 1000
	// maxMemberPages bounds the start sweep: a million members.
	maxMemberPages = 1000
	// supporterCheckInterval is how often the grace periods are checked. It is chosen, not measured;
	// a role is taken at most this long after its grace period ends.
	supporterCheckInterval = time.Hour
	// maxSupporterChecks bounds the check loop: more hours than a process runs.
	maxSupporterChecks = 1 << 20
)

// restSupporterCalls give and take Supporter over the platform's HTTP API, each with the fixed
// reason the audit log shows.
type restSupporterCalls struct {
	rest    rest.Rest
	guildID snowflake.ID
}

func (c restSupporterCalls) AddRole(ctx context.Context, memberID, roleID uint64) error {
	err := c.rest.AddMemberRole(c.guildID, snowflake.ID(memberID), snowflake.ID(roleID),
		rest.WithCtx(ctx), rest.WithReason(supporterAuditReason))
	if err != nil {
		return callError("add member role", err, "")
	}
	return nil
}

func (c restSupporterCalls) RemoveRole(ctx context.Context, memberID, roleID uint64) error {
	err := c.rest.RemoveMemberRole(c.guildID, snowflake.ID(memberID), snowflake.ID(roleID),
		rest.WithCtx(ctx), rest.WithReason(supporterAuditReason))
	if err != nil {
		return callError("remove member role", err, "")
	}
	return nil
}

// supporterMemberOf keeps the id and the roles of m and drops everything else (ADR-0027).
func supporterMemberOf(m discord.Member) SupporterMember {
	ids := make([]uint64, len(m.RoleIDs))
	for i, id := range m.RoleIDs {
		ids[i] = uint64(id)
	}
	return SupporterMember{ID: uint64(m.User.ID), RoleIDs: ids}
}

// handleMember hands a member event of the configured server to the supporter function.
func (d *dispatcher) handleMember(ctx context.Context, e *events.GenericGuildMember, rc rest.Rest) {
	if !d.dispatching.Load() || uint64(e.GuildID) != d.cfg.GuildID {
		return
	}
	m := supporterMemberOf(e.Member)
	calls := restSupporterCalls{rest: rc, guildID: e.GuildID}
	d.runTracked(func() { d.cfg.Supporter.Observe(ctx, m, calls) })
}

// handleMemberLeave drops a member who left from the grace periods.
func (d *dispatcher) handleMemberLeave(e *events.GuildMemberLeave) {
	if uint64(e.GuildID) != d.cfg.GuildID {
		return
	}
	d.cfg.Supporter.Forget(uint64(e.User.ID))
}

// readSupporterRoles reads the server's roles once at start and stops the start when one of the
// three is missing.
func readSupporterRoles(ctx context.Context, client *bot.Client, cfg BotConfig, opts clientOptions) error {
	readCtx, cancel := context.WithTimeout(ctx, opts.callTimeout())
	defer cancel()
	roles, err := restRoleCalls{rest: client.Rest, guildID: snowflake.ID(cfg.GuildID)}.ReadRoles(readCtx)
	if err != nil {
		return fmt.Errorf("read roles for the supporter function: %w", sanitizeToken(err, cfg.Token))
	}
	return cfg.Supporter.SetRoles(roles)
}

// runSupporter lists the members once, since a restart lost every grace period, and then checks
// the grace periods until ctx ends.
func runSupporter(ctx context.Context, restClient rest.Rest, cfg BotConfig, opts clientOptions) {
	calls := restSupporterCalls{rest: restClient, guildID: snowflake.ID(cfg.GuildID)}
	n, err := sweepMembers(ctx, restClient, cfg, calls, opts.callTimeout())
	if err != nil {
		cfg.Logger.Warn("supporter sweep failed", slog.Int("members", n), slog.Any("error", err))
	} else {
		cfg.Logger.Info("supporter sweep done", slog.Int("members", n))
	}
	ticker := time.NewTicker(opts.supporterCheck())
	defer ticker.Stop()
	for range maxSupporterChecks {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if removed := cfg.Supporter.RemoveLapsed(ctx, calls); removed > 0 {
			cfg.Logger.Info("supporter grace periods ended", slog.Int("taken", removed))
		}
	}
}

// sweepMembers reads the members page by page, in id order, and hands each to the function.
func sweepMembers(ctx context.Context, rc rest.Rest, cfg BotConfig, calls SupporterCalls, timeout time.Duration) (int, error) {
	var after snowflake.ID
	seen := 0
	for range maxMemberPages {
		pageCtx, cancel := context.WithTimeout(ctx, timeout)
		page, err := rc.GetMembers(snowflake.ID(cfg.GuildID), memberPageSize, after, rest.WithCtx(pageCtx))
		cancel()
		if err != nil {
			return seen, callError("list guild members", err, "")
		}
		for _, m := range page {
			cfg.Supporter.Observe(ctx, supporterMemberOf(m), calls)
			after = m.User.ID
		}
		seen += len(page)
		if len(page) < memberPageSize {
			return seen, nil
		}
	}
	return seen, fmt.Errorf("list guild members: more than %d pages", maxMemberPages)
}

// supporterListeners are the member event listeners of the supporter function. The member cache
// stays off, so no member is held beyond the grace state (ADR-0027).
func supporterListeners(ctx context.Context, d *dispatcher) []bot.ConfigOpt {
	return []bot.ConfigOpt{
		bot.WithEventListenerFunc(func(e *events.GuildMemberJoin) {
			d.handleMember(ctx, e.GenericGuildMember, e.Client().Rest)
		}),
		bot.WithEventListenerFunc(func(e *events.GuildMemberUpdate) {
			d.handleMember(ctx, e.GenericGuildMember, e.Client().Rest)
		}),
		bot.WithEventListenerFunc(d.handleMemberLeave),
	}
}
