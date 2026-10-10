// Command discord-bot is the community bot for the hub server.
//
// This skeleton validates its configuration and exits. The gateway connection arrives with the
// first feature slice.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/tekmora/discord-bot/internal/config"
)

func main() {
	os.Exit(report(run(os.Getenv, os.Stdout), os.Stderr))
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

// run loads the configuration and reports it without ever printing the token.
func run(getenv func(string) string, out io.Writer) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if _, err := fmt.Fprintf(out, "configuration valid for guild %d; this skeleton opens no gateway connection\n", cfg.GuildID); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	return nil
}
