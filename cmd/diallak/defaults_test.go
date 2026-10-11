package main

import (
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/tekmora/diallak/internal/answer"
	"github.com/tekmora/diallak/internal/config"
	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/discord"
	"github.com/tekmora/diallak/internal/model"
	"github.com/tekmora/diallak/internal/retrieval"
)

// The built-in values of the optional settings are part of the bot's behaviour: a deployment
// that sets none of them must keep answering the same way. This test pins each value, so that
// changing one is a deliberate edit here as well.
func TestDefaults_Positive_BuiltInValuesArePinned(t *testing.T) {
	pins := []struct {
		name string
		got  any
		want any
	}{
		{"passages", retrieval.DefaultLimit, 3},
		{"score", retrieval.DefaultMinScore, 0.20},
		{"question length", answer.MaxQuestionRunes, 500},
		{"excerpt length", answer.DefaultMaxExcerptRunes, 400},
		{"reply length", answer.DefaultMaxRenderRunes, 1900},
		{"concurrent asks", discord.DefaultMaxConcurrent, 4},
		{"ask timeout", discord.DefaultAskTimeout, 60 * time.Second},
		{"model timeout", model.DefaultTimeout, 45 * time.Second},
		{"source link", corpus.DefaultSourceLinkTemplate, "https://github.com/{repository}/blob/{revision}/{path}"},
	}
	for _, p := range pins {
		if p.got != p.want {
			t.Errorf("built-in %s = %v, want %v", p.name, p.got, p.want)
		}
	}
}

// The README tells an operator the default and the range of every optional setting. This test
// builds each table row from the constants the code uses, so the table cannot say something the
// code does not do.
func TestReadme_Negative_OptionalSettingsTableMatchesTheCode(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	rows := []struct {
		variable      string
		defaultAndRng string
	}{
		{config.EnvTopPassages, fmt.Sprintf("`%d` | `1` to `%d`", retrieval.DefaultLimit, retrieval.MaxLimit)},
		{config.EnvMinScore, fmt.Sprintf("`%.2f` | above `0`, at most `1`", retrieval.DefaultMinScore)},
		{config.EnvQuestionMaxChars, fmt.Sprintf("`%d` | `1` to `%d`", answer.MaxQuestionRunes, answer.MaxQuestionRunes)},
		{config.EnvExcerptChars, fmt.Sprintf("`%d` | `%d` to `%d`", answer.DefaultMaxExcerptRunes, answer.MinExcerptRunes, answer.MaxExcerptRunesLimit)},
		{config.EnvReplyMaxChars, fmt.Sprintf("`%d` | `%d` to `%d`", answer.DefaultMaxRenderRunes, answer.MinRenderRunes, answer.MaxAllowedRenderRunes)},
		{config.EnvMaxConcurrentAsks, fmt.Sprintf("`%d` | `%d` to `%d`", discord.DefaultMaxConcurrent, config.MinConcurrentAsks, config.MaxConcurrentAsksLimit)},
		{config.EnvAskTimeout, fmt.Sprintf("`%s` | `%s` to `%s`", short(discord.DefaultAskTimeout), short(config.MinAskTimeout), short(config.MaxAskTimeout))},
		{config.EnvModelTimeout, fmt.Sprintf("`%s` | `%s` to `%s`, and below the ask timeout", short(model.DefaultTimeout), short(model.MinTimeout), short(model.MaxTimeout))},
		{config.EnvSourceLinkTemplate, fmt.Sprintf("`%s` | an `https://` address that holds each of the three placeholders once, after the host, at most %d characters", corpus.DefaultSourceLinkTemplate, corpus.MaxSourceLinkTemplateRunes)},
	}
	for _, row := range rows {
		pattern := `(?m)^\| ` + regexp.QuoteMeta("`"+row.variable+"`") + ` \| [^|]+ \| ` + regexp.QuoteMeta(row.defaultAndRng) + ` \|$`
		if !regexp.MustCompile(pattern).Match(readme) {
			t.Errorf("README has no row for %s with default and range %q", row.variable, row.defaultAndRng)
		}
	}
}

// short writes a duration the way the README does: 60s, 10m, 5m.
func short(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 && d != time.Minute {
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return fmt.Sprintf("%ds", d/time.Second)
}
