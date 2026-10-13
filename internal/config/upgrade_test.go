package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The files under testdata/upgrade are the configurations of earlier releases,
// each accepted by the release it names. An upgrade starts this release on
// them unchanged: it reads every value the earlier release read, or it stops
// with the error that a breaking change's Migration footer describes.

const (
	upgradeToken = "fixture-token"
	upgradeKey   = "fixture-key"
)

// readUpgradeFixture returns the variables of one fixture, with the token and
// key file placeholders replaced by files that hold upgradeToken and upgradeKey.
func readUpgradeFixture(t *testing.T, name string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "upgrade", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	files := strings.NewReplacer(
		"@TOKEN_FILE@", writeSecret(t, "token", upgradeToken+"\n"),
		"@KEY_FILE@", writeSecret(t, "key", upgradeKey+"\n"),
	)
	vars := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if _, seen := vars[key]; !ok || seen {
			t.Fatalf("fixture %s: line %q is not a new KEY=VALUE", name, line)
		}
		vars[key] = files.Replace(value)
	}
	return vars
}

func TestLoad_Positive_UpgradeKeepsTheConfigurationOfEarlierReleases(t *testing.T) {
	cases := []struct {
		fixture string
		want    Config
	}{
		{"v0.1.2-minimal.env", Config{
			Token: upgradeToken, GuildID: 123456789012345678, CorpusDir: "/srv/diallak/corpus",
		}},
		{"v0.1.2-excerpts.env", Config{
			Token: upgradeToken, GuildID: 123456789012345678, CorpusDir: "/srv/diallak/corpus",
			SupportChannel: "#help", HealthAddr: "0.0.0.0:8080",
			TopPassages: 5, MinScore: 0.35, QuestionMaxChars: 400, ExcerptChars: 300, ReplyMaxChars: 1800,
			MaxConcurrentAsks: 8, AskTimeout: 90 * time.Second, ModelTimeout: 30 * time.Second,
			SourceLinkTemplate: "https://git.example.org/{repository}/src/{revision}/{path}",
		}},
		{"v0.4.2-model.env", Config{
			Token: upgradeToken, GuildID: 123456789012345678, CorpusDir: "/srv/diallak/corpus",
			ModelBaseURL: "https://gateway.example.org/v1", ModelAlias: "docs-answer",
			ModelPolicy: "docs-guardrails/v1", ModelKey: upgradeKey,
			SupportChannel: "#help", HealthAddr: "127.0.0.1:9090",
			TopPassages: 10, MinScore: 1, QuestionMaxChars: 500, ExcerptChars: 1000, ReplyMaxChars: 2000,
			MaxConcurrentAsks: 64, AskTimeout: 10 * time.Minute, ModelTimeout: 5 * time.Minute,
			SourceLinkTemplate: "https://github.com/{repository}/blob/{revision}/{path}?plain=1",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			got, err := Load(env(readUpgradeFixture(t, tc.fixture)))
			if err != nil {
				t.Fatalf("Load: %v, want the configuration of the earlier release accepted", err)
			}
			if got != tc.want {
				t.Errorf("Load read\n%#v\nwant\n%#v", got, tc.want)
			}
		})
	}
}

func TestLoad_Negative_UpgradeStopsOnlyWhereAMigrationIsDocumented(t *testing.T) {
	cases := []struct {
		fixture, want string
	}{
		// 4a7d947 (v0.3.0): a model needs the gateway policy.
		{"v0.1.2-model.env", EnvModelPolicy + " is not set"},
		// d87dd04 (v0.2.0): the host of a source link is written literally.
		{"v0.1.2-template-host.env", "holds a placeholder before the path"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			_, err := Load(env(readUpgradeFixture(t, tc.fixture)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load: error %v, want the documented migration %q", err, tc.want)
			}
		})
	}
}

func TestLoad_Boundary_UpgradeFromTheLatestReleaseSetsEveryVariable(t *testing.T) {
	vars := readUpgradeFixture(t, "v0.4.2-model.env")
	names := []string{
		EnvTokenFile, EnvGuildID, EnvModelBaseURL, EnvModelAlias, EnvModelPolicy, EnvModelKeyFile,
		EnvCorpusDir, EnvSupportChannel, EnvTopPassages, EnvMinScore, EnvQuestionMaxChars,
		EnvExcerptChars, EnvReplyMaxChars, EnvMaxConcurrentAsks, EnvAskTimeout, EnvModelTimeout,
		EnvSourceLinkTemplate, EnvHealthAddr,
	}
	for _, name := range names {
		if vars[name] == "" {
			t.Errorf("the latest release's fixture does not set %s, so an upgrade would not read it", name)
		}
	}
	if len(vars) != len(names) {
		t.Errorf("the latest release's fixture sets %d variables, this release reads %d", len(vars), len(names))
	}
}
