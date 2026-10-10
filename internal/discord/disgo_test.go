package discord

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/tekmora/diallak/internal/answer"
)

func TestBuildCommandCreate_Positive_Properties(t *testing.T) {
	cmd := buildCommandCreate()

	if cmd.CommandName() != CommandName {
		t.Errorf("command name = %q, want %q", cmd.CommandName(), CommandName)
	}
	if cmd.Type() != discord.ApplicationCommandTypeSlash {
		t.Errorf("command type = %v, want slash", cmd.Type())
	}

	slashCmd, ok := cmd.(discord.SlashCommandCreate)
	if !ok {
		t.Fatalf("command type is not SlashCommandCreate: %T", cmd)
	}
	if slashCmd.Description == "" {
		t.Errorf("command description is empty")
	}
	if len(slashCmd.Options) != 1 {
		t.Fatalf("options len = %d, want 1", len(slashCmd.Options))
	}

	opt, ok := slashCmd.Options[0].(discord.ApplicationCommandOptionString)
	if !ok {
		t.Fatalf("option 0 is not ApplicationCommandOptionString: %T", slashCmd.Options[0])
	}
	if opt.Name != OptionQuestion {
		t.Errorf("option name = %q, want %q", opt.Name, OptionQuestion)
	}
	if !opt.Required {
		t.Errorf("option required = false, want true")
	}
	if opt.MaxLength == nil || *opt.MaxLength != answer.MaxQuestionRunes {
		t.Errorf("option max length = %v, want %d", opt.MaxLength, answer.MaxQuestionRunes)
	}
	if opt.Description == "" {
		t.Errorf("option description is empty")
	}
}

func TestBuildDeferPayload_Positive_EphemeralAndAllowedMentions(t *testing.T) {
	payload := buildDeferPayload()

	if !payload.Flags.Has(discord.MessageFlagEphemeral) {
		t.Errorf("defer payload missing MessageFlagEphemeral: %v", payload.Flags)
	}
	if payload.AllowedMentions == nil {
		t.Fatalf("defer payload allowed mentions is nil")
	}
	if len(payload.AllowedMentions.Parse) != 0 {
		t.Errorf("defer payload parse mentions non-empty: %v", payload.AllowedMentions.Parse)
	}
	if len(payload.AllowedMentions.Roles) != 0 {
		t.Errorf("defer payload role mentions non-empty: %v", payload.AllowedMentions.Roles)
	}
	if len(payload.AllowedMentions.Users) != 0 {
		t.Errorf("defer payload user mentions non-empty: %v", payload.AllowedMentions.Users)
	}
	if payload.AllowedMentions.RepliedUser {
		t.Errorf("defer payload replied_user is true, want false")
	}
}

func TestBuildReplyPayload_Positive_ContentAndAllowedMentions(t *testing.T) {
	text := "Here is the rendered answer text."
	payload := buildReplyPayload(text)

	if payload.Content == nil || *payload.Content != text {
		t.Errorf("reply content = %v, want %q", payload.Content, text)
	}
	if payload.AllowedMentions == nil {
		t.Fatalf("reply payload allowed mentions is nil")
	}
	if len(payload.AllowedMentions.Parse) != 0 {
		t.Errorf("reply payload parse mentions non-empty: %v", payload.AllowedMentions.Parse)
	}
	if len(payload.AllowedMentions.Roles) != 0 {
		t.Errorf("reply payload role mentions non-empty: %v", payload.AllowedMentions.Roles)
	}
	if len(payload.AllowedMentions.Users) != 0 {
		t.Errorf("reply payload user mentions non-empty: %v", payload.AllowedMentions.Users)
	}
	if payload.AllowedMentions.RepliedUser {
		t.Errorf("reply payload replied_user is true, want false")
	}
}

func verifyDeferPayload(p discord.MessageCreate) error {
	if !p.Flags.Has(discord.MessageFlagEphemeral) {
		return errors.New("missing ephemeral flag")
	}
	if p.AllowedMentions == nil {
		return errors.New("nil allowed mentions")
	}
	if len(p.AllowedMentions.Parse) != 0 || len(p.AllowedMentions.Roles) != 0 || len(p.AllowedMentions.Users) != 0 || p.AllowedMentions.RepliedUser {
		return errors.New("non-empty allowed mentions")
	}
	return nil
}

func verifyReplyPayload(p discord.MessageUpdate) error {
	if p.AllowedMentions == nil {
		return errors.New("nil allowed mentions")
	}
	if len(p.AllowedMentions.Parse) != 0 || len(p.AllowedMentions.Roles) != 0 || len(p.AllowedMentions.Users) != 0 || p.AllowedMentions.RepliedUser {
		return errors.New("non-empty allowed mentions")
	}
	return nil
}

