package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/answer"
)

const (
	defaultShutdownTimeout   = 15 * time.Second
	gatewayOpenTimeout       = 15 * time.Second
	applicationLookupTimeout = 15 * time.Second

	// maxBusyGoroutines is the fixed upper bound on concurrent goroutines executing immediate
	// busy replies while all asking slots are saturated.
	maxBusyGoroutines = 8
)

var errEmptyApplicationID = errors.New("application id is empty")

// serverCallTimeout is chosen, not measured.
const serverCallTimeout = 10 * time.Second

// applyCallTimeout is chosen, not measured; it has to cover the library's wait on a 429.
const applyCallTimeout = 60 * time.Second

// clientOptions are the seams of this package's tests: the address of the platform's REST API, the
// deadline of the application lookup and the function that opens the gateway. The exported
// functions pass the zero value, which means the real platform and the defaults.
type clientOptions struct {
	restURL           string
	lookupTimeout     time.Duration
	openGateway       func(ctx context.Context, client *bot.Client, token string) error
	maxQuestionChars  int
	now               func() time.Time
	serverCallTimeout time.Duration
	applyCallTimeout  time.Duration
}

// lookupDeadline is the configured deadline of the application lookup, or the default.
func (o clientOptions) lookupDeadline() time.Duration {
	if o.lookupTimeout > 0 {
		return o.lookupTimeout
	}
	return applicationLookupTimeout
}

func (o clientOptions) getNow() time.Time {
	if o.now != nil {
		return o.now()
	}
	return time.Now()
}

func (o clientOptions) callTimeout() time.Duration {
	if o.serverCallTimeout > 0 {
		return o.serverCallTimeout
	}
	return serverCallTimeout
}

func (o clientOptions) getApplyCallTimeout() time.Duration {
	if o.applyCallTimeout > 0 {
		return o.applyCallTimeout
	}
	return applyCallTimeout
}

// BotConfig configures the live Discord gateway client and runtime adapter.
type BotConfig struct {
	Token           string
	GuildID         uint64
	Handler         *Handler
	Logger          *slog.Logger
	ShutdownTimeout time.Duration
	// ReportReady reports whether the bot is ready to serve. It is called with true
	// once the gateway is open and with false when shutdown begins. A nil callback
	// is ignored.
	ReportReady func(bool)
}

// RegisterCommands registers the single guild-scoped /ask command with Discord via REST.
// maxQuestionChars is the longest question the command accepts, in runes; zero means
// answer.MaxQuestionRunes.
func RegisterCommands(ctx context.Context, token string, guildID uint64, maxQuestionChars int) error {
	return registerCommandsWithOptions(ctx, token, guildID, clientOptions{maxQuestionChars: maxQuestionChars})
}

func checkTokenAndGuild(token string, guildID uint64) error {
	if token == "" {
		return errors.New("bot token is required")
	}
	if guildID == 0 {
		return errors.New("guild ID must not be zero")
	}
	return nil
}

func newRestClient(token string, restURL string) (*bot.Client, error) {
	// Without a logger the library writes to the process default with every attribute it has.
	botOpts := []bot.ConfigOpt{bot.WithLogger(libraryLogger(slog.Default()))}
	if restURL != "" {
		botOpts = append(botOpts, bot.WithRestClientConfigOpts(rest.WithURL(restURL)))
	}
	client, err := disgo.New(token, botOpts...)
	if err != nil {
		return nil, fmt.Errorf("create discord client: %w", sanitizeToken(err, token))
	}
	return client, nil
}

func registerCommandsWithOptions(ctx context.Context, token string, guildID uint64, opts clientOptions) error {
	if err := checkTokenAndGuild(token, guildID); err != nil {
		return err
	}
	maxQuestionChars, err := validateQuestionMaxChars(opts.maxQuestionChars)
	if err != nil {
		return err
	}

	client, err := newRestClient(token, opts.restURL)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	appID, err := lookupApplicationID(ctx, client.Rest, token, opts.lookupDeadline())
	if err != nil {
		return err
	}

	cmd := buildCommandCreate(maxQuestionChars)
	commands := []discord.ApplicationCommandCreate{cmd}

	_, err = client.Rest.SetGuildCommands(appID, snowflake.ID(guildID), commands, rest.WithCtx(ctx))
	if err != nil {
		return callError("set guild commands", err, token)
	}
	return nil
}

