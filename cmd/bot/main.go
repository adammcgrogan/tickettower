package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/adammcgrogan/tickettower/internal/config"
	"github.com/adammcgrogan/tickettower/internal/logging"
	"github.com/adammcgrogan/tickettower/internal/store"
	"github.com/adammcgrogan/tickettower/internal/ticketbot"
)

func main() {
	if err := run(); err != nil {
		slog.Error("bot exited", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load("DATABASE_URL", "DISCORD_TOKEN")
	if err != nil {
		return err
	}
	log := logging.New(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	b, err := ticketbot.New(cfg, st, log)
	if err != nil {
		return err
	}
	return b.Run(ctx)
}
