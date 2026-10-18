// Package config loads the bot's settings from the process environment.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	"github.com/tekmora/diallak/internal/embed"
	"github.com/tekmora/diallak/internal/ghapp"
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
	// EnvModelPolicy names the variable holding the expected gateway policy.
	EnvModelPolicy = "DIALLAK_MODEL_POLICY"
	// EnvModelGuardrail names the optional variable holding the guardrail of the bot's own masking
	// request (ADR-0013, step two). Unset, the bot sends no masking request.
	EnvModelGuardrail = "DIALLAK_MODEL_GUARDRAIL"
	// EnvModelKeyFile names the variable holding the path of the optional model API key file.
	EnvModelKeyFile = "DIALLAK_MODEL_KEY_FILE" // #nosec G101 -- the name of an environment variable, not a credential.
	// EnvEmbedBaseURL names the variable holding the base URL of the embedding endpoint.
	EnvEmbedBaseURL = "DIALLAK_EMBED_BASE_URL"
	// EnvEmbedAlias names the variable holding the embedding routing alias.
	EnvEmbedAlias = "DIALLAK_EMBED_ALIAS"
	// EnvEmbedKeyFile names the variable holding the path of the optional embedding API key file.
	EnvEmbedKeyFile = "DIALLAK_EMBED_KEY_FILE" // #nosec G101 -- the name of an environment variable, not a credential.
	// EnvEmbedTimeout names the optional variable setting the embedding request timeout.
	EnvEmbedTimeout = "DIALLAK_EMBED_TIMEOUT"
	// EnvEmbedMinSimilarity names the optional variable setting the minimum similarity threshold.
	EnvEmbedMinSimilarity = "DIALLAK_EMBED_MIN_SIMILARITY"
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
	// EnvTrailRetention names the optional variable setting how long a moderation trail line stays.
	EnvTrailRetention = "DIALLAK_TRAIL_RETENTION"
	// EnvForumChannelID names the optional variable holding the snowflake of the support forum the
	// bot answers new posts in.
	EnvForumChannelID = "DIALLAK_FORUM_CHANNEL_ID"
	// EnvForumOptInTagID names the variable holding the snowflake of the forum tag a member sets to
	// ask for the automated answer. It is required when EnvForumChannelID is set.
	EnvForumOptInTagID = "DIALLAK_FORUM_OPT_IN_TAG_ID"
	// EnvNewsStateFile names the optional variable holding the absolute path of the news state file
	// (ADR-0019). Unset, the news function is off and reads nothing.
	EnvNewsStateFile = "DIALLAK_NEWS_STATE_FILE"
	// EnvNewsGitHubTokenFile names the optional variable holding the path of a read-only GitHub
	// token file for the advisory reads. It needs EnvNewsStateFile.
	EnvNewsGitHubTokenFile = "DIALLAK_NEWS_GITHUB_TOKEN_FILE" // #nosec G101 -- the name of an environment variable, not a credential.
	// EnvDiscussionsAppClientID names the optional variable holding the GitHub App client ID.
	EnvDiscussionsAppClientID = "DIALLAK_DISCUSSIONS_APP_CLIENT_ID"
	// EnvDiscussionsAppKeyFile names the optional variable holding the path of the GitHub App RSA key.
	EnvDiscussionsAppKeyFile = "DIALLAK_DISCUSSIONS_APP_KEY_FILE" // #nosec G101 -- the name of an environment variable, not a credential.

	// MaxTokenBytes bounds secret files (both bot token and model key).
	MaxTokenBytes = 4096
	// MaxKeyFileBytes bounds secret files for GitHub App RSA private keys.
	MaxKeyFileBytes = 16384
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
	// MinTrailRetention is the lower bound for DIALLAK_TRAIL_RETENTION: one day, the sweep interval.
	MinTrailRetention = 24 * time.Hour
	// MaxTrailRetention is the upper bound for DIALLAK_TRAIL_RETENTION: 365 days.
	MaxTrailRetention = 365 * 24 * time.Hour
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
	// ModelPolicy is the name of the gateway policy every chat reply must name.
	ModelPolicy string
	// ModelGuardrail names the guardrail of the bot's own masking request. It is empty when the
	// variable is not set; the bot then sends no masking request.
	ModelGuardrail string
	// ModelKey is the optional API key for the model endpoint. It is never logged and is left out
	// of JSON encodings.
	ModelKey string `json:"-"`
	// EmbedBaseURL is the base URL of the OpenAI-compatible embedding endpoint.
	EmbedBaseURL string
	// EmbedAlias is the routing name passed in embedding requests.
	EmbedAlias string
	// EmbedKey is the optional API key for the embedding endpoint. It is never logged and is left out
	// of JSON encodings.
	EmbedKey string `json:"-"`
	// EmbedTimeout is the maximum duration for embedding requests to the embedding endpoint.
	// It is 0 when the variable is not set; embed then uses its built-in default.
	EmbedTimeout time.Duration
	// EmbedMinSimilarity is the minimum cosine similarity threshold for semantic retrieval hits.
	// It is 0 when the variable is not set; retrieval then uses its built-in default.
	EmbedMinSimilarity float64
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
	// TrailRetention is how long a line of the moderation trail stays before the bot deletes it.
	// It is 0 when the variable is not set; discord then uses its built-in default of 90 days.
	TrailRetention time.Duration
	// ForumChannelID is the support forum whose new opted-in posts get a first answer. It is 0 when
	// the variable is not set; the forum answer is then off.
	ForumChannelID uint64
	// ForumOptInTagID is the forum tag that asks for the automated answer. It is set exactly when
	// ForumChannelID is set.
	ForumOptInTagID uint64
	// NewsStateFile is the absolute path of the news state file. It is empty when the variable is
	// not set; the news function is then off.
	NewsStateFile string
	// NewsGitHubToken is the optional read-only GitHub token of the advisory reads. It is never
	// logged and is left out of JSON encodings.
	NewsGitHubToken string `json:"-"`
	// DiscussionsAppClientID is the GitHub App client ID for reading repository discussions.
	DiscussionsAppClientID string
	// DiscussionsAppKey is the private key PEM text of the GitHub App. It is never logged
	// and is left out of JSON encodings.
	DiscussionsAppKey string `json:"-"`
}