// lookupApplicationID asks the platform which application the token belongs to. The id is never
// taken from the token: for an old application the two differ. Every failure is an error, and
// the error text never holds the token.
func lookupApplicationID(
	ctx context.Context,
	restClient rest.Rest,
	token string,
	deadline time.Duration,
) (snowflake.ID, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	app, err := restClient.GetCurrentApplication(rest.WithCtx(lookupCtx))
	if err != nil {
		return 0, callError("lookup current application", err, token)
	}
	if app == nil || app.ID == 0 {
		return 0, fmt.Errorf("lookup current application: %w", errEmptyApplicationID)
	}
	return app.ID, nil
}

// Run starts the Discord gateway connection and dispatches interactions to the handler.
func Run(ctx context.Context, cfg BotConfig) error {
	return runWithOptions(ctx, cfg, clientOptions{})
}

func runWithOptions(ctx context.Context, cfg BotConfig, opts clientOptions) error {
	cfg, err := normalizeBotConfig(cfg)
	if err != nil {
		return err
	}

	// The worker pool bound is MaxConcurrent + 8 immediate busy replies, ensuring goroutines
	// executing HandleAsk are strictly bounded under HISS-06 even during interaction surges.
	workerSem := make(chan struct{}, cfg.Handler.maxConcurrent+maxBusyGoroutines)
	var dispatching atomic.Bool
	dispatching.Store(true)

	client, err := buildClient(ctx, cfg, workerSem, &dispatching, opts)
	if err != nil {
		return fmt.Errorf("build discord client: %w", sanitizeToken(err, cfg.Token))
	}

	if _, err := preGatewayStep(ctx, cfg, client, opts); err != nil {
		client.Close(ctx)
		return err
	}

	if cfg.ReportReady != nil {
		cfg.ReportReady(true)
	}

	<-ctx.Done()
	if cfg.ReportReady != nil {
		cfg.ReportReady(false)
	}
	dispatching.Store(false)

	return shutdownClient(cfg, client, cfg.ShutdownTimeout)
}

func preGatewayStep(
	ctx context.Context,
	cfg BotConfig,
	client *bot.Client,
	opts clientOptions,
) (snowflake.ID, error) {
	appID, err := lookupApplicationID(ctx, client.Rest, cfg.Token, opts.lookupDeadline())
	if err != nil {
		return 0, err
	}

	matchesTokenID := appID == client.ApplicationID
	cfg.Logger.Info("application verified",
		slog.Uint64("application_id", uint64(appID)),
		slog.Bool("matches_token_id", matchesTokenID),
	)

	opener := opts.openGateway
	if opener == nil {
		opener = openGatewayWithTimeout
	}
	if err := opener(ctx, client, cfg.Token); err != nil {
		return 0, err
	}
	return appID, nil
}

