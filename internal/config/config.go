// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// BotPermissions are the permissions requested when a server adds the bot.
// Servers that added it earlier keep their old grant until they re-invite it,
// so code can't assume a newly added permission is there.
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
	discord.PermissionUseExternalEmojis |
	// Lets the bot ping support roles that aren't mentionable, which is also
	// what adds them to private thread tickets.
	discord.PermissionMentionEveryone

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
	// e.g. https://tickettower.net (no trailing slash).
	PublicURL string
	Port      string
	StaticDir string
	// TrustedProxies is how many reverse proxies sit in front of the API
	// (Railway's edge is one). The client IP is read from X-Forwarded-For
	// past that many hops; 0 means the connection's own address is used and
	// forwarded headers are ignored, so nobody can spoof their IP.
	TrustedProxies int
	// SuperadminUserID, if set, is the only Discord user who can open the
	// /admin panel (install and usage stats across every guild).
	SuperadminUserID snowflake.ID

	// Stripe billing. All optional: a self-host without these set simply
	// never shows the billing UI (see BillingEnabled).
	StripeSecretKey     string
	StripeWebhookSecret string
	// StripePriceID is the one premium monthly price. It's fixed here,
	// server-side, and never accepted from a client request.
	StripePriceID string

	// Bot list API tokens. Each one set makes the bot post its server count
	// to that site; TopggToken also turns on the dashboard's review prompt.
	TopggToken          string
	DiscordBotListToken string
	DiscordBotsGGToken  string
}

// BillingEnabled reports whether Stripe billing is configured, so the
// dashboard knows whether to show the upgrade UI at all.
func (c Config) BillingEnabled() bool { return c.StripePriceID != "" }

func (c Config) IsDev() bool { return c.Env != "production" }

// ReviewURL is where the dashboard asks happy servers to leave a review: the
// bot's top.gg page, once it's listed there (a top.gg token is configured).
func (c Config) ReviewURL() string {
	if c.TopggToken == "" || c.DiscordClientID == 0 {
		return ""
	}
	return "https://top.gg/bot/" + c.DiscordClientID.String() + "#reviews"
}

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
		AppName:             get("APP_NAME", "Ticket Tower"),
		Env:                 get("APP_ENV", "development"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		DiscordToken:        os.Getenv("DISCORD_TOKEN"),
		DiscordClientSecret: os.Getenv("DISCORD_CLIENT_SECRET"),
		PublicURL:           strings.TrimRight(get("PUBLIC_URL", "http://localhost:5173"), "/"),
		Port:                get("PORT", "8080"),
		StaticDir:           get("STATIC_DIR", "web/build"),
		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePriceID:       os.Getenv("STRIPE_PRICE_ID"),
		TopggToken:          os.Getenv("TOPGG_TOKEN"),
		DiscordBotListToken: os.Getenv("DISCORDBOTLIST_TOKEN"),
		DiscordBotsGGToken:  os.Getenv("DISCORDBOTSGG_TOKEN"),
	}

	var err error
	if cfg.DiscordClientID, err = optionalID("DISCORD_CLIENT_ID"); err != nil {
		return Config{}, err
	}
	if cfg.DevGuildID, err = optionalID("DEV_GUILD_ID"); err != nil {
		return Config{}, err
	}
	if cfg.TrustedProxies, err = optionalCount("TRUSTED_PROXIES"); err != nil {
		return Config{}, err
	}
	if cfg.SuperadminUserID, err = optionalID("SUPERADMIN_USER_ID"); err != nil {
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

func optionalCount(key string) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a whole number of proxies, got %q", key, v)
	}
	return n, nil
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
