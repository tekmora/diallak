package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/tekmora/diallak/internal/config"
)

func TestBuild_Positive_ReadyLogListsEverySourceOnce(t *testing.T) {
	dir := setupTestCorpus(t)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	_, cleanup, err := Build(context.Background(), config.Config{CorpusDir: dir, GuildID: 1, Token: "t"}, logger)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := cleanup(); closeErr != nil {
			t.Errorf("cleanup: %v", closeErr)
		}
	})

	// One object key per source would repeat the key and make the line ambiguous JSON, so the
	// inventory is a list.
	var line struct {
		Msg       string   `json:"msg"`
		Sources   int      `json:"sources"`
		Inventory []string `json:"inventory"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("ready line is not the expected JSON: %v\n%s", err, buf.String())
	}
	want := []string{"tekmora/alpha@abcdef123456", "tekmora/beta@123456abcdef"}
	if line.Sources != 2 || strings.Join(line.Inventory, " ") != strings.Join(want, " ") {
		t.Fatalf("ready line = %+v, want sources 2 and inventory %v", line, want)
	}
	if strings.Count(buf.String(), `"source":`) != 0 {
		t.Fatalf("ready line repeats a key per source: %s", buf.String())
	}
}
