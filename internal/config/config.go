// Package config loads the bot's settings from the process environment.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	// EnvTokenFile names the variable holding the path of the Discord bot token file.
	EnvTokenFile = "DISCORD_BOT_TOKEN_FILE" // #nosec G101 -- the name of an environment variable, not a credential.
	// EnvGuildID names the variable holding the snowflake of the hub guild.
	EnvGuildID = "DISCORD_BOT_GUILD_ID"
	// MaxTokenBytes bounds the token file. A Discord bot token is far shorter.
	MaxTokenBytes = 4096
)

// Config is the validated configuration of one bot process.
type Config struct {
	// Token is the Discord bot token. It comes from a file, never from the environment itself,
	// and must never be logged.
	Token string
	// GuildID is the snowflake of the hub guild the bot serves.
	GuildID uint64
}

// Load reads the configuration through getenv. It fails closed: a missing variable, an
// unreadable, empty or oversized token file, or a guild id that is not a non-zero decimal
// snowflake is an error.
func Load(getenv func(string) string) (Config, error) {
	guild, err := parseGuildID(getenv(EnvGuildID))
	if err != nil {
		return Config{}, err
	}
	token, err := readToken(getenv(EnvTokenFile))
	if err != nil {
		return Config{}, err
	}
	return Config{Token: token, GuildID: guild}, nil
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

func readToken(path string) (token string, err error) {
	if path == "" {
		return "", fmt.Errorf("%s is not set", EnvTokenFile)
	}
	file, err := os.Open(path) // #nosec G304 -- the operator names the token file.
	if err != nil {
		return "", fmt.Errorf("open token file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close token file: %w", closeErr))
		}
	}()
	data, err := io.ReadAll(io.LimitReader(file, MaxTokenBytes+1))
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	if len(data) > MaxTokenBytes {
		return "", fmt.Errorf("token file is larger than %d bytes", MaxTokenBytes)
	}
	token = strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("token file is empty")
	}
	return token, nil
}
