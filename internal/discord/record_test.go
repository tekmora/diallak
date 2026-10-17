package discord

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
)

// commandReference is how the compliance documents name a command: a slash command with its
// slash, a message command by its label, both in code spans.
func commandReference(t *testing.T, command discord.ApplicationCommandCreate) string {
	t.Helper()
	switch c := command.(type) {
	case discord.SlashCommandCreate:
		return "`/" + c.Name + "`"
	case discord.MessageCommandCreate:
		return "`" + c.Name + "`"
	default:
		t.Fatalf("command of type %T has no reference form", command)
		return ""
	}
}

func recordEntry(t *testing.T, text, row string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "| "+row+" ") {
			return line
		}
	}
	t.Fatalf("the record of processing has no row %q", row)
	return ""
}

// Article 30(1): every command the bot registers processes member data, so the record names each
// one among its purposes and among its data subjects. A command added without its entry fails.
func TestRecordOfProcessing_Negative_EveryCommandIsNamed(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "compliance", "record-of-processing.md"))
	if err != nil {
		t.Fatalf("read the record of processing: %v", err)
	}
	purposes := recordEntry(t, string(data), "(b) Purposes")
	subjects := recordEntry(t, string(data), "(c) Data subjects and data")
	commands := guildCommands(0)
	if len(commands) == 0 {
		t.Fatal("guildCommands returned no command")
	}
	for _, command := range commands {
		reference := commandReference(t, command)
		if !strings.Contains(purposes, reference) {
			t.Errorf("(b) Purposes does not name %s", reference)
		}
		if !strings.Contains(subjects, reference) {
			t.Errorf("(c) Data subjects and data does not name %s", reference)
		}
	}
}