func TestBuildPayloads_Negative_ControlAllowedMentionsAndEphemeralFlag(t *testing.T) {
	// Assert normal builder succeeds
	if err := verifyDeferPayload(buildDeferPayload()); err != nil {
		t.Fatalf("legitimate defer payload failed verification: %v", err)
	}
	if err := verifyReplyPayload(buildReplyPayload("ok")); err != nil {
		t.Fatalf("legitimate reply payload failed verification: %v", err)
	}

	// Negative control 1: Defer payload without ephemeral flag fails
	missingEphemeral := buildDeferPayload()
	missingEphemeral.Flags = discord.MessageFlagsNone
	if err := verifyDeferPayload(missingEphemeral); err == nil {
		t.Errorf("verifyDeferPayload accepted missing ephemeral flag")
	}

	// Negative control 2: Defer payload with everyone parse mention fails
	leakyDeferMentions := buildDeferPayload()
	leakyDeferMentions.AllowedMentions = &discord.AllowedMentions{
		Parse: []discord.AllowedMentionType{discord.AllowedMentionTypeEveryone},
	}
	if err := verifyDeferPayload(leakyDeferMentions); err == nil {
		t.Errorf("verifyDeferPayload accepted leaky mentions")
	}

	// Negative control 3: Reply payload with role mention fails
	leakyReplyMentions := buildReplyPayload("ok")
	leakyReplyMentions.AllowedMentions = &discord.AllowedMentions{
		Roles: []snowflake.ID{123},
	}
	if err := verifyReplyPayload(leakyReplyMentions); err == nil {
		t.Errorf("verifyReplyPayload accepted leaky mentions")
	}
}

func TestExtractGuildID_Positive_And_Boundary(t *testing.T) {
	if got := extractGuildID(nil); got != 0 {
		t.Errorf("extractGuildID(nil) = %d, want 0", got)
	}
	id := snowflake.ID(987654321)
	if got := extractGuildID(&id); got != 987654321 {
		t.Errorf("extractGuildID(&id) = %d, want 987654321", got)
	}
}

func TestSanitizeToken_Positive_And_Negative(t *testing.T) {
	const secret = "token-secret-xyz-987"

	if got := sanitizeToken(nil, secret); got != nil {
		t.Errorf("sanitizeToken(nil) = %v, want nil", got)
	}

	plainErr := errors.New("connection failed to remote server")
	if got := sanitizeToken(plainErr, secret); !errors.Is(got, plainErr) {
		t.Errorf("sanitizeToken(plainErr) = %v, want %v", got, plainErr)
	}

	leakyErr := errors.New("unauthorized request with token token-secret-xyz-987 to api")
	sanitized := sanitizeToken(leakyErr, secret)
	if strings.Contains(sanitized.Error(), secret) {
		t.Errorf("sanitized error still contains secret: %v", sanitized)
	}
	if !strings.Contains(sanitized.Error(), "[redacted]") {
		t.Errorf("sanitized error does not contain [redacted]: %v", sanitized)
	}
}

func TestNewHandler_Negative_ValidationErrors(t *testing.T) {
	asker := &fakeAsker{}

	cases := []struct {
		name string
		cfg  HandlerConfig
	}{
		{
			name: "nil asker",
			cfg: HandlerConfig{
				Asker:   nil,
				GuildID: 42,
			},
		},
		{
			name: "zero guild ID",
			cfg: HandlerConfig{
				Asker:   asker,
				GuildID: 0,
			},
		},
		{
			name: "negative max concurrent",
			cfg: HandlerConfig{
				Asker:         asker,
				GuildID:       42,
				MaxConcurrent: -1,
			},
		},
		{
			name: "max concurrent over 64",
			cfg: HandlerConfig{
				Asker:         asker,
				GuildID:       42,
				MaxConcurrent: 65,
			},
		},
		{
			name: "negative ask timeout",
			cfg: HandlerConfig{
				Asker:      asker,
				GuildID:    42,
				AskTimeout: -time.Second,
			},
		},
		{
			name: "ask timeout over 10m",
			cfg: HandlerConfig{
				Asker:      asker,
				GuildID:    42,
				AskTimeout: 11 * time.Minute,
			},
		},
		{
			name: "negative reply timeout",
			cfg: HandlerConfig{
				Asker:        asker,
				GuildID:      42,
				ReplyTimeout: -time.Second,
			},
		},
		{
			name: "reply timeout over 1m",
			cfg: HandlerConfig{
				Asker:        asker,
				GuildID:      42,
				ReplyTimeout: 2 * time.Minute,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewHandler(tc.cfg)
			if err == nil {
				t.Fatalf("NewHandler() = %v, want error", h)
			}
		})
	}
}

func TestRegisterCommands_Negative_ValidationErrors(t *testing.T) {
	if err := RegisterCommands(context.Background(), "", 42); err == nil {
		t.Errorf("RegisterCommands with empty token succeeded, want error")
	}
	if err := RegisterCommands(context.Background(), "token", 0); err == nil {
		t.Errorf("RegisterCommands with zero guild ID succeeded, want error")
	}
}

func TestRun_Negative_ValidationErrors(t *testing.T) {
	asker := &fakeAsker{}
	h, err := NewHandler(HandlerConfig{Asker: asker, GuildID: 42})
	if err != nil {
		t.Fatalf("NewHandler() = %v", err)
	}

	cases := []struct {
		name string
		cfg  BotConfig
	}{
		{
			name: "empty token",
			cfg: BotConfig{
				Token:   "",
				GuildID: 42,
				Handler: h,
			},
		},
		{
			name: "zero guild ID",
			cfg: BotConfig{
				Token:   "some-token",
				GuildID: 0,
				Handler: h,
			},
		},
		{
			name: "nil handler",
			cfg: BotConfig{
				Token:   "some-token",
				GuildID: 42,
				Handler: nil,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Run(context.Background(), tc.cfg); err == nil {
				t.Errorf("Run() with %s succeeded, want error", tc.name)
			}
		})
	}
}
