// Package config loads the bot's settings from the process environment.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tekmora/diallak/internal/answer"
	"github.com/tekmora/diallak/internal/corpus"
	"github.com/tekmora/diallak/internal/model"
	"github.com/tekmora/diallak/internal/retrieval"
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
	// EnvTopPassages names the optional variable setting the maximum passages retrieved.
	EnvTopPassages = "DIALLAK_TOP_PASSAGES" // #nosec G101 -- the name of an environment variable, not a credential.
	// EnvMinScore names the optional variable setting the minimum score threshold.
	EnvMinScore = "DIALLAK_MIN_SCORE"
	// EnvQuestionMaxChars names the optional variable setting the maximum question length.
	EnvQuestionMaxChars = "DIALLAK_QUESTION_MAX_CHARS"
	// EnvExcerptChars names the optional variable setting the maximum excerpt length.
	EnvExcerptChars = "DIALLAK_EXCERPT_CHARS"
	// EnvReplyMaxChars names the optional variable setting the maximum reply length.
	EnvReplyMaxChars = "DIALLAK_REPLY_MAX_CHARS"
	// EnvMaxConcurrentAsks names the optional variable setting maximum concurrent asks.
	EnvMaxConcurrentAsks = "DIALLAK_MAX_CONCURRENT_ASKS"
	// EnvAskTimeout names the optional variable setting the inquiry processing timeout.
	EnvAskTimeout = "DIALLAK_ASK_TIMEOUT"
	// EnvModelTimeout names the optional variable setting the model completion timeout.
	EnvModelTimeout = "DIALLAK_MODEL_TIMEOUT"
	// EnvSourceLinkTemplate names the optional variable setting the citation URL template.
	EnvSourceLinkTemplate = "DIALLAK_SOURCE_LINK_TEMPLATE"
	// EnvHealthAddr names the optional variable setting the health probe listen address.
	EnvHealthAddr = "DIALLAK_HEALTH_ADDR"

	// MaxTokenBytes bounds secret files (both bot token and model key).
	MaxTokenBytes = 4096
	// MaxSupportChannelRunes is the maximum allowed length of the support channel name in runes.
	MaxSupportChannelRunes = 64

	// MinConcurrentAsks is the lower bound for DIALLAK_MAX_CONCURRENT_ASKS.
	MinConcurrentAsks = 1
	// MaxConcurrentAsksLimit is the upper bound for DIALLAK_MAX_CONCURRENT_ASKS.
	MaxConcurrentAsksLimit = 64
	// MinAskTimeout is the lower bound for DIALLAK_ASK_TIMEOUT.
	MinAskTimeout = 1 * time.Second
	// MaxAskTimeout is the upper bound for DIALLAK_ASK_TIMEOUT.
	MaxAskTimeout = 10 * time.Minute
	// DefaultAskTimeout is the default duration for DIALLAK_ASK_TIMEOUT.
	DefaultAskTimeout = 60 * time.Second
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
	// TopPassages is the maximum number of retrieved passages offered to the model or returned as excerpts.
	// It is 0 when the variable is not set; retrieval then uses its built-in default.
	TopPassages int
	// MinScore is the minimum lexical match score for retrieval hits.
	// It is 0 when the variable is not set; retrieval then uses its built-in default.
	MinScore float64
	// QuestionMaxChars is the maximum question length in characters.
	// It is 0 when the variable is not set; answer and discord then use their built-in default.
	QuestionMaxChars int
	// ExcerptChars is the maximum length of documentation excerpts in characters.
	// It is 0 when the variable is not set; answer then uses its built-in default.
	ExcerptChars int
	// ReplyMaxChars is the maximum rendered reply length in characters.
	// It is 0 when the variable is not set; answer then uses its built-in default.
	ReplyMaxChars int
	// MaxConcurrentAsks is the maximum number of concurrent asks processed by the bot.
	// It is 0 when the variable is not set; discord then uses its built-in default.
	MaxConcurrentAsks int
	// AskTimeout is the maximum duration to process one ask inquiry.
	// It is 0 when the variable is not set; discord then uses its built-in default.
	AskTimeout time.Duration
	// ModelTimeout is the maximum duration for completion requests to the model endpoint.
	// It is 0 when the variable is not set; model then uses its built-in default.
	ModelTimeout time.Duration
	// SourceLinkTemplate is the URL template for document links in citations.
	// It is empty when the variable is not set; corpus then uses its built-in default.
	SourceLinkTemplate string
	// HealthAddr is the optional TCP address ("host:port") for the health and readiness HTTP listener.
	// It is empty when the variable is not set; no listener is then started.
	HealthAddr string
}

// HasModel reports whether a model endpoint is configured.
func (c Config) HasModel() bool {
	return c.ModelBaseURL != "" && c.ModelAlias != ""
}