// HasModel reports whether a model endpoint is configured.
func (c Config) HasModel() bool {
	return c.ModelBaseURL != "" && c.ModelAlias != ""
}

// HasEmbed reports whether an embedding endpoint is configured.
func (c Config) HasEmbed() bool {
	return c.EmbedBaseURL != "" && c.EmbedAlias != ""
}

// HasForum reports whether the first answer in the support forum is configured.
func (c Config) HasForum() bool {
	return c.ForumChannelID != 0
}

// HasNews reports whether the news function is on.
func (c Config) HasNews() bool {
	return c.NewsStateFile != ""
}

// HasDiscussions reports whether the discussions reading function is configured.
func (c Config) HasDiscussions() bool {
	return c.DiscussionsAppClientID != "" && c.DiscussionsAppKey != "" && c.HasNews()
}

// String returns a redacted representation of Config. Token, ModelKey, EmbedKey and NewsGitHubToken are
// never printed.
func (c Config) String() string {
	return fmt.Sprintf("Config{Token:%q, GuildID:%d, ModelBaseURL:%q, ModelAlias:%q, ModelPolicy:%q, ModelGuardrail:%q, ModelKey:%q, EmbedBaseURL:%q, EmbedAlias:%q, EmbedKey:%q, EmbedTimeout:%v, EmbedMinSimilarity:%g, CorpusDir:%q, SupportChannel:%q, TopPassages:%d, MinScore:%g, QuestionMaxChars:%d, ExcerptChars:%d, ReplyMaxChars:%d, MaxConcurrentAsks:%d, AskTimeout:%v, ModelTimeout:%v, SourceLinkTemplate:%q, HealthAddr:%q, TrailRetention:%v, ForumChannelID:%d, ForumOptInTagID:%d, NewsStateFile:%q, NewsGitHubToken:%q, DiscussionsAppClientID:%q, DiscussionsAppKey:%q}",
		redactSecret(c.Token), c.GuildID, c.ModelBaseURL, c.ModelAlias, c.ModelPolicy, c.ModelGuardrail, redactSecret(c.ModelKey),
		c.EmbedBaseURL, c.EmbedAlias, redactSecret(c.EmbedKey), c.EmbedTimeout, c.EmbedMinSimilarity,
		c.CorpusDir, c.SupportChannel,
		c.TopPassages, c.MinScore, c.QuestionMaxChars, c.ExcerptChars, c.ReplyMaxChars, c.MaxConcurrentAsks,
		c.AskTimeout, c.ModelTimeout, c.SourceLinkTemplate, c.HealthAddr, c.TrailRetention,
		c.ForumChannelID, c.ForumOptInTagID, c.NewsStateFile, redactSecret(c.NewsGitHubToken),
		c.DiscussionsAppClientID, redactSecret(c.DiscussionsAppKey))
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
		slog.String("ModelPolicy", c.ModelPolicy),
		slog.String("ModelGuardrail", c.ModelGuardrail),
		slog.String("ModelKey", redactSecret(c.ModelKey)),
		slog.String("EmbedBaseURL", c.EmbedBaseURL),
		slog.String("EmbedAlias", c.EmbedAlias),
		slog.String("EmbedKey", redactSecret(c.EmbedKey)),
		slog.Duration("EmbedTimeout", c.EmbedTimeout),
		slog.Float64("EmbedMinSimilarity", c.EmbedMinSimilarity),
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
		slog.Duration("TrailRetention", c.TrailRetention),
		slog.Uint64("ForumChannelID", c.ForumChannelID),
		slog.Uint64("ForumOptInTagID", c.ForumOptInTagID),
		slog.String("NewsStateFile", c.NewsStateFile),
		slog.String("NewsGitHubToken", redactSecret(c.NewsGitHubToken)),
		slog.String("DiscussionsAppClientID", c.DiscussionsAppClientID),
		slog.String("DiscussionsAppKey", redactSecret(c.DiscussionsAppKey)),
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
	guild, token, err := parseDiscordAccess(getenv)
	if err != nil {
		return Config{}, err
	}
	opt, err := readOptional(getenv)
	if err != nil {
		return Config{}, err
	}
	corpusDir, err := parseCorpusDir(getenv(EnvCorpusDir))
	if err != nil {
		return Config{}, err
	}
	m, err := parseModelConfig(getenv)
	if err != nil {
		return Config{}, err
	}
	emb, err := parseEmbedConfig(getenv, opt)
	if err != nil {
		return Config{}, err
	}
	supportChannel, err := parseSupportChannel(getenv(EnvSupportChannel))
	if err != nil {
		return Config{}, err
	}
	lim, err := parseLimits(opt, getenv(EnvSourceLinkTemplate), m.baseURL != "")
	if err != nil {
		return Config{}, err
	}
	healthAddr, retention, err := parseHealthAndRetention(getenv(EnvHealthAddr), opt)
	if err != nil {
		return Config{}, err
	}
	feat, err := parseFeatures(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Token:          token,
		GuildID:        guild,
		CorpusDir:      corpusDir,
		SupportChannel: supportChannel,
		HealthAddr:     healthAddr,
		TrailRetention: retention,
	}
	lim.set(&cfg)
	m.set(&cfg)
	emb.set(&cfg)
	feat.set(&cfg)
	return cfg, nil
}

