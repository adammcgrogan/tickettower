package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/disgoorg/disgo/rest"
	"github.com/redis/go-redis/v9"

	"github.com/adammcgrogan/ticketsbot/internal/api"
	"github.com/adammcgrogan/ticketsbot/internal/auth"
	"github.com/adammcgrogan/ticketsbot/internal/config"
	"github.com/adammcgrogan/ticketsbot/internal/logging"
	"github.com/adammcgrogan/ticketsbot/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api exited", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load("DATABASE_URL", "REDIS_URL", "DISCORD_TOKEN", "DISCORD_CLIENT_ID", "DISCORD_CLIENT_SECRET")
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

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	am := auth.NewManager(cfg.DiscordClientID, cfg.DiscordClientSecret, cfg.PublicURL, rdb)
	// The API talks to Discord as the bot to publish panels and read
	// channels and roles.
	discordRest := rest.New(rest.NewClient(cfg.DiscordToken))
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewServer(cfg, st, am, discordRest, log).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
