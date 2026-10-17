package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/disgoorg/disgo/discord"

	"github.com/tekmora/diallak/internal/serverdef"
)

const (
	// AreasCommandName is the slash command a member takes and drops area roles with (ADR-0011).
	AreasCommandName = "areas"
	// AreasCommandDescription is the description of the areas command in Discord's command list.
	AreasCommandDescription = "Choose your interest, language and notification roles"

	// areasCustomIDPrefix starts the custom id of every menu of the areas command; the question
	// id of the definition follows it.
	areasCustomIDPrefix = "areas:"
	// areasAuditReason is sent with every role call and shows in the server's audit log.
	areasAuditReason = "The member's own choice through /areas"

	// TextAreasIntro heads the message of the areas command.
	TextAreasIntro = "Choose what you want to see. Nothing is required, and /areas changes your choice at any time.\n" +
		"Other members can see the roles you hold, so they can see what you choose here."
	// TextAreasPending refuses a member who has still to pass the server's screening.
	TextAreasPending = "Finish the server's membership screening first, then run /areas again."
	// TextAreasRefused refuses a change that does not match the options of its menu.
	TextAreasRefused = "This choice was not applied: it does not match the options of the menu. Run /areas again."
	// TextAreasNone is shown when no option of the definition has its role on the server.
	TextAreasNone = "No area can be chosen right now."
	// TextAreasUnread says that nothing was applied because the server's roles could not be read.
	TextAreasUnread = "Nothing was applied: the server's roles could not be read. Run /areas again later."
	// textAreasNoChange is the status of a change that matched what the member held.
	textAreasNoChange = "Nothing changed."

	// defaultAreasCallTimeout is chosen, not measured.
	defaultAreasCallTimeout = 10 * time.Second
	maxAreasCallTimeout     = 1 * time.Minute
)

// areaMenu is one question of the definition's choices as the areas command shows it.
type areaMenu struct {
	id       string
	title    string
	multiple bool
	options  []areaOption
}

// areaOption is one option of a menu: its title, the role id of the definition and the role's
// name on the server.
type areaOption struct {
	title    string
	role     string
	roleName string
}

// AreasView is a message of the areas command: a text and the menus below it.
type AreasView struct {
	Text  string
	Menus []MenuView
}

// MenuView is one menu of a view. Values are role ids of the definition.
type MenuView struct {
	CustomID    string
	Placeholder string
	MaxValues   int
	Options     []MenuOption
}

// MenuOption is one option of a menu; Default marks a role the member holds.
type MenuOption struct {
	Label   string
	Value   string
	Default bool
}

// AreasInteraction is an areas command or a change of one of its menus, as the handler needs it.
type AreasInteraction interface {
	GuildID() uint64
	// MemberID is the member who sent the interaction, or zero when Discord sent none.
	MemberID() uint64
	// MemberRoleIDs are the roles Discord sent with the interaction.
	MemberRoleIDs() []uint64
	// Pending reports whether the member has still to pass the server's screening.
	Pending() bool
	// Show answers at once: a new message only the member sees for the command, the
	// updated message for a change.
	Show(ctx context.Context, view AreasView) error
	// Acknowledge answers a change at once and leaves the message as it is until Edit.
	Acknowledge(ctx context.Context) error
	// Edit replaces the message after Acknowledge.
	Edit(ctx context.Context, view AreasView) error
}

// RoleCalls are the calls a change makes against the server.
type RoleCalls interface {
	ReadRoles(ctx context.Context) ([]discord.Role, error)
	AddRole(ctx context.Context, memberID, roleID uint64) error
	RemoveRole(ctx context.Context, memberID, roleID uint64) error
}

// AreasConfig configures an AreasHandler.
type AreasConfig struct {
	Definition *serverdef.Definition
	GuildID    uint64
	Logger     *slog.Logger
	// CallTimeout is the deadline of each call to Discord. Zero means the default.
	CallTimeout time.Duration
}

// AreasHandler serves the areas command: it shows the definition's choices and gives and takes
// the chosen roles. It stores nothing; the roles on the server are the state (ADR-0011).
type AreasHandler struct {
	menus       []areaMenu
	guildID     uint64
	logger      *slog.Logger
	callTimeout time.Duration
	roles       atomic.Pointer[serverIndex]
}