// parseDiscordAccess reads the id of the one server the bot serves and the bot token.
func parseDiscordAccess(getenv func(string) string) (uint64, string, error) {
	guild, err := parseGuildID(getenv(EnvGuildID))
	if err != nil {
		return 0, "", err
	}
	token, err := readSecretFile(getenv(EnvTokenFile), EnvTokenFile, "token")
	if err != nil {
		return 0, "", err
	}
	return guild, token, nil
}

func parseHealthAndRetention(rawHealthAddr string, opt optionals) (string, time.Duration, error) {
	healthAddr, err := parseHealthAddr(rawHealthAddr)
	if err != nil {
		return "", 0, err
	}
	retention, err := opt.durationIn(EnvTrailRetention, opt.values.TrailRetention, MinTrailRetention, MaxTrailRetention)
	if err != nil {
		return "", 0, err
	}
	return healthAddr, retention, nil
}

// features are the settings of the functions that are off unless configured.
type features struct {
	forumChannel, forumTag uint64
	newsState, newsToken   string
	discClientID, discKey  string
}

// set copies the features' settings into c.
func (f features) set(c *Config) {
	c.ForumChannelID, c.ForumOptInTagID = f.forumChannel, f.forumTag
	c.NewsStateFile, c.NewsGitHubToken = f.newsState, f.newsToken
	c.DiscussionsAppClientID, c.DiscussionsAppKey = f.discClientID, f.discKey
}