// normalizeBotConfig validates cfg and fills its defaults, so every later use finds a logger and
// a shutdown timeout.
func normalizeBotConfig(cfg BotConfig) (BotConfig, error) {
	if cfg.Token == "" {
		return BotConfig{}, errors.New("bot token is required")
	}
	if cfg.GuildID == 0 {
		return BotConfig{}, errors.New("guild ID must not be zero")
	}
	if cfg.Handler == nil {
		return BotConfig{}, errors.New("handler is required")
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	return cfg, nil
}

// gatewayIntents is the intent set of the gateway session: none. Application command interactions
// arrive without any intent, and the bot requests no privileged one (ADR-0001).
func gatewayIntents() gateway.Intents {
	return gateway.IntentsNone
}

// libraryLogFloor is the lowest level at which the Discord library may write to the log. Below it
// the library logs request and response bodies and raw gateway payloads, which carry questions and
// answers; those never reach a log, whatever level the process runs at. Records at or above the
// floor pass only allow-listed attributes with primitive values and bounded string lengths.
const libraryLogFloor = slog.LevelWarn

const (
	maxRecordAttrs = 32
	maxErrorLinks  = 8
	maxStringBytes = 64
)

// libraryAllowedAttrs is the allow-list of attribute keys a record of the Discord library may carry:
// counters, durations, flags, status codes, type names and the name of the library part. A key
// outside it is dropped with its value, so a key a later library version adds stays out of the log
// until it is listed here. The key "data" is not listed: one library site writes a whole event under
// it.
var libraryAllowedAttrs = map[string]struct{}{
	"name":               {},
	"shard_id":           {},
	"shard_count":        {},
	"retry_after":        {},
	"try":                {},
	"delay":              {},
	"last_heartbeat_ago": {},
	"reconnect":          {},
	"can_resume":         {},
	"code":               {},
	"type":               {},
	"event_type":         {},
}

func isAllowedKey(key string) bool {
	_, ok := libraryAllowedAttrs[key]
	return ok
}

func cutString(s string) string {
	if len(s) > maxStringBytes {
		return s[:maxStringBytes]
	}
	return s
}

func formatErrorTypes(err error) string {
	parts := make([]string, 0, maxErrorLinks)
	curr := err
	for i := 0; i < maxErrorLinks && curr != nil; i++ {
		parts = append(parts, fmt.Sprintf("%T", curr))
		rv := reflect.ValueOf(curr)
		if rv.Kind() == reflect.Pointer && rv.IsNil() {
			break
		}
		curr = errors.Unwrap(curr)
	}
	return strings.Join(parts, ">")
}

func extractError(attr slog.Attr) (error, bool) {
	if attr.Key != "err" {
		return nil, false
	}
	if e, ok := attr.Value.Resolve().Any().(error); ok && e != nil {
		return e, true
	}
	return nil, false
}

// sanitizeValue passes a number, a flag, a duration or a string cut to maxStringBytes, and refuses
// every other kind: a group, a time or a structured value could carry content under a listed key.
func sanitizeValue(key string, val slog.Value) (slog.Attr, bool) {
	switch val.Kind() {
	case slog.KindInt64, slog.KindUint64, slog.KindFloat64, slog.KindBool, slog.KindDuration:
		return slog.Attr{Key: key, Value: val}, true
	case slog.KindString:
		return slog.String(key, cutString(val.String())), true
	case slog.KindAny:
		anyVal := val.Any()
		if anyVal != nil && reflect.ValueOf(anyVal).Kind() == reflect.String {
			return slog.String(key, cutString(reflect.ValueOf(anyVal).String())), true
		}
		return slog.Attr{}, false
	default:
		return slog.Attr{}, false
	}
}

// filterAttr returns the attribute to log, whether there is one, and whether it replaces the given
// one. An error under "err" becomes its chain of type names: an error text can quote the value that
// failed to parse.
func filterAttr(attr slog.Attr) (slog.Attr, bool, bool) {
	if attr.Key == "err" {
		if errVal, ok := extractError(attr); ok {
			return slog.String("error_types", formatErrorTypes(errVal)), true, true
		}
		return slog.Attr{}, false, false
	}
	if !isAllowedKey(attr.Key) {
		return slog.Attr{}, false, false
	}
	kept, ok := sanitizeValue(attr.Key, attr.Value.Resolve())
	return kept, ok, false
}

// libraryLogger returns the logger handed to the Discord library: base, cut off below
// libraryLogFloor, passing only allow-listed attributes (retry_after, try, delay, last_heartbeat_ago,
// reconnect, can_resume, code, type, event_type, data, name, shard_id, shard_count) and summarizing
// err as error_types.
func libraryLogger(base *slog.Logger) *slog.Logger {
	return slog.New(floorHandler{inner: base.Handler()})
}

// floorHandler drops every record below libraryLogFloor and passes only allow-listed attributes to inner.
type floorHandler struct {
	inner slog.Handler
}

// Enabled reports whether inner takes a record of level and level reaches the floor.
func (h floorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= libraryLogFloor && h.inner.Enabled(ctx, level)
}

// Handle passes a record at or above the floor to inner with allow-listed attributes.
func (h floorHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level < libraryLogFloor {
		return nil
	}

	totalAttrs := record.NumAttrs()
	readLimit := totalAttrs
	if readLimit > maxRecordAttrs {
		readLimit = maxRecordAttrs
	}

	keptAttrs := make([]slog.Attr, 0, readLimit)
	var droppedCount int
	var readCount int

	record.Attrs(func(attr slog.Attr) bool {
		if readCount >= maxRecordAttrs {
			return false
		}
		readCount++
		filtered, kept, replaced := filterAttr(attr)
		if kept {
			keptAttrs = append(keptAttrs, filtered)
		}
		if !kept || replaced {
			droppedCount++
		}
		return true
	})

	if totalAttrs > readCount {
		droppedCount += totalAttrs - readCount
	}
	if droppedCount > 0 {
		keptAttrs = append(keptAttrs, slog.Int("dropped_attrs", droppedCount))
	}

	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	newRecord.AddAttrs(keptAttrs...)
	if err := h.inner.Handle(ctx, newRecord); err != nil {
		return fmt.Errorf("handle library log record: %w", err)
	}
	return nil
}

// WithAttrs applies the allow-list filter before passing attributes to inner.
func (h floorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	limit := len(attrs)
	if limit > maxRecordAttrs {
		limit = maxRecordAttrs
	}
	filtered := make([]slog.Attr, 0, limit)
	for i := 0; i < limit; i++ {
		attr, kept, _ := filterAttr(attrs[i])
		if kept {
			filtered = append(filtered, attr)
		}
	}
	return floorHandler{inner: h.inner.WithAttrs(filtered)}
}

// WithGroup returns the receiver unchanged because a group would rename keys and defeat the allow-list.
func (h floorHandler) WithGroup(_ string) slog.Handler {
	return h
}

func buildClient(
	ctx context.Context,
	cfg BotConfig,
	workerSem chan struct{},
	dispatching *atomic.Bool,
	opts clientOptions,
) (*bot.Client, error) {
	botOpts := []bot.ConfigOpt{
		bot.WithLogger(libraryLogger(cfg.Logger)),
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(gatewayIntents()),
			gateway.WithAutoReconnect(true),
		),
		bot.WithRestConfigOpts(
			rest.WithDefaultAllowedMentions(defaultAllowedMentions()),
		),
		bot.WithEventListenerFunc(func(e *events.InteractionCreate) {
			handleIncomingInteraction(ctx, cfg, e, workerSem, dispatching)
		}),
	}
	if opts.restURL != "" {
		botOpts = append(botOpts, bot.WithRestClientConfigOpts(rest.WithURL(opts.restURL)))
	}
	return disgo.New(cfg.Token, botOpts...)
}

