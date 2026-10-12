//go:build semgrep_fixture

package fixture

import (
	"context"
	"fmt"
	"os/exec"
)

const shell = "/bin/sh"

func commands(ctx context.Context, tool, arg string) []*exec.Cmd {
	return []*exec.Cmd{
		// ruleid: no-dynamic-exec-command
		exec.Command("sh", "-c", arg),
		// ruleid: no-dynamic-exec-command
		exec.CommandContext(ctx, "/usr/bin/bash", "-c", arg),
		// ruleid: no-dynamic-exec-command
		exec.Command(shell, "-c", arg),
		// ruleid: no-dynamic-exec-command
		exec.Command("/opt/" + tool),
		// ruleid: no-dynamic-exec-command
		exec.CommandContext(ctx, fmt.Sprintf("%s-helper", tool)),
		// ok: no-dynamic-exec-command
		exec.Command("git", "status", arg),
		// ok: no-dynamic-exec-command
		exec.CommandContext(ctx, "/usr/bin/git", "log"),
		// ok: no-dynamic-exec-command
		exec.Command("shellcheck", arg),
	}
}
