// Command diallak is the community bot for the hub server.
//
// It serves guild command /ask answered by the documentation pipeline and /areas, with which a
// member takes and drops area roles, and can
// register commands via -register-commands.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tekmora/diallak/internal/app"
	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/discord"
	"github.com/tekmora/diallak/internal/health"
	"github.com/tekmora/diallak/internal/model"
	"github.com/tekmora/diallak/internal/serverdef"
	"github.com/tekmora/diallak/internal/serverplan"
	"github.com/tekmora/diallak/server"

	disgodiscord "github.com/disgoorg/disgo/discord"
)

const (
	startTimeout           = 30 * time.Second
	registerTimeout        = 30 * time.Second
	serverTimeout          = 60 * time.Second
	applyTimeout           = 15 * time.Minute // chosen, not measured; covers a full apply run under 429 back-off
	roleConnectionsTimeout = 60 * time.Second
)

type dependencies struct {
	registerCommands    func(ctx context.Context, token string, guildID uint64, maxQuestionChars int) error
	runDiscord          func(ctx context.Context, cfg discord.BotConfig) error
	readServer          func(ctx context.Context, token string, guildID uint64) (*serverplan.Snapshot, error)
	applyServer         func(ctx context.Context, token string, guildID uint64, in discord.ApplyInput, out io.Writer) error
	pushRoleConnections func(ctx context.Context, token string, records []disgodiscord.ApplicationRoleConnectionMetadata, out io.Writer) error
}

