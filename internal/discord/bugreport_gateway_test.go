package discord

import (
	"log/slog"
	"strings"
	"testing"
)

// The test in this file starts the bot as Run does, with the bug command, and hands its client the
// INTERACTION_CREATE frame of /bug as in areas_gateway_test.go.

func bugCommandFrame(t *testing.T, seq int, id uint64, title, steps string) []byte {
	t.Helper()
	data := map[string]any{"id": "750000000000000005", "name": BugCommandName, "type": 1, "options": []any{
		map[string]any{"name": OptionBugTitle, "type": 3, "value": title},
		map[string]any{"name": OptionBugSteps, "type": 3, "value": steps},
	}}
	return areasFrame(t, seq, id, 2, nil, data, nil)
}

func TestRunBug_Positive_CommandAnswersWithEphemeralIssueLink(t *testing.T) {
	bug, err := NewBugHandler(BugConfig{
		Repository: "tekmora/diallak", GuildID: areasGuildID, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("NewBugHandler: %v", err)
	}
	fake := &areasFake{t: t, answers: make(chan struct{}, 4)}
	calls := runComponentsOverGateway(t, fake, BotConfig{Bug: bug}, 1,
		bugCommandFrame(t, 1, 770000000000000001, "Crash on ask", "1. run ask"))

	var callback *areasCall
	for i := range calls {
		if calls[i].method == "POST" && strings.HasPrefix(calls[i].path, "/interactions/") {
			callback = &calls[i]
		}
	}
	if callback == nil {
		t.Fatalf("no interaction callback; requests: %+v", calls)
	}
	prefix := TextBugIntro + "\nhttps://github.com/tekmora/diallak/issues/new?"
	if !strings.HasPrefix(callback.body.Data.Content, prefix) {
		t.Errorf("reply %q does not start with %q", callback.body.Data.Content, prefix)
	}
	if callback.body.Data.Flags&64 == 0 {
		t.Errorf("reply flags = %d, want ephemeral (64)", callback.body.Data.Flags)
	}
}
