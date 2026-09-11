// Package logging configures the process-wide slog logger.
package logging

import (
	"log/slog"
	"os"

	"github.com/adammcgrogan/ticketsbot/internal/config"
)

// New returns a text logger in development and a JSON logger in production,
// and installs it as the slog default.
//
// Debug logging is opt-in via LOG_LEVEL=debug because disgo's debug output
// includes raw gateway payloads, including the bot token.
func New(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if os.Getenv("LOG_LEVEL") == "debug" {
		opts.Level = slog.LevelDebug
	}

	var h slog.Handler
	if cfg.IsDev() {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	log := slog.New(h)
	slog.SetDefault(log)
	return log
}