// parseFeatures reads the support forum's, the news function's, and discussions settings.
func parseFeatures(getenv func(string) string) (features, error) {
	var f features
	var err error
	if f.forumChannel, f.forumTag, err = parseForum(getenv); err != nil {
		return features{}, err
	}
	if f.newsState, f.newsToken, err = parseNews(getenv); err != nil {
		return features{}, err
	}
	if f.discClientID, f.discKey, err = parseDiscussions(getenv, f.newsState != ""); err != nil {
		return features{}, err
	}
	return f, nil
}

// parseForum reads the support forum and its opt-in tag. Neither set means the forum answer is off.
// One without the other is an error: the forum answer never runs without the opt-in tag (ADR-0009
// case 18, ADR-0020), and a tag without a forum would be a setting that does nothing.
func parseForum(getenv func(string) string) (uint64, uint64, error) {
	rawChannel, rawTag := getenv(EnvForumChannelID), getenv(EnvForumOptInTagID)
	if rawChannel == "" && rawTag == "" {
		return 0, 0, nil
	}
	channel, err := parseSnowflake(rawChannel, EnvForumChannelID)
	if err != nil {
		return 0, 0, err
	}
	tag, err := parseSnowflake(rawTag, EnvForumOptInTagID)
	if err != nil {
		return 0, 0, err
	}
	return channel, tag, nil
}

// parseNews reads the news state file and the optional GitHub token. No state file means the news
// function is off; a token without it would be a setting that does nothing, so it stops the start.
func parseNews(getenv func(string) string) (string, string, error) {
	state, tokenFile := getenv(EnvNewsStateFile), getenv(EnvNewsGitHubTokenFile)
	if state == "" {
		if tokenFile != "" {
			return "", "", fmt.Errorf("%s is set but %s is not", EnvNewsGitHubTokenFile, EnvNewsStateFile)
		}
		return "", "", nil
	}
	if !filepath.IsAbs(state) {
		return "", "", fmt.Errorf("%s must be an absolute path: %q", EnvNewsStateFile, state)
	}
	if tokenFile == "" {
		return state, "", nil
	}
	token, err := readSecretFile(tokenFile, EnvNewsGitHubTokenFile, "GitHub token")
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", EnvNewsGitHubTokenFile, err)
	}
	return state, token, nil
}

func parseDiscussions(getenv func(string) string, hasNews bool) (string, string, error) {
	clientID := getenv(EnvDiscussionsAppClientID)
	keyFile := getenv(EnvDiscussionsAppKeyFile)
	if clientID == "" && keyFile == "" {
		return "", "", nil
	}
	if clientID == "" {
		return "", "", fmt.Errorf("%s is set but %s is not", EnvDiscussionsAppKeyFile, EnvDiscussionsAppClientID)
	}
	if keyFile == "" {
		return "", "", fmt.Errorf("%s is set but %s is not", EnvDiscussionsAppClientID, EnvDiscussionsAppKeyFile)
	}
	if !hasNews {
		return "", "", fmt.Errorf("%s is set but %s is not", EnvDiscussionsAppClientID, EnvNewsStateFile)
	}
	if !validDiscussionsClientID(clientID) {
		return "", "", fmt.Errorf("%s must be 1 to 64 alphanumeric characters or dots: %q", EnvDiscussionsAppClientID, clientID)
	}
	keyPEM, err := readKeyFile(keyFile)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", EnvDiscussionsAppKeyFile, err)
	}
	return clientID, keyPEM, nil
}

func readKeyFile(path string) (string, error) {
	secret, err := readSecretFileLimit(path, EnvDiscussionsAppKeyFile, "discussions key", MaxKeyFileBytes)
	if err != nil {
		return "", err
	}
	if _, err := ghapp.ParseKey([]byte(secret)); err != nil {
		return "", fmt.Errorf("parse discussions key: %w", err)
	}
	return secret, nil
}

func isAllowedClientIDChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '.':
		return true
	default:
		return false
	}
}

func validDiscussionsClientID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isAllowedClientIDChar(s[i]) {
			return false
		}
	}
	return true
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

// modelSettings are the model variables as parseModelConfig read them.
type modelSettings struct {
	baseURL, alias, policy, guardrail, key string
}

func (m modelSettings) set(c *Config) {
	c.ModelBaseURL = m.baseURL
	c.ModelAlias = m.alias
	c.ModelPolicy = m.policy
	c.ModelGuardrail = m.guardrail
	c.ModelKey = m.key
}