// String returns a redacted representation of Config. Token and ModelKey are never printed.
func (c Config) String() string {
	return fmt.Sprintf("Config{Token:%q, GuildID:%d, ModelBaseURL:%q, ModelAlias:%q, ModelKey:%q, CorpusDir:%q, SupportChannel:%q, TopPassages:%d, MinScore:%g, QuestionMaxChars:%d, ExcerptChars:%d, ReplyMaxChars:%d, MaxConcurrentAsks:%d, AskTimeout:%v, ModelTimeout:%v, SourceLinkTemplate:%q, HealthAddr:%q}",
		redactSecret(c.Token), c.GuildID, c.ModelBaseURL, c.ModelAlias, redactSecret(c.ModelKey), c.CorpusDir, c.SupportChannel,
		c.TopPassages, c.MinScore, c.QuestionMaxChars, c.ExcerptChars, c.ReplyMaxChars, c.MaxConcurrentAsks,
		c.AskTimeout, c.ModelTimeout, c.SourceLinkTemplate, c.HealthAddr)
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
		slog.Int("TopPassages", c.TopPassages),
		slog.Float64("MinScore", c.MinScore),
		slog.Int("QuestionMaxChars", c.QuestionMaxChars),
		slog.Int("ExcerptChars", c.ExcerptChars),
		slog.Int("ReplyMaxChars", c.ReplyMaxChars),
		slog.Int("MaxConcurrentAsks", c.MaxConcurrentAsks),
		slog.Duration("AskTimeout", c.AskTimeout),
		slog.Duration("ModelTimeout", c.ModelTimeout),
		slog.String("SourceLinkTemplate", c.SourceLinkTemplate),
		slog.String("HealthAddr", c.HealthAddr),
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
	passages, minScore, qMax, excerpt, reply, err := parseOptionalLimits(getenv)
	if err != nil {
		return Config{}, err
	}
	concurrent, askTimeout, modelTimeout, linkTmpl, err := parseOptionalTimeoutsAndTemplate(getenv, baseURL != "")
	if err != nil {
		return Config{}, err
	}
	healthAddr, err := parseHealthAddr(getenv(EnvHealthAddr))
	if err != nil {
		return Config{}, err
	}
	return Config{
		Token:              token,
		GuildID:            guild,
		ModelBaseURL:       baseURL,
		ModelAlias:         alias,
		ModelKey:           key,
		CorpusDir:          corpusDir,
		SupportChannel:     supportChannel,
		TopPassages:        passages,
		MinScore:           minScore,
		QuestionMaxChars:   qMax,
		ExcerptChars:       excerpt,
		ReplyMaxChars:      reply,
		MaxConcurrentAsks:  concurrent,
		AskTimeout:         askTimeout,
		ModelTimeout:       modelTimeout,
		SourceLinkTemplate: linkTmpl,
		HealthAddr:         healthAddr,
	}, nil
}

// parseHealthAddr validates the optional listen address of the health listener. The host is
// empty or an IP literal, never a name, so that starting the listener needs no name lookup. The
// port is written in one way only: decimal, 1 to 65535, without sign or leading zero.
func parseHealthAddr(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return "", fmt.Errorf("%s must be in 'host:port' format: %w", EnvHealthAddr, err)
	}
	portNum, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNum == 0 || strconv.FormatUint(portNum, 10) != port {
		return "", fmt.Errorf("%s port must be a number from 1 to 65535 without leading zero: %q", EnvHealthAddr, port)
	}
	if host == "" {
		return raw, nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("%s host is not an IP literal: %q", EnvHealthAddr, host)
	}
	// SplitHostPort takes brackets around any host. They belong to IPv6 only.
	if ip.To4() != nil && strings.HasPrefix(raw, "[") {
		return "", fmt.Errorf("%s IPv4 host must not be enclosed in brackets", EnvHealthAddr)
	}
	return raw, nil
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

func parseOptionalLimits(getenv func(string) string) (int, float64, int, int, int, error) {
	passages, err := parseOptionalInt(getenv(EnvTopPassages), EnvTopPassages, 1, retrieval.MaxLimit)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	minScore, err := parseOptionalFloat(getenv(EnvMinScore), EnvMinScore, 0.0, 1.0)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	qMax, err := parseOptionalInt(getenv(EnvQuestionMaxChars), EnvQuestionMaxChars, 1, answer.MaxQuestionRunes)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	excerpt, err := parseOptionalInt(getenv(EnvExcerptChars), EnvExcerptChars, answer.MinExcerptRunes, answer.MaxExcerptRunesLimit)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	reply, err := parseOptionalInt(getenv(EnvReplyMaxChars), EnvReplyMaxChars, answer.MinRenderRunes, answer.MaxAllowedRenderRunes)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return passages, minScore, qMax, excerpt, reply, nil
}

