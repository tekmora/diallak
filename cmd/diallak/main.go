// Command diallak is the community bot for the hub server.
//
// It serves guild command /ask answered by the documentation pipeline and can
// register commands via -register-commands.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tekmora/diallak/internal/app"
	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/discord"
)

const (
	startTimeout    = 30 * time.Second
	registerTimeout = 30 * time.Second
)

type dependencies struct {
	registerCommands func(ctx context.Context, token string, guildID uint64) error
	runDiscord       func(ctx context.Context, cfg discord.BotConfig) error
}

func defaultDependencies() dependencies {
	return dependencies{
		registerCommands: discord.RegisterCommands,
		runDiscord:       discord.Run,
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

// run parses arguments, loads configuration, and executes either command registration or the bot runtime.
func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer, deps dependencies) (err error) {
	fs := flag.NewFlagSet("diallak", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerCmds := fs.Bool("register-commands", false, "register guild commands and exit")
	if parseErr := fs.Parse(args); parseErr != nil {
		return fmt.Errorf("parse flags: %w", parseErr)
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	if *registerCmds {
		return executeRegisterCommands(ctx, cfg, out, deps.registerCommands)
	}
	return executeBotRun(ctx, cfg, out, deps.runDiscord)
}

func executeRegisterCommands(
	ctx context.Context,
	cfg config.Config,
	out io.Writer,
	register func(ctx context.Context, token string, guildID uint64) error,
) error {
	regCtx, cancel := context.WithTimeout(ctx, registerTimeout)
	defer cancel()

	if err := register(regCtx, cfg.Token, cfg.GuildID); err != nil {
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

	modelStatus := "not configured"
	if assembly.HasModel {
		modelStatus = "configured"
	}

	if _, writeErr := fmt.Fprintf(out, "ready: %d sources, %d chunks, model %s\n",
		len(assembly.Sources), assembly.Chunks, modelStatus); writeErr != nil {
		return fmt.Errorf("write status: %w", writeErr)
	}

	handler, err := discord.NewHandler(discord.HandlerConfig{
		Asker:          assembly.Pipeline,
		GuildID:        cfg.GuildID,
		Logger:         slog.Default(),
		SupportChannel: cfg.SupportChannel,
	})
	if err != nil {
		return fmt.Errorf("create discord handler: %w", err)
	}

	botCfg := discord.BotConfig{
		Token:   cfg.Token,
		GuildID: cfg.GuildID,
		Handler: handler,
		Logger:  slog.Default(),
	}

	if err := runDiscord(ctx, botCfg); err != nil {
		return fmt.Errorf("run discord bot: %w", err)
	}
	return nil
}