func handleIncomingInteraction(
	ctx context.Context,
	cfg BotConfig,
	e *events.InteractionCreate,
	workerSem chan struct{},
	dispatching *atomic.Bool,
) {
	if !dispatching.Load() {
		cfg.Logger.Debug("ignoring interaction during shutdown")
		return
	}

	appCmd, ok := e.Interaction.(discord.ApplicationCommandInteraction)
	if !ok {
		cfg.Logger.Debug("ignoring non-application-command interaction", slog.Int("type", int(e.Type())))
		return
	}

	data, ok := slashCommandData(appCmd)
	if !ok {
		cfg.Logger.Debug("ignoring application command that is no slash command")
		return
	}
	if data.CommandName() != CommandName {
		cfg.Logger.Debug("ignoring unknown command", slog.String("command", data.CommandName()))
		return
	}

	adapter := &interactionAdapter{
		client:      e.Client(),
		interaction: appCmd,
		respond:     e.Respond,
	}

	select {
	case workerSem <- struct{}{}:
		go func() {
			defer func() { <-workerSem }()
			cfg.Handler.HandleAsk(ctx, adapter)
		}()
	default:
		cfg.Logger.Warn("worker pool saturated, dropping interaction")
	}
}

func openGatewayWithTimeout(ctx context.Context, client *bot.Client, token string) error {
	openCtx, cancel := context.WithTimeout(ctx, gatewayOpenTimeout)
	defer cancel()

	if err := client.OpenGateway(openCtx); err != nil {
		return fmt.Errorf("open gateway: %w", sanitizeToken(err, token))
	}
	return nil
}

