// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// BotPermissions are the permissions requested when a server adds the bot.
const BotPermissions = discord.PermissionViewChannel |
	discord.PermissionManageChannels |
	discord.PermissionManageRoles |
	discord.PermissionSendMessages |
	discord.PermissionSendMessagesInThreads |
	discord.PermissionCreatePrivateThreads |
	discord.PermissionManageThreads |
	discord.PermissionEmbedLinks |
	discord.PermissionAttachFiles |
	discord.PermissionReadMessageHistory |
	discord.PermissionUseExternalEmojis

type Config struct {
	AppName string
	Env     string // "development" or "production"

	DatabaseURL string
	RedisURL    string

	DiscordToken        string
	DiscordClientID     snowflake.ID
	DiscordClientSecret string
	// DevGuildID, when set, registers slash commands to a single guild so
	// they update instantly during development.
	DevGuildID snowflake.ID

	// PublicURL is the externally reachable base URL of the dashboard,
	// e.g. https://relay.example.com (no trailing slash).
	PublicURL string
	Port      string
	StaticDir string
}

func (c Config) IsDev() bool { return c.Env != "production" }

// Load reads configuration from the environment and fails if any of the
// required variables are unset.
func Load(required ...string) (Config, error) {
	var missing []string
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	cfg := Config{
		AppName:             get("APP_NAME", "Relay"),
		Env:                 get("APP_ENV", "development"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		DiscordToken:        os.Getenv("DISCORD_TOKEN"),
		DiscordClientSecret: os.Getenv("DISCORD_CLIENT_SECRET"),
		PublicURL:           strings.TrimRight(get("PUBLIC_URL", "http://localhost:5173"), "/"),
		Port:                get("PORT", "8080"),
		StaticDir:           get("STATIC_DIR", "web/build"),
	}

	var err error
	if cfg.DiscordClientID, err = optionalID("DISCORD_CLIENT_ID"); err != nil {
		return Config{}, err
	}
	if cfg.DevGuildID, err = optionalID("DEV_GUILD_ID"); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func optionalID(key string) (snowflake.ID, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, nil
	}
	id, err := snowflake.Parse(v)
	if err != nil {
		return 0, fmt.Errorf("%s is not a valid Discord ID: %w", key, err)
	}
	return id, nil
}