func parseModelConfig(getenv func(string) string) (modelSettings, error) {
	m := modelSettings{
		baseURL:   getenv(EnvModelBaseURL),
		alias:     getenv(EnvModelAlias),
		policy:    getenv(EnvModelPolicy),
		guardrail: getenv(EnvModelGuardrail),
	}
	keyFile := getenv(EnvModelKeyFile)

	hasBase := m.baseURL != ""
	if hasBase != (m.alias != "") {
		return modelSettings{}, fmt.Errorf("%s and %s must both be set or both be unset", EnvModelBaseURL, EnvModelAlias)
	}
	if !hasBase {
		for _, name := range []string{EnvModelKeyFile, EnvModelPolicy, EnvModelGuardrail} {
			if getenv(name) != "" {
				return modelSettings{}, fmt.Errorf("%s is set but no model is configured", name)
			}
		}
		return modelSettings{}, nil
	}

	if m.policy == "" {
		return modelSettings{}, fmt.Errorf("%s is not set", EnvModelPolicy)
	}
	if !model.ValidPolicyName(m.policy) {
		return modelSettings{}, fmt.Errorf("%s must match ^[A-Za-z0-9._:/-]{1,128}$", EnvModelPolicy)
	}
	if m.guardrail != "" && !model.ValidPolicyName(m.guardrail) {
		return modelSettings{}, fmt.Errorf("%s must match ^[A-Za-z0-9._:/-]{1,128}$", EnvModelGuardrail)
	}

	if keyFile != "" {
		readKey, err := readSecretFile(keyFile, EnvModelKeyFile, "model key")
		if err != nil {
			return modelSettings{}, err
		}
		m.key = readKey
	}
	return m, nil
}

// embedSettings are the embedding variables as parseEmbedConfig read them.
type embedSettings struct {
	baseURL, alias, key string
	timeout             time.Duration
	minSimilarity       float64
}

func (e embedSettings) set(c *Config) {
	c.EmbedBaseURL = e.baseURL
	c.EmbedAlias = e.alias
	c.EmbedKey = e.key
	c.EmbedTimeout = e.timeout
	c.EmbedMinSimilarity = e.minSimilarity
}

func parseEmbedConfig(getenv func(string) string, opt optionals) (embedSettings, error) {
	e := embedSettings{
		baseURL: getenv(EnvEmbedBaseURL),
		alias:   getenv(EnvEmbedAlias),
	}
	keyFile := getenv(EnvEmbedKeyFile)

	hasBase := e.baseURL != ""
	if hasBase != (e.alias != "") {
		return embedSettings{}, fmt.Errorf("%s and %s must both be set or both be unset", EnvEmbedBaseURL, EnvEmbedAlias)
	}
	if !hasBase {
		for _, name := range []string{EnvEmbedKeyFile, EnvEmbedTimeout, EnvEmbedMinSimilarity} {
			if getenv(name) != "" {
				return embedSettings{}, fmt.Errorf("%s is set but no embedding endpoint is configured", name)
			}
		}
		return embedSettings{}, nil
	}

	if err := model.ValidateBaseURL(e.baseURL); err != nil {
		return embedSettings{}, fmt.Errorf("%s: %w", EnvEmbedBaseURL, err)
	}
	if err := embed.OnPremises(e.baseURL); err != nil {
		return embedSettings{}, fmt.Errorf("%s: %w", EnvEmbedBaseURL, err)
	}
	if !model.ValidAlias(e.alias) {
		return embedSettings{}, fmt.Errorf("%s must match ^[A-Za-z0-9._:/-]{1,128}$", EnvEmbedAlias)
	}

	if keyFile != "" {
		readKey, err := readSecretFile(keyFile, EnvEmbedKeyFile, "embedding key")
		if err != nil {
			return embedSettings{}, err
		}
		e.key = readKey
	}

	timeout, err := opt.durationIn(EnvEmbedTimeout, opt.values.EmbedTimeout, embed.MinTimeout, embed.MaxTimeout)
	if err != nil {
		return embedSettings{}, err
	}
	e.timeout = timeout

	minSim, err := opt.floatIn(EnvEmbedMinSimilarity, opt.values.EmbedMinSimilarity, 0.0, 1.0)
	if err != nil {
		return embedSettings{}, err
	}
	e.minSimilarity = minSim

	return e, nil
}

func parseGuildID(raw string) (uint64, error) {
	return parseSnowflake(raw, EnvGuildID)
}

