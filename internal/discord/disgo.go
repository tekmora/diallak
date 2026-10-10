package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	"github.com/tekmora/discord-bot/internal/answer"
)

const (
	defaultShutdownTimeout = 15 * time.Second
	gatewayOpenTimeout     = 15 * time.Second

	// maxBusyGoroutines is the fixed upper bound on concurrent goroutines executing immediate
	// busy replies while all asking slots are saturated.
	maxBusyGoroutines = 8
)

// BotConfig configures the live Discord gateway client and runtime adapter.
type BotConfig struct {
	Token           string
	GuildID         uint64
	Handler         *Handler
	Logger          *slog.Logger
	ShutdownTimeout time.Duration
}

// RegisterCommands registers the single guild-scoped /ask command with Discord via REST.
func RegisterCommands(ctx context.Context, token string, guildID uint64) error {
	if token == "" {
		return errors.New("bot token is required")
	}
	if guildID == 0 {
		return errors.New("guild ID must not be zero")
	}

	client, err := disgo.New(token)
	if err != nil {
		return fmt.Errorf("create discord client: %w", sanitizeToken(err, token))
	}
	defer client.Close(ctx)

	cmd := buildCommandCreate()
	commands := []discord.ApplicationCommandCreate{cmd}

	_, err = client.Rest.SetGuildCommands(client.ApplicationID, snowflake.ID(guildID), commands, rest.WithCtx(ctx))
	if err != nil {
		return fmt.Errorf("set guild commands: %w", sanitizeToken(err, token))
	}
	return nil
}

// Run starts the Discord gateway connection and dispatches interactions to the handler.
func Run(ctx context.Context, cfg BotConfig) error {
	shutdownTimeout, err := validateBotConfig(cfg)
	if err != nil {
		return err
	}

	// The worker pool bound is MaxConcurrent + 8 immediate busy replies, ensuring goroutines
	// executing HandleAsk are strictly bounded under HISS-06 even during interaction surges.
	workerSem := make(chan struct{}, cfg.Handler.maxConcurrent+maxBusyGoroutines)
	var dispatching atomic.Bool
	dispatching.Store(true)

	client, err := buildClient(ctx, cfg, workerSem, &dispatching)
	if err != nil {
		return fmt.Errorf("build discord client: %w", sanitizeToken(err, cfg.Token))
	}

	if err := openGatewayWithTimeout(ctx, client, cfg.Token); err != nil {
		return err
	}

	<-ctx.Done()
	dispatching.Store(false)

	return shutdownClient(cfg, client, shutdownTimeout)
}

func validateBotConfig(cfg BotConfig) (time.Duration, error) {
	if cfg.Token == "" {
		return 0, errors.New("bot token is required")
	}
	if cfg.GuildID == 0 {
		return 0, errors.New("guild ID must not be zero")
	}
	if cfg.Handler == nil {
		return 0, errors.New("handler is required")
	}
	timeout := cfg.ShutdownTimeout
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	return timeout, nil
}

func buildClient(ctx context.Context, cfg BotConfig, workerSem chan struct{}, dispatching *atomic.Bool) (*bot.Client, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	return disgo.New(cfg.Token,
		bot.WithLogger(logger),
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(gateway.IntentsNone),
			gateway.WithAutoReconnect(true),
		),
		bot.WithRestConfigOpts(
			rest.WithDefaultAllowedMentions(defaultAllowedMentions()),
		),
		bot.WithEventListenerFunc(func(e *events.InteractionCreate) {
			handleIncomingInteraction(ctx, cfg, e, workerSem, dispatching)
		}),
	)
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

	data := appCmd.SlashCommandInteractionData()
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
	return extractQuestion(a.interaction.SlashCommandInteractionData())
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

func buildCommandCreate() discord.ApplicationCommandCreate {
	maxLen := answer.MaxQuestionRunes
	return discord.SlashCommandCreate{
		Name:        CommandName,
		Description: "Ask a question about the project documentation",
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
