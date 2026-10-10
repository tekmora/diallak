// Package config loads the bot's settings from the process environment.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// EnvTokenFile names the variable holding the path of the Discord bot token file.
	EnvTokenFile = "DIALLAK_TOKEN_FILE" // #nosec G101 -- the name of an environment variable, not a credential.
	// EnvGuildID names the variable holding the snowflake of the hub guild.
	EnvGuildID = "DIALLAK_GUILD_ID"
	// EnvModelBaseURL names the variable holding the base URL of the model endpoint.
	EnvModelBaseURL = "DIALLAK_MODEL_BASE_URL"
	// EnvModelAlias names the variable holding the model routing alias.
	EnvModelAlias = "DIALLAK_MODEL_ALIAS"
	// EnvModelKeyFile names the variable holding the path of the optional model API key file.
	EnvModelKeyFile = "DIALLAK_MODEL_KEY_FILE" // #nosec G101 -- the name of an environment variable, not a credential.
	// EnvCorpusDir names the variable holding the absolute path of the documentation corpus directory.
	EnvCorpusDir = "DIALLAK_CORPUS_DIR"
	// EnvSupportChannel names the optional variable naming where a person helps.
	EnvSupportChannel = "DIALLAK_SUPPORT_CHANNEL"
	// MaxTokenBytes bounds secret files (both bot token and model key).
	MaxTokenBytes = 4096
	// MaxSupportChannelRunes is the maximum allowed length of the support channel name in runes.
	MaxSupportChannelRunes = 64
)

// Config is the validated configuration of one bot process.
type Config struct {
	// Token is the Discord bot token. It comes from a file, never from the environment itself,
	// and must never be logged. It is left out of JSON encodings.
	Token string `json:"-"`
	// GuildID is the snowflake of the hub guild the bot serves.
	GuildID uint64
	// ModelBaseURL is the base URL of the OpenAI-compatible model endpoint.
	ModelBaseURL string
	// ModelAlias is the routing name passed in model completion requests.
	ModelAlias string
	// ModelKey is the optional API key for the model endpoint. It is never logged and is left out
	// of JSON encodings.
	ModelKey string `json:"-"`
	// CorpusDir is the absolute path to the local directory holding the corpus manifest and sources.
	CorpusDir string
	// SupportChannel names where a person helps, as replies word it. It is empty when the
	// variable is not set; replies then use their built-in wording.
	SupportChannel string
}

// HasModel reports whether a model endpoint is configured.
func (c Config) HasModel() bool {
	return c.ModelBaseURL != "" && c.ModelAlias != ""
}

// String returns a redacted representation of Config. Token and ModelKey are never printed.
func (c Config) String() string {
	return fmt.Sprintf("Config{Token:%q, GuildID:%d, ModelBaseURL:%q, ModelAlias:%q, ModelKey:%q, CorpusDir:%q, SupportChannel:%q}",
		redactSecret(c.Token), c.GuildID, c.ModelBaseURL, c.ModelAlias, redactSecret(c.ModelKey), c.CorpusDir, c.SupportChannel)
}

// GoString returns a redacted representation of Config for Go syntax formatting.
func (c Config) GoString() string {
	return c.String()
}

// LogValue implements slog.LogValuer to ensure secrets are redacted in structured logs.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("Token", redactSecret(c.Token)),
		slog.Uint64("GuildID", c.GuildID),
		slog.String("ModelBaseURL", c.ModelBaseURL),
		slog.String("ModelAlias", c.ModelAlias),
		slog.String("ModelKey", redactSecret(c.ModelKey)),
		slog.String("CorpusDir", c.CorpusDir),
		slog.String("SupportChannel", c.SupportChannel),
	)
}

func redactSecret(s string) string {
	if s == "" {
		return ""
	}
	return "[redacted]"
}