func parseOptionalTimeoutsAndTemplate(getenv func(string) string, hasModel bool) (int, time.Duration, time.Duration, string, error) {
	concurrent, err := parseOptionalInt(getenv(EnvMaxConcurrentAsks), EnvMaxConcurrentAsks, MinConcurrentAsks, MaxConcurrentAsksLimit)
	if err != nil {
		return 0, 0, 0, "", err
	}
	askTimeout, err := parseOptionalDuration(getenv(EnvAskTimeout), EnvAskTimeout, MinAskTimeout, MaxAskTimeout)
	if err != nil {
		return 0, 0, 0, "", err
	}
	modelTimeout, err := parseOptionalDuration(getenv(EnvModelTimeout), EnvModelTimeout, model.MinTimeout, model.MaxTimeout)
	if err != nil {
		return 0, 0, 0, "", err
	}
	if err := validateTimeoutCrossConstraint(modelTimeout, askTimeout, hasModel); err != nil {
		return 0, 0, 0, "", err
	}
	linkTmpl, err := parseSourceLinkTemplate(getenv(EnvSourceLinkTemplate))
	if err != nil {
		return 0, 0, 0, "", err
	}
	return concurrent, askTimeout, modelTimeout, linkTmpl, nil
}

func parseOptionalInt(raw, envVar string, minVal, maxVal int) (int, error) {
	if raw == "" {
		return 0, nil
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer between %d and %d: %q", envVar, minVal, maxVal, raw)
	}
	if val < minVal || val > maxVal {
		return 0, fmt.Errorf("%s (%s) outside accepted range [%d, %d]", envVar, raw, minVal, maxVal)
	}
	return val, nil
}

func parseOptionalFloat(raw, envVar string, minValExclusive, maxValInclusive float64) (float64, error) {
	if raw == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a decimal number above %g and at most %g: %q", envVar, minValExclusive, maxValInclusive, raw)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f <= minValExclusive || f > maxValInclusive {
		return 0, fmt.Errorf("%s (%s) outside accepted range: above %g and at most %g", envVar, raw, minValExclusive, maxValInclusive)
	}
	return f, nil
}

func parseOptionalDuration(raw, envVar string, minVal, maxVal time.Duration) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration between %v and %v: %q", envVar, minVal, maxVal, raw)
	}
	if d < minVal || d > maxVal {
		return 0, fmt.Errorf("%s (%s) outside accepted range [%v, %v]", envVar, raw, minVal, maxVal)
	}
	return d, nil
}

func parseSourceLinkTemplate(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if utf8.RuneCountInString(raw) > corpus.MaxSourceLinkTemplateRunes {
		return "", fmt.Errorf("%s exceeds %d characters", EnvSourceLinkTemplate, corpus.MaxSourceLinkTemplateRunes)
	}
	if !strings.HasPrefix(raw, "https://") {
		return "", fmt.Errorf("%s must start with 'https://'", EnvSourceLinkTemplate)
	}
	if err := checkSourceLinkCharacters(raw); err != nil {
		return "", err
	}
	if err := checkSourceLinkPlaceholders(raw); err != nil {
		return "", err
	}
	// The address check reads the template as written; a placeholder may stand in the path,
	// the query or the fragment only, so the host of every link is the one the operator wrote
	// and no value filled in later can add a user name.
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("%s is not an https address with a host and without a user name, or holds a placeholder before the path", EnvSourceLinkTemplate)
	}
	return raw, nil
}

// checkSourceLinkCharacters refuses a template that holds a space, an angle bracket or a
// character that is not printable.
func checkSourceLinkCharacters(raw string) error {
	for _, r := range raw {
		// A reply puts the link between angle brackets, so neither may be part of it.
		if r == ' ' || r == '<' || r == '>' || !unicode.IsPrint(r) {
			return fmt.Errorf("%s holds a space, an angle bracket or a character that is not printable", EnvSourceLinkTemplate)
		}
	}
	return nil
}

// checkSourceLinkPlaceholders refuses a template that does not hold each of the three
// placeholders exactly once.
func checkSourceLinkPlaceholders(raw string) error {
	placeholders := [3]string{"{repository}", "{revision}", "{path}"}
	for _, p := range placeholders {
		if count := strings.Count(raw, p); count != 1 {
			return fmt.Errorf("%s must contain placeholder %q exactly once, found %d", EnvSourceLinkTemplate, p, count)
		}
	}
	return nil
}

// validateTimeoutCrossConstraint refuses a model timeout that the ask timeout would cut short.
// A request to the model that runs into its own timeout yields excerpts; one that runs into the
// ask timeout yields the failure reply, so the model timeout in effect has to end first. The
// check applies when a model is configured or the model timeout is set, and it counts a value
// that is not set as its built-in default.
func validateTimeoutCrossConstraint(modelTimeout, askTimeout time.Duration, hasModel bool) error {
	if modelTimeout == 0 && !hasModel {
		return nil
	}
	effectiveModel := modelTimeout
	if effectiveModel == 0 {
		effectiveModel = model.DefaultTimeout
	}
	effectiveAsk := askTimeout
	if effectiveAsk == 0 {
		effectiveAsk = DefaultAskTimeout
	}
	if effectiveModel >= effectiveAsk {
		return fmt.Errorf("%s (%v in effect) must be strictly below %s (%v in effect)",
			EnvModelTimeout, effectiveModel, EnvAskTimeout, effectiveAsk)
	}
	return nil
}