// NewAreasHandler builds the menus from the definition's choices.
func NewAreasHandler(cfg AreasConfig) (*AreasHandler, error) {
	if cfg.Definition == nil {
		return nil, errors.New("definition is required")
	}
	if cfg.GuildID == 0 {
		return nil, errors.New("guild ID must not be zero")
	}
	if cfg.CallTimeout < 0 || cfg.CallTimeout > maxAreasCallTimeout {
		return nil, fmt.Errorf("call timeout %v out of range [0, %v]", cfg.CallTimeout, maxAreasCallTimeout)
	}
	h := &AreasHandler{
		menus:       areaMenus(cfg.Definition),
		guildID:     cfg.GuildID,
		logger:      cfg.Logger,
		callTimeout: cfg.CallTimeout,
	}
	if h.logger == nil {
		h.logger = slog.New(slog.DiscardHandler)
	}
	if h.callTimeout == 0 {
		h.callTimeout = defaultAreasCallTimeout
	}
	return h, nil
}

func areaMenus(def *serverdef.Definition) []areaMenu {
	names := make(map[string]string, len(def.Roles))
	for _, r := range def.Roles {
		names[r.ID] = r.Name
	}
	menus := make([]areaMenu, 0, len(def.Choices.Questions))
	for _, q := range def.Choices.Questions {
		m := areaMenu{id: q.ID, title: q.Title, multiple: q.Multiple, options: make([]areaOption, 0, len(q.Options))}
		for _, o := range q.Options {
			m.options = append(m.options, areaOption{title: o.Title, role: o.Role, roleName: names[o.Role]})
		}
		menus = append(menus, m)
	}
	return menus
}

// SetRoles takes the server's roles; options are matched to them by name.
func (h *AreasHandler) SetRoles(roles []discord.Role) {
	idx := newServerIndex(h.guildID, 0)
	for _, r := range roles {
		idx.addRole(r.Name, r.ID)
	}
	h.roles.Store(idx)
}

// HandleCommand answers the areas command with the menus, the member's roles selected. It makes
// no call besides the answer.
func (h *AreasHandler) HandleCommand(ctx context.Context, in AreasInteraction) {
	defer h.recoverPanic("HandleCommand")
	if refusal, ok := h.refuseSender(in); ok {
		h.show(ctx, in, AreasView{Text: refusal})
		return
	}
	h.show(ctx, in, h.view(TextAreasIntro, roleSet(in.MemberRoleIDs())))
}

// HandleChange gives and takes the roles of one menu so that the member holds exactly the
// chosen ones. A change that does not match the menu is refused whole, before any call.
func (h *AreasHandler) HandleChange(ctx context.Context, in AreasInteraction, customID string, values []string, calls RoleCalls) {
	defer h.recoverPanic("HandleChange")
	if refusal, ok := h.refuseSender(in); ok {
		h.show(ctx, in, AreasView{Text: refusal})
		return
	}
	if in.MemberID() == 0 {
		h.logger.Info("area change refused", slog.String("reason", "no-member"))
		h.show(ctx, in, AreasView{Text: TextAreasRefused})
		return
	}
	menu, chosen, ok := h.checkChange(customID, values)
	if !ok {
		h.logger.Info("area change refused", slog.String("reason", "not-an-option"))
		h.show(ctx, in, AreasView{Text: TextAreasRefused})
		return
	}

	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	err := in.Acknowledge(ackCtx)
	cancel()
	if err != nil {
		h.logger.Warn("area change acknowledge failed", slog.String("error_class", classifyError(err)))
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, h.callTimeout)
	roles, err := calls.ReadRoles(readCtx)
	cancel()
	if err != nil {
		h.logger.Warn("area roles read failed", slog.String("error_class", classifyError(err)))
		h.edit(ctx, in, AreasView{Text: TextAreasUnread})
		return
	}
	h.SetRoles(roles)

	held := roleSet(in.MemberRoleIDs())
	status := h.apply(ctx, calls, in.MemberID(), menu, chosen, held)
	h.edit(ctx, in, h.view(status, held))
}

// refuseSender returns the refusal for an interaction from another server or from a member in
// screening.
func (h *AreasHandler) refuseSender(in AreasInteraction) (string, bool) {
	if in.GuildID() == 0 || in.GuildID() != h.guildID {
		h.logger.Info("area interaction refused", slog.String("reason", "other-guild"))
		return TextWrongGuild, true
	}
	if in.Pending() {
		h.logger.Info("area interaction refused", slog.String("reason", "pending"))
		return TextAreasPending, true
	}
	return "", false
}

// checkChange returns the menu of customID and the chosen role ids of the definition. The list of
// options comes from the definition, never from the request.
func (h *AreasHandler) checkChange(customID string, values []string) (areaMenu, map[string]bool, bool) {
	id, ok := strings.CutPrefix(customID, areasCustomIDPrefix)
	if !ok {
		return areaMenu{}, nil, false
	}
	for _, m := range h.menus {
		if m.id != id {
			continue
		}
		chosen, ok := checkValues(m, values)
		return m, chosen, ok
	}
	return areaMenu{}, nil, false
}

