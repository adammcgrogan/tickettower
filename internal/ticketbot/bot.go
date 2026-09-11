// Package ticketbot is the Discord gateway side of the application.
package ticketbot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/handler/middleware"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/ticketsbot/internal/config"
	"github.com/adammcgrogan/ticketsbot/internal/panels"
	"github.com/adammcgrogan/ticketsbot/internal/store"
)

type Bot struct {
	client *bot.Client
	store  *store.Store
	log    *slog.Logger
	cfg    config.Config

	// readyGuilds collects guild IDs seen during startup so guilds removed
	// while the bot was offline can be marked as left.
	readyMu     sync.Mutex
	readyGuilds []snowflake.ID

	// openLocks serialises ticket creation per guild member so double
	// clicks can't bypass the open ticket limit.
	openLocks sync.Map

	tickets *ticketCache
}

func New(cfg config.Config, st *store.Store, log *slog.Logger) (*Bot, error) {
	b := &Bot{store: st, log: log, cfg: cfg, tickets: newTicketCache()}

	r := handler.New()
	r.Use(middleware.Go)
	r.Command("/ping", b.handlePing)
	r.Route("/ticket", func(r handler.Router) {
		r.Command("/close", b.handleCloseCommand)
		r.Command("/claim", b.handleClaimCommand)
		r.Command("/add", b.handleAddCommand)
		r.Command("/remove", b.handleRemoveCommand)
		r.Command("/rename", b.handleRenameCommand)
	})
	r.Command("/close", b.handleCloseCommand)
	r.Component(panels.OpenButtonPrefix+"{typeID}", b.handleOpenButton)
	r.Component(panels.OpenSelectID, b.handleOpenSelect)
	r.Modal(formModalPrefix+"{source}/{typeID}/{version}", b.handleFormModal)
	r.Component(claimButtonID, b.handleClaimButton)
	r.Component(closeButtonID, b.handleCloseButton)
	r.Component(keepOpenButtonID, b.handleKeepOpen)
	r.Modal(closeModalID, b.handleCloseModal)
	r.Component(rateButtonPrefix+"{ticketID}/{rating}", b.handleRate)
	r.Component(commentButtonPrefix+"{ticketID}", b.handleCommentButton)
	r.Modal(commentModalPrefix+"{ticketID}", b.handleCommentModal)

	client, err := disgo.New(cfg.DiscordToken,
		bot.WithGatewayConfigOpts(gateway.WithIntents(
			gateway.IntentGuilds,
			gateway.IntentGuildMessages,
			gateway.IntentMessageContent, // privileged: required for transcripts
		)),
		bot.WithCacheConfigOpts(cache.WithCaches(cache.FlagGuilds, cache.FlagChannels, cache.FlagRoles)),
		bot.WithEventListeners(
			r,
			bot.NewListenerFunc(b.onGuildReady),
			bot.NewListenerFunc(b.onGuildsReady),
			bot.NewListenerFunc(b.onGuildJoin),
			bot.NewListenerFunc(b.onGuildLeave),
			bot.NewListenerFunc(b.onChannelDelete),
			bot.NewListenerFunc(b.onThreadDelete),
			bot.NewListenerFunc(b.onMessageCreate),
			bot.NewListenerFunc(b.onMessageUpdate),
			bot.NewListenerFunc(b.onMessageDelete),
		),
	)
	if err != nil {
		return nil, err
	}
	b.client = client
	return b, nil
}

// Run syncs slash commands, connects to the gateway and blocks until ctx is
// cancelled.
func (b *Bot) Run(ctx context.Context) error {
	var guildIDs []snowflake.ID
	if b.cfg.DevGuildID != 0 {
		guildIDs = []snowflake.ID{b.cfg.DevGuildID}
	}
	if err := handler.SyncCommands(b.client, commands, guildIDs); err != nil {
		return err
	}
	refs, err := b.store.OpenTicketRefs(ctx)
	if err != nil {
		return err
	}
	b.tickets.replace(refs)

	if err := b.client.OpenGateway(ctx); err != nil {
		return err
	}
	b.log.Info("bot connected", slog.Int("open_tickets", len(refs)))
	go b.purgeTranscripts(ctx)
	go b.autoCloseTickets(ctx)

	<-ctx.Done()
	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b.client.Close(closeCtx)
	return nil
}

func (b *Bot) upsertGuild(g discord.Guild) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := b.store.UpsertGuild(ctx, store.Guild{ID: g.ID, Name: g.Name, Icon: g.Icon, OwnerID: g.OwnerID})
	if err != nil {
		b.log.Error("failed to save guild", slog.String("guild_id", g.ID.String()), slog.Any("err", err))
	}
}

func (b *Bot) onGuildReady(e *events.GuildReady) {
	b.readyMu.Lock()
	b.readyGuilds = append(b.readyGuilds, e.Guild.ID)
	b.readyMu.Unlock()
	b.upsertGuild(e.Guild.Guild)
}

// onGuildsReady fires once all guilds from the initial connection are loaded.
// NOTE: this assumes a single shard; revisit when sharding is enabled.
func (b *Bot) onGuildsReady(_ *events.GuildsReady) {
	b.readyMu.Lock()
	ids := b.readyGuilds
	b.readyGuilds = nil
	b.readyMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := b.store.MarkGuildsLeftExcept(ctx, ids); err != nil {
		b.log.Error("failed to reconcile guilds", slog.Any("err", err))
	}
	b.log.Info("guilds ready", slog.Int("count", len(ids)))
}

func (b *Bot) onGuildJoin(e *events.GuildJoin) {
	b.log.Info("joined guild", slog.String("guild_id", e.Guild.ID.String()), slog.String("name", e.Guild.Name))
	b.upsertGuild(e.Guild.Guild)
}

func (b *Bot) onGuildLeave(e *events.GuildLeave) {
	b.log.Info("left guild", slog.String("guild_id", e.GuildID.String()))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.store.MarkGuildLeft(ctx, e.GuildID); err != nil {
		b.log.Error("failed to mark guild left", slog.Any("err", err))
	}
}

func (b *Bot) onChannelDelete(e *events.GuildChannelDelete) { b.closeDeletedChannel(e.ChannelID) }
func (b *Bot) onThreadDelete(e *events.ThreadDelete)        { b.closeDeletedChannel(e.ThreadID) }

// closeDeletedChannel closes the ticket for a channel someone deleted by hand.
func (b *Bot) closeDeletedChannel(channelID snowflake.ID) {
	b.tickets.remove(channelID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	closed, err := b.store.CloseTicketByChannel(ctx, channelID, "Channel was deleted")
	if err != nil {
		b.log.Error("failed to close ticket for deleted channel", slog.Any("err", err))
		return
	}
	if !closed {
		return // not a ticket, or the bot already closed it
	}
	if t, err := b.store.GetTicketByChannel(ctx, channelID); err == nil {
		go b.logEvent(t.GuildID, b.closedLog(t))
	}
}