func defaultDependencies() dependencies {
	return dependencies{
		registerCommands:    discord.RegisterCommands,
		runDiscord:          discord.Run,
		readServer:          discord.ReadServer,
		applyServer:         discord.ApplyServer,
		pushRoleConnections: discord.PushRoleConnections,
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	os.Exit(report(run(ctx, os.Args[1:], os.Getenv, os.Stdout, defaultDependencies()), os.Stderr))
}

// report prints err to errOut and returns the process exit code: 0 without an error, 1 with a
// reported one, and 2 when the report itself could not be written.
func report(err error, errOut io.Writer) int {
	if err == nil {
		return 0
	}
	if _, writeErr := fmt.Fprintln(errOut, "diallak:", err); writeErr != nil {
		return 2
	}
	return 1
}

// run parses arguments, loads configuration, and executes either command registration, server inspection, or the bot runtime.
func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer, deps dependencies) (err error) {
	fs := flag.NewFlagSet("diallak", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerCmds := fs.Bool("register-commands", false, "register guild commands and exit")
	serverMode := fs.String("server", "", "plan, compare, or apply Discord server configuration")
	roleConnMode := fs.String("role-connections", "", "plan or push Discord role-connection metadata")
	if parseErr := fs.Parse(args); parseErr != nil {
		return fmt.Errorf("parse flags: %w", parseErr)
	}

	if err := validateCommandFlags(*serverMode, *registerCmds, *roleConnMode); err != nil {
		return err
	}

	if *roleConnMode == "plan" {
		return executeRoleConnectionsPlan(out)
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	if strings.HasPrefix(*roleConnMode, "push=") {
		planDigest := strings.TrimPrefix(*roleConnMode, "push=")
		return executeRoleConnectionsPush(ctx, cfg, planDigest, out, deps.pushRoleConnections)
	}
	if *registerCmds {
		return executeRegisterCommands(ctx, cfg, out, deps.registerCommands)
	}
	if strings.HasPrefix(*serverMode, "apply=") {
		planDigest := strings.TrimPrefix(*serverMode, "apply=")
		return executeApplyCommand(ctx, cfg, planDigest, out, deps.applyServer)
	}
	if *serverMode != "" {
		return executeServerCommand(ctx, cfg, *serverMode, out, deps.readServer)
	}
	return executeBotRun(ctx, cfg, out, deps.runDiscord)
}

func validateCommandFlags(serverMode string, registerCmds bool, roleConnMode string) error {
	if roleConnMode != "" && (serverMode != "" || registerCmds) {
		return errors.New("-role-connections cannot be used with -server or -register-commands")
	}
	if err := validateRoleConnFlags(roleConnMode); err != nil {
		return err
	}
	return validateServerFlags(serverMode, registerCmds)
}

func validateRoleConnFlags(roleConnMode string) error {
	if roleConnMode == "" || roleConnMode == "plan" {
		return nil
	}
	if strings.HasPrefix(roleConnMode, "push=") {
		planDigest := strings.TrimPrefix(roleConnMode, "push=")
		if !isValidPlanDigest(planDigest) {
			return fmt.Errorf("invalid -role-connections mode %q: push needs the digest that -role-connections=plan printed, as push=sha256:<64 hex digits>", roleConnMode)
		}
		return nil
	}
	return fmt.Errorf("invalid -role-connections mode %q: must be plan or push=sha256:<64 hex digits>", roleConnMode)
}

func executeRoleConnectionsPlan(out io.Writer) error {
	recs, _, err := server.RoleConnections()
	if err != nil {
		return fmt.Errorf("load role connections: %w", err)
	}
	body, err := json.MarshalIndent(recs.Records, "", "  ")
	if err != nil {
		return fmt.Errorf("format role connections: %w", err)
	}
	digest := server.RoleConnectionsDigest()
	if _, err := fmt.Fprintln(out, "PUT /applications/{application.id}/role-connections/metadata"); err != nil {
		return fmt.Errorf("write plan method: %w", err)
	}
	if _, err := fmt.Fprintln(out, string(body)); err != nil {
		return fmt.Errorf("write plan body: %w", err)
	}
	if _, err := fmt.Fprintf(out, "digest %s\n", digest); err != nil {
		return fmt.Errorf("write plan digest: %w", err)
	}
	if _, err := fmt.Fprintf(out, "dry run: nothing sent; push with -role-connections=push=%s\n", digest); err != nil {
		return fmt.Errorf("write plan footer: %w", err)
	}
	return nil
}

func executeRoleConnectionsPush(
	ctx context.Context,
	cfg config.Config,
	planDigest string,
	out io.Writer,
	pushRoleConnections func(ctx context.Context, token string, records []disgodiscord.ApplicationRoleConnectionMetadata, out io.Writer) error,
) error {
	if planDigest != server.RoleConnectionsDigest() {
		return errors.New("records changed since plan")
	}

	pushCtx, cancel := context.WithTimeout(ctx, roleConnectionsTimeout)
	defer cancel()

	recs, _, err := server.RoleConnections()
	if err != nil {
		return fmt.Errorf("load role connections: %w", err)
	}

	if err := pushRoleConnections(pushCtx, cfg.Token, recs.Records, out); err != nil {
		return fmt.Errorf("push role connections: %w", err)
	}
	return nil
}

func validateServerFlags(serverMode string, registerCmds bool) error {
	if serverMode != "" && registerCmds {
		return errors.New("-server and -register-commands cannot be used together")
	}
	if strings.HasPrefix(serverMode, "apply=") {
		planDigest := strings.TrimPrefix(serverMode, "apply=")
		if !isValidPlanDigest(planDigest) {
			return fmt.Errorf("invalid -server mode %q: apply needs the digest that -server=plan printed, as apply=sha256:<64 hex digits>", serverMode)
		}
		return nil
	}
	if serverMode != "" && serverMode != "plan" && serverMode != "compare" {
		return fmt.Errorf("invalid -server mode %q: must be plan, compare or apply=<plan digest>", serverMode)
	}
	return nil
}

func isValidPlanDigest(d string) bool {
	const prefix = "sha256:"
	if !strings.HasPrefix(d, prefix) {
		return false
	}
	hexPart := d[len(prefix):]
	if len(hexPart) != 64 {
		return false
	}
	for i := 0; i < len(hexPart); i++ {
		c := hexPart[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func executeApplyCommand(
	ctx context.Context,
	cfg config.Config,
	planDigest string,
	out io.Writer,
	applyServer func(ctx context.Context, token string, guildID uint64, in discord.ApplyInput, out io.Writer) error,
) error {
	applyCtx, cancel := context.WithTimeout(ctx, applyTimeout)
	defer cancel()

	def, err := server.Definition()
	if err != nil {
		return fmt.Errorf("load definition: %w", err)
	}

	in := discord.ApplyInput{
		Definition:       def,
		DefinitionDigest: server.DefinitionDigest(),
		PlanDigest:       planDigest,
	}
	if err := applyServer(applyCtx, cfg.Token, cfg.GuildID, in, out); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	return nil
}

func executeServerCommand(
	ctx context.Context,
	cfg config.Config,
	mode string,
	out io.Writer,
	readServer func(ctx context.Context, token string, guildID uint64) (*serverplan.Snapshot, error),
) error {
	srvCtx, cancel := context.WithTimeout(ctx, serverTimeout)
	defer cancel()

	def, err := server.Definition()
	if err != nil {
		return fmt.Errorf("load definition: %w", err)
	}

	snap, err := readServer(srvCtx, cfg.Token, cfg.GuildID)
	if err != nil {
		return fmt.Errorf("read server: %w", err)
	}

	if mode == "plan" {
		res, err := serverplan.Plan(def, snap)
		if err != nil {
			return fmt.Errorf("plan: %w", err)
		}
		if err := serverplan.Render(out, res); err != nil {
			return fmt.Errorf("render plan: %w", err)
		}
		defDigest := server.DefinitionDigest()
		if _, writeErr := fmt.Fprintf(out, "definition: %s\n", defDigest); writeErr != nil {
			return fmt.Errorf("write definition digest: %w", writeErr)
		}
		digest, err := serverplan.Digest(res, defDigest)
		if err != nil {
			return fmt.Errorf("digest plan: %w", err)
		}
		if _, writeErr := fmt.Fprintf(out, "plan digest: %s\n", digest); writeErr != nil {
			return fmt.Errorf("write plan digest: %w", writeErr)
		}
		return nil
	}

	diffs, err := serverplan.Compare(def, snap)
	if err != nil {
		return fmt.Errorf("compare: %w", err)
	}
	raised := serverplan.Raised(def, snap)
	if err := serverplan.RenderDifferences(out, raised, diffs); err != nil {
		return fmt.Errorf("render differences: %w", err)
	}
	return nil
}

func executeRegisterCommands(
	ctx context.Context,
	cfg config.Config,
	out io.Writer,
	register func(ctx context.Context, token string, guildID uint64, maxQuestionChars int) error,
) error {
	regCtx, cancel := context.WithTimeout(ctx, registerTimeout)
	defer cancel()

	if err := register(regCtx, cfg.Token, cfg.GuildID, cfg.QuestionMaxChars); err != nil {
		return fmt.Errorf("register commands: %w", err)
	}

	if _, writeErr := fmt.Fprintf(out, "commands registered for guild %d\n", cfg.GuildID); writeErr != nil {
		return fmt.Errorf("write status: %w", writeErr)
	}
	return nil
}

func executeBotRun(
	ctx context.Context,
	cfg config.Config,
	out io.Writer,
	runDiscord func(ctx context.Context, cfg discord.BotConfig) error,
) (err error) {
	hh, err := initHealth(ctx, cfg.HealthAddr)
	if err != nil {
		return err
	}
	if hh != nil {
		defer func() {
			if hh.ln != nil {
				if closeErr := hh.ln.Close(); closeErr != nil {
					err = errors.Join(err, fmt.Errorf("close health listener: %w", closeErr))
				}
			}
		}()
	}

	buildCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	assembly, cleanup, err := app.Build(buildCtx, cfg, slog.Default())
	if err != nil {
		return fmt.Errorf("build application: %w", err)
	}
	defer func() {
		if closeErr := cleanup(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close application: %w", closeErr))
		}
	}()

	if err := probeModel(ctx, assembly.Model); err != nil {
		return err
	}

	if err := reportAssemblyStatus(out, assembly); err != nil {
		return err
	}

	hh.attachModel(assembly.Model)

	botCfg, err := buildDiscordConfig(cfg, assembly)
	if err != nil {
		return err
	}

	return runDiscordWithHealth(ctx, hh, hh.takeListener(), botCfg, runDiscord)
}

func probeModel(ctx context.Context, gate *model.Gate) error {
	if gate == nil {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	if err := gate.Start(probeCtx); err != nil {
		return fmt.Errorf("%w; check %s, %s, %s, %s and %s", err,
			config.EnvModelBaseURL, config.EnvModelAlias, config.EnvModelKeyFile, config.EnvModelPolicy,
			config.EnvModelGuardrail)
	}
	return nil
}

// newReceipts holds receipts when the model is on: only then can text leave the bot (ADR-0013).
func newReceipts(assembly *app.Assembly) (*discord.Receipts, error) {
	if assembly.Model == nil {
		return nil, nil
	}
	receipts, err := discord.NewReceipts(discord.ReceiptsConfig{})
	if err != nil {
		return nil, fmt.Errorf("create receipts: %w", err)
	}
	return receipts, nil
}

// newForum builds the first answer in the support forum when it is configured (ADR-0009,
// ADR-0020). It answers with excerpts only, whatever the model settings are.
func newForum(cfg config.Config, assembly *app.Assembly) (*discord.ForumHandler, error) {
	if !cfg.HasForum() {
		return nil, nil
	}
	forum, err := discord.NewForumHandler(discord.ForumConfig{
		Asker:            assembly.Pipeline,
		ChannelID:        cfg.ForumChannelID,
		OptInTagID:       cfg.ForumOptInTagID,
		MaxConcurrent:    cfg.MaxConcurrentAsks,
		Timeout:          cfg.AskTimeout,
		QuestionMaxChars: cfg.QuestionMaxChars,
		Logger:           slog.Default(),
	})
	if err != nil {
		return nil, fmt.Errorf("create forum handler: %w", err)
	}
	return forum, nil
}

func newAreas(def *serverdef.Definition, guildID uint64) (*discord.AreasHandler, error) {
	areas, err := discord.NewAreasHandler(discord.AreasConfig{
		Definition: def,
		GuildID:    guildID,
		Logger:     slog.Default(),
	})
	if err != nil {
		return nil, fmt.Errorf("create areas handler: %w", err)
	}
	return areas, nil
}

func newBug(def *serverdef.Definition, guildID uint64) (*discord.BugHandler, error) {
	if def.BugReports == nil {
		return nil, nil
	}
	bug, err := discord.NewBugHandler(discord.BugConfig{
		Repository: def.BugReports.Repository,
		GuildID:    guildID,
		Logger:     slog.Default(),
	})
	if err != nil {
		return nil, fmt.Errorf("create bug handler: %w", err)
	}
	return bug, nil
}

// newSupporter builds the supporter function when it is switched on (ADR-0024, ADR-0027). Off, the
// bot asks for no members intent.
func newSupporter(cfg config.Config, def *serverdef.Definition) (*discord.SupporterHandler, error) {
	if !cfg.HasSupporterRoles() {
		return nil, nil
	}
	supporter, err := discord.NewSupporterHandler(discord.SupporterConfig{
		Definition: def,
		GuildID:    cfg.GuildID,
		Grace:      cfg.SupporterGrace,
		Logger:     slog.Default(),
	})
	if err != nil {
		return nil, fmt.Errorf("create supporter handler: %w", err)
	}
	return supporter, nil
}

func buildDiscordConfig(cfg config.Config, assembly *app.Assembly) (discord.BotConfig, error) {
	receipts, err := newReceipts(assembly)
	if err != nil {
		return discord.BotConfig{}, err
	}
	handler, err := discord.NewHandler(discord.HandlerConfig{
		Asker:            assembly.Pipeline,
		GuildID:          cfg.GuildID,
		Logger:           slog.Default(),
		QuestionMaxChars: cfg.QuestionMaxChars,
		MaxConcurrent:    cfg.MaxConcurrentAsks,
		AskTimeout:       cfg.AskTimeout,
		RenderOptions:    assembly.RenderOptions,
		Receipts:         receipts,
	})
	if err != nil {
		return discord.BotConfig{}, fmt.Errorf("create discord handler: %w", err)
	}
	def, err := server.Definition()
	if err != nil {
		return discord.BotConfig{}, fmt.Errorf("load definition: %w", err)
	}
	areas, err := newAreas(def, cfg.GuildID)
	if err != nil {
		return discord.BotConfig{}, err
	}
	bug, err := newBug(def, cfg.GuildID)
	if err != nil {
		return discord.BotConfig{}, err
	}
	moderation, err := discord.NewModerationHandler(discord.ModerationConfig{
		Definition: def,
		GuildID:    cfg.GuildID,
		Logger:     slog.Default(),
		Retention:  cfg.TrailRetention,
	})
	if err != nil {
		return discord.BotConfig{}, fmt.Errorf("create moderation handler: %w", err)
	}
	bc := discord.BotConfig{
		Token:      cfg.Token,
		GuildID:    cfg.GuildID,
		Handler:    handler,
		Areas:      areas,
		Bug:        bug,
		Moderation: moderation,
		Logger:     slog.Default(),
	}
	if err := addOptionalFunctions(&bc, cfg, def, assembly); err != nil {
		return discord.BotConfig{}, err
	}
	return bc, nil
}

// addOptionalFunctions sets the functions that are off unless configured: the forum answer, the
// news function and the supporter function.
func addOptionalFunctions(bc *discord.BotConfig, cfg config.Config, def *serverdef.Definition, assembly *app.Assembly) error {
	var err error
	if bc.Forum, err = newForum(cfg, assembly); err != nil {
		return err
	}
	if bc.News, err = newNews(cfg, def); err != nil {
		return err
	}
	bc.Supporter, err = newSupporter(cfg, def)
	return err
}

type healthHarness struct {
	state  *health.State
	server *health.Server
	ln     net.Listener
}

func initHealth(ctx context.Context, addr string) (*healthHarness, error) {
	if addr == "" {
		return nil, nil
	}
	ln, err := health.Listen(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", config.EnvHealthAddr, err)
	}
	state := &health.State{}
	return &healthHarness{
		state:  state,
		server: health.NewServer(state, slog.Default()),
		ln:     ln,
	}, nil
}

func (h *healthHarness) attachModel(gate *model.Gate) {
	if h != nil && gate != nil {
		h.state.SetModelState(gate.State)
	}
}

func (h *healthHarness) takeListener() net.Listener {
	if h == nil {
		return nil
	}
	ln := h.ln
	h.ln = nil
	return ln
}

func reportAssemblyStatus(out io.Writer, assembly *app.Assembly) error {
	modelStatus := "not configured"
	if assembly.Model != nil {
		modelStatus = assembly.Model.State()
	}
	if _, writeErr := fmt.Fprintf(out, "ready: %d sources, %d chunks, model %s\n",
		len(assembly.Sources), assembly.Chunks, modelStatus); writeErr != nil {
		return fmt.Errorf("write status: %w", writeErr)
	}
	return nil
}

func runDiscordWithHealth(
	ctx context.Context,
	hh *healthHarness,
	ln net.Listener,
	botCfg discord.BotConfig,
	runDiscord func(ctx context.Context, cfg discord.BotConfig) error,
) error {
	if hh == nil {
		if err := runDiscord(ctx, botCfg); err != nil {
			return fmt.Errorf("run discord bot: %w", err)
		}
		return nil
	}

	botCtx, botCancel := context.WithCancel(ctx)
	defer botCancel()

	botCfg.ReportReady = hh.state.SetReady
	healthErrCh := make(chan error, 1)

	// Exactly one goroutine serves the health endpoint (bounded, HISS-06).
	go func() {
		serveErr := hh.server.Serve(botCtx, ln)
		if serveErr != nil {
			botCancel()
		}
		healthErrCh <- serveErr
	}()

	botErr := runDiscord(botCtx, botCfg)
	botCancel()
	healthErr := <-healthErrCh

	if healthErr != nil {
		if botErr != nil {
			return errors.Join(fmt.Errorf("health server: %w", healthErr), fmt.Errorf("run discord bot: %w", botErr))
		}
		return fmt.Errorf("health server: %w", healthErr)
	}
	if botErr != nil {
		return fmt.Errorf("run discord bot: %w", botErr)
	}
	return nil
}