// Load reads the configuration through getenv. It fails closed: a missing variable, an
// unreadable, empty or oversized secret file, an invalid corpus directory, or invalid
// model configuration is an error.
func Load(getenv func(string) string) (Config, error) {
	guild, err := parseGuildID(getenv(EnvGuildID))
	if err != nil {
		return Config{}, err
	}
	token, err := readSecretFile(getenv(EnvTokenFile), EnvTokenFile, "token")
	if err != nil {
		return Config{}, err
	}
	corpusDir, err := parseCorpusDir(getenv(EnvCorpusDir))
	if err != nil {
		return Config{}, err
	}
	baseURL, alias, key, err := parseModelConfig(getenv)
	if err != nil {
		return Config{}, err
	}
	supportChannel, err := parseSupportChannel(getenv(EnvSupportChannel))
	if err != nil {
		return Config{}, err
	}
	return Config{
		Token:          token,
		GuildID:        guild,
		ModelBaseURL:   baseURL,
		ModelAlias:     alias,
		ModelKey:       key,
		CorpusDir:      corpusDir,
		SupportChannel: supportChannel,
	}, nil
}

func parseCorpusDir(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%s is not set", EnvCorpusDir)
	}
	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("%s must be an absolute path: %q", EnvCorpusDir, raw)
	}
	return raw, nil
}

func parseModelConfig(getenv func(string) string) (string, string, string, error) {
	baseURL := getenv(EnvModelBaseURL)
	alias := getenv(EnvModelAlias)
	keyFile := getenv(EnvModelKeyFile)

	hasBase := baseURL != ""
	hasAlias := alias != ""
	if hasBase != hasAlias {
		return "", "", "", fmt.Errorf("%s and %s must both be set or both be unset", EnvModelBaseURL, EnvModelAlias)
	}
	if !hasBase {
		if keyFile != "" {
			return "", "", "", fmt.Errorf("%s is set but no model is configured", EnvModelKeyFile)
		}
		return "", "", "", nil
	}

	var key string
	if keyFile != "" {
		readKey, err := readSecretFile(keyFile, EnvModelKeyFile, "model key")
		if err != nil {
			return "", "", "", err
		}
		key = readKey
	}
	return baseURL, alias, key, nil
}

func parseGuildID(raw string) (uint64, error) {
	if raw == "" {
		return 0, fmt.Errorf("%s is not set", EnvGuildID)
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is not a decimal snowflake: %w", EnvGuildID, err)
	}
	if id == 0 {
		return 0, fmt.Errorf("%s must not be zero", EnvGuildID)
	}
	return id, nil
}

func readSecretFile(path, envVar, description string) (secret string, err error) {
	if path == "" {
		return "", fmt.Errorf("%s is not set", envVar)
	}
	file, err := os.Open(path) // #nosec G304 -- the operator names the secret file.
	if err != nil {
		return "", fmt.Errorf("open %s file: %w", description, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close %s file: %w", description, closeErr))
		}
	}()
	data, err := io.ReadAll(io.LimitReader(file, MaxTokenBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s file: %w", description, err)
	}
	if len(data) > MaxTokenBytes {
		return "", fmt.Errorf("%s file is larger than %d bytes", description, MaxTokenBytes)
	}
	secret = strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("%s file is empty", description)
	}
	return secret, nil
}

// parseSupportChannel validates the optional text that names where a person helps. The text is
// put into every reply, so it may hold printable characters only: a line break or an invisible
// character would move or hide the notice it is part of. An "@" is refused so that the text can
// never read as a mention. Errors name the variable and never the value.
func parseSupportChannel(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%s is whitespace-only", EnvSupportChannel)
	}
	for _, r := range raw {
		if !unicode.IsPrint(r) {
			return "", fmt.Errorf("%s holds a character that is not printable", EnvSupportChannel)
		}
	}
	if strings.Contains(raw, "@") {
		return "", fmt.Errorf("%s contains '@'", EnvSupportChannel)
	}
	if utf8.RuneCountInString(trimmed) > MaxSupportChannelRunes {
		return "", fmt.Errorf("%s exceeds %d runes", EnvSupportChannel, MaxSupportChannelRunes)
	}
	return trimmed, nil
}
