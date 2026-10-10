// Command discord-bot is the community bot for the hub server.
//
// This build validates configuration, assembles the corpus index and answer pipeline,
// and exits. The gateway connection arrives with the first feature slice.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tekmora/discord-bot/internal/app"
	"github.com/tekmora/discord-bot/internal/config"
)

const startTimeout = 30 * time.Second

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	os.Exit(report(run(ctx, os.Getenv, os.Stdout), os.Stderr))
}

// report prints err to errOut and returns the process exit code: 0 without an error, 1 with a
// reported one, and 2 when the report itself could not be written.
func report(err error, errOut io.Writer) int {
	if err == nil {
		return 0
	}
	if _, writeErr := fmt.Fprintln(errOut, "discord-bot:", err); writeErr != nil {
		return 2
	}
	return 1
}

// run loads configuration and builds the application without opening a gateway connection.
func run(ctx context.Context, getenv func(string) string, out io.Writer) (err error) {
	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
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

	modelStatus := "not configured"
	if assembly.HasModel {
		modelStatus = "configured"
	}

	if _, writeErr := fmt.Fprintf(out, "ready: %d sources, %d chunks, model %s; this build opens no gateway connection\n",
		len(assembly.Sources), assembly.Chunks, modelStatus); writeErr != nil {
		return fmt.Errorf("write status: %w", writeErr)
	}
	return nil
}