func shutdownClient(cfg BotConfig, client *bot.Client, timeout time.Duration) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	waitErr := cfg.Handler.Wait(shutdownCtx)
	client.Close(shutdownCtx)

	if waitErr != nil {
		return fmt.Errorf("wait for in-flight asks: %w", waitErr)
	}
	return nil
}

type interactionAdapter struct {
	client      *bot.Client
	interaction discord.ApplicationCommandInteraction
	respond     events.InteractionResponderFunc
}

func (a *interactionAdapter) GuildID() uint64 {
	return extractGuildID(a.interaction.GuildID())
}

func (a *interactionAdapter) Question() string {
	data, ok := slashCommandData(a.interaction)
	if !ok {
		return ""
	}
	return extractQuestion(data)
}

func (a *interactionAdapter) Defer(ctx context.Context) error {
	return a.respond(discord.InteractionResponseTypeDeferredCreateMessage, buildDeferPayload(), rest.WithCtx(ctx))
}

func (a *interactionAdapter) Reply(ctx context.Context, text string) error {
	_, err := a.client.Rest.UpdateInteractionResponse(
		a.interaction.ApplicationID(),
		a.interaction.Token(),
		buildReplyPayload(text),
		rest.WithCtx(ctx),
	)
	return err
}

func buildCommandCreate(maxChars int) discord.ApplicationCommandCreate {
	maxLen := maxChars
	if maxLen <= 0 {
		maxLen = answer.MaxQuestionRunes
	}
	return discord.SlashCommandCreate{
		Name:        CommandName,
		Description: CommandDescription,
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        OptionQuestion,
				Description: "The question to ask",
				Required:    true,
				MaxLength:   &maxLen,
			},
		},
	}
}

func defaultAllowedMentions() discord.AllowedMentions {
	return discord.AllowedMentions{
		Parse: []discord.AllowedMentionType{},
		Roles: []snowflake.ID{},
		Users: []snowflake.ID{},
	}
}

func buildDeferPayload() discord.MessageCreate {
	mentions := defaultAllowedMentions()
	return discord.MessageCreate{
		Flags:           discord.MessageFlagEphemeral,
		AllowedMentions: &mentions,
	}
}

func buildReplyPayload(text string) discord.MessageUpdate {
	mentions := defaultAllowedMentions()
	return discord.MessageUpdate{
		Content:         &text,
		AllowedMentions: &mentions,
	}
}

func extractGuildID(id *snowflake.ID) uint64 {
	if id == nil {
		return 0
	}
	return uint64(*id)
}

// slashCommandData returns the data of a slash command. The application is shared with another
// bot, so user and message commands can arrive too; the library's own accessor panics on those.
func slashCommandData(i discord.ApplicationCommandInteraction) (discord.SlashCommandInteractionData, bool) {
	data, ok := i.Data.(discord.SlashCommandInteractionData)
	return data, ok
}

func extractQuestion(data discord.SlashCommandInteractionData) string {
	return data.String(OptionQuestion)
}

func sanitizeToken(err error, token string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if token != "" && strings.Contains(msg, token) {
		return errors.New(strings.ReplaceAll(msg, token, "[redacted]"))
	}
	return err
}