// parseSnowflake reads a required, non-zero decimal snowflake from the variable envVar.
func parseSnowflake(raw, envVar string) (uint64, error) {
	if raw == "" {
		return 0, fmt.Errorf("%s is not set", envVar)
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is not a decimal snowflake: %w", envVar, err)
	}
	if id == 0 {
		return 0, fmt.Errorf("%s must not be zero", envVar)
	}
	return id, nil
}

func readSecretFile(path, envVar, description string) (string, error) {
	return readSecretFileLimit(path, envVar, description, MaxTokenBytes)
}

// readSecretFileLimit reads a secret file of at most limit bytes.
func readSecretFileLimit(path, envVar, description string, limit int64) (secret string, err error) {
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
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return "", fmt.Errorf("read %s file: %w", description, err)
	}
	if int64(len(data)) > limit {
		return "", fmt.Errorf("%s file is larger than %d bytes", description, limit)
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

// limits are the answer path's optional bounds, timeouts and link template.
type limits struct {
	passages, qMax, excerpt, reply, concurrent int
	minScore                                   float64
	askTimeout, modelTimeout                   time.Duration
	linkTmpl                                   string
}

// set copies the limits into c.
func (l limits) set(c *Config) {
	c.TopPassages, c.MinScore, c.QuestionMaxChars = l.passages, l.minScore, l.qMax
	c.ExcerptChars, c.ReplyMaxChars, c.MaxConcurrentAsks = l.excerpt, l.reply, l.concurrent
	c.AskTimeout, c.ModelTimeout, c.SourceLinkTemplate = l.askTimeout, l.modelTimeout, l.linkTmpl
}

// parseLimits checks the answer path's optional bounds, timeouts and link template.
func parseLimits(opt optionals, rawLinkTemplate string, hasModel bool) (limits, error) {
	var l limits
	var err error
	if l.passages, l.minScore, l.qMax, l.excerpt, l.reply, err = parseOptionalLimits(opt); err != nil {
		return limits{}, err
	}
	l.concurrent, l.askTimeout, l.modelTimeout, l.linkTmpl, err = parseOptionalTimeoutsAndTemplate(opt, rawLinkTemplate, hasModel)
	if err != nil {
		return limits{}, err
	}
	return l, nil
}

func parseOptionalLimits(opt optionals) (int, float64, int, int, int, error) {
	passages, err := opt.intIn(EnvTopPassages, opt.values.TopPassages, 1, retrieval.MaxLimit)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	minScore, err := opt.floatIn(EnvMinScore, opt.values.MinScore, 0.0, 1.0)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	qMax, err := opt.intIn(EnvQuestionMaxChars, opt.values.QuestionMaxChars, 1, answer.MaxQuestionRunes)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	excerpt, err := opt.intIn(EnvExcerptChars, opt.values.ExcerptChars, answer.MinExcerptRunes, answer.MaxExcerptRunesLimit)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	reply, err := opt.intIn(EnvReplyMaxChars, opt.values.ReplyMaxChars, answer.MinRenderRunes, answer.MaxAllowedRenderRunes)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return passages, minScore, qMax, excerpt, reply, nil
}

func parseOptionalTimeoutsAndTemplate(opt optionals, rawLinkTemplate string, hasModel bool) (int, time.Duration, time.Duration, string, error) {
	concurrent, err := opt.intIn(EnvMaxConcurrentAsks, opt.values.MaxConcurrentAsks, MinConcurrentAsks, MaxConcurrentAsksLimit)
	if err != nil {
		return 0, 0, 0, "", err
	}
	askTimeout, err := opt.durationIn(EnvAskTimeout, opt.values.AskTimeout, MinAskTimeout, MaxAskTimeout)
	if err != nil {
		return 0, 0, 0, "", err
	}
	modelTimeout, err := opt.durationIn(EnvModelTimeout, opt.values.ModelTimeout, model.MinTimeout, model.MaxTimeout)
	if err != nil {
		return 0, 0, 0, "", err
	}
	if err := validateTimeoutCrossConstraint(modelTimeout, askTimeout, hasModel); err != nil {
		return 0, 0, 0, "", err
	}
	linkTmpl, err := parseSourceLinkTemplate(rawLinkTemplate)
	if err != nil {
		return 0, 0, 0, "", err
	}
	return concurrent, askTimeout, modelTimeout, linkTmpl, nil
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