func checkValues(m areaMenu, values []string) (map[string]bool, bool) {
	limit := 1
	if m.multiple {
		limit = len(m.options)
	}
	if len(values) > limit {
		return nil, false
	}
	options := make(map[string]bool, len(m.options))
	for _, o := range m.options {
		options[o.role] = true
	}
	chosen := make(map[string]bool, len(values))
	for _, v := range values {
		if !options[v] || chosen[v] {
			return nil, false
		}
		chosen[v] = true
	}
	return chosen, true
}

// apply makes one call per option whose role the member has to gain or lose, in the menu's
// order, and updates held with each call that succeeded. A failed call is not repeated.
func (h *AreasHandler) apply(
	ctx context.Context,
	calls RoleCalls,
	memberID uint64,
	m areaMenu,
	chosen map[string]bool,
	held map[uint64]bool,
) string {
	idx := h.roles.Load()
	var given, taken, failed []string
	for _, o := range m.options {
		roleID, err := idx.lookupRole(o.roleName)
		if err != nil {
			if chosen[o.role] {
				failed = append(failed, o.title)
			}
			continue
		}
		id := uint64(roleID)
		if chosen[o.role] == held[id] {
			continue
		}
		if !h.call(ctx, calls, memberID, id, o.role, chosen[o.role]) {
			failed = append(failed, o.title)
			continue
		}
		held[id] = chosen[o.role]
		if chosen[o.role] {
			given = append(given, o.title)
		} else {
			taken = append(taken, o.title)
		}
	}
	return changeStatus(given, taken, failed)
}

// call gives or takes one role and logs the role of the definition, the action and the outcome,
// never the member.
func (h *AreasHandler) call(ctx context.Context, calls RoleCalls, memberID, roleID uint64, role string, give bool) bool {
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
		h.logger.Warn("area role change failed", slog.String("role", role), slog.String("action", action),
			slog.String("outcome", "failure"), slog.String("error_class", classifyError(err)))
		return false
	}
	h.logger.Info("area role changed", slog.String("role", role), slog.String("action", action),
		slog.String("outcome", "ok"))
	return true
}

func changeStatus(given, taken, failed []string) string {
	parts := make([]string, 0, 3)
	if len(given) > 0 {
		parts = append(parts, "Given: "+strings.Join(given, ", ")+".")
	}
	if len(taken) > 0 {
		parts = append(parts, "Taken: "+strings.Join(taken, ", ")+".")
	}
	if len(failed) > 0 {
		parts = append(parts, "Not applied: "+strings.Join(failed, ", ")+". Run /areas again to see what you hold.")
	}
	if len(parts) == 0 {
		return textAreasNoChange
	}
	return strings.Join(parts, "\n")
}

// view builds the menus, the roles in held selected. An option whose role is missing on the
// server, or whose name is there twice, is left out; a menu without options is left out.
func (h *AreasHandler) view(text string, held map[uint64]bool) AreasView {
	idx := h.roles.Load()
	if idx == nil {
		return AreasView{Text: TextAreasNone}
	}
	v := AreasView{Text: text, Menus: make([]MenuView, 0, len(h.menus))}
	for _, m := range h.menus {
		mv := MenuView{CustomID: areasCustomIDPrefix + m.id, Placeholder: m.title}
		for _, o := range m.options {
			roleID, err := idx.lookupRole(o.roleName)
			if err != nil {
				continue
			}
			mv.Options = append(mv.Options, MenuOption{Label: o.title, Value: o.role, Default: held[uint64(roleID)]})
		}
		if len(mv.Options) == 0 {
			continue
		}
		mv.MaxValues = 1
		if m.multiple {
			mv.MaxValues = len(mv.Options)
		}
		v.Menus = append(v.Menus, mv)
	}
	if len(v.Menus) == 0 {
		return AreasView{Text: TextAreasNone}
	}
	return v
}

func roleSet(ids []uint64) map[uint64]bool {
	held := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		held[id] = true
	}
	return held
}

func (h *AreasHandler) show(ctx context.Context, in AreasInteraction, v AreasView) {
	showCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	defer cancel()
	if err := in.Show(showCtx, v); err != nil {
		h.logger.Warn("area answer failed", slog.String("error_class", classifyError(err)))
	}
}

func (h *AreasHandler) edit(ctx context.Context, in AreasInteraction, v AreasView) {
	editCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), h.callTimeout)
	defer cancel()
	if err := in.Edit(editCtx, v); err != nil {
		h.logger.Warn("area edit failed", slog.String("error_class", classifyError(err)))
	}
}

func (h *AreasHandler) recoverPanic(where string) {
	if r := recover(); r != nil {
		h.logger.Error("recovered from panic in "+where, slog.String("panic_type", fmt.Sprintf("%T", r)))
	}
}
