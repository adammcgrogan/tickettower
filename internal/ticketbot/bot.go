// Package ticketbot is the Discord gateway side of the application.
package ticketbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/config"
	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/panels"
	"github.com/adammcgrogan/tickettower/internal/store"
)

type Bot struct {
	client *bot.Client
	// rest is client.Rest, or a plain REST client when the dashboard uses the
	// bot's ticket logic without a gateway connection (client is then nil).
	rest  rest.Rest
	store *store.Store
	log   *slog.Logger
	cfg   config.Config

	// readyGuilds collects guild IDs seen during startup so guilds removed
	// while the bot was offline can be marked as left.
	readyMu     sync.Mutex
	readyGuilds []snowflake.ID

	// openLocks serialises ticket creation per guild member so double
	// clicks can't bypass the open ticket limit. Keys are dropped once
	// nobody holds them.
	openLocks keyedLocks

	tickets *ticketCache
}

func New(cfg config.Config, st *store.Store, log *slog.Logger) (*Bot, error) {
	b := &Bot{store: st, log: log, cfg: cfg, tickets: newTicketCache()}

	r := handler.New()
	r.Use(middleware.Go)
	r.Command("/ping", b.handlePing)
	r.Route("/ticket", func(r handler.Router) {
		r.Command("/open", b.handleOpenCommand)
		r.Autocomplete("/open", b.handleOpenAutocomplete)
		r.Command("/close", b.handleCloseCommand)
		r.Command("/claim", b.handleClaimCommand)
		r.Command("/closerequest", b.handleCloseRequestCommand)
		r.Command("/add", b.handleAddCommand)
		r.Command("/remove", b.handleRemoveCommand)
		r.Command("/move", b.handleMoveCommand)
		r.Autocomplete("/move", b.handleMoveAutocomplete)
		r.Command("/rename", b.handleRenameCommand)
		r.Command("/note", b.handleNoteCommand)
		r.Command("/hold", b.handleHoldCommand)
		r.Command("/resume", b.handleResumeCommand)
		r.Command("/block", b.handleBlockCommand)
		r.Command("/unblock", b.handleUnblockCommand)
		r.Command("/available", b.handleAvailableCommand)
	})
	r.Command("/close", b.handleCloseCommand)
	r.Command("/reply", b.handleReplyCommand)
	r.Autocomplete("/reply", b.handleReplyAutocomplete)
	r.Component(panels.OpenButtonPrefix+"{typeID}", b.handleOpenButton)
	r.Component(panels.OpenSelectID, b.handleOpenSelect)
	r.Component(answerOpenButtonPrefix+"{typeID}", b.handleAnswerOpen)
	r.Component(answerCloseButtonPrefix+"{typeID}", b.handleAnswerClose)
	r.Modal(formModalPrefix+"{source}/{typeID}/{version}", b.handleFormModal)
	r.Component(claimButtonID, b.handleClaimButton)
	r.Component(closeButtonID, b.handleCloseButton)
	r.Component(closeConfirmButtonID, b.handleCloseConfirm)
	r.Component(reopenButtonPrefix+"{ticketID}", b.handleReopenButton)
	r.Component(keepOpenButtonID, b.handleKeepOpen)
	r.Component(closeRequestAcceptID, b.handleCloseRequestAccept)
	r.Component(closeRequestKeepID, b.handleCloseRequestKeep)
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
			bot.NewListenerFunc(b.onThreadUpdate),
			bot.NewListenerFunc(b.onMessageCreate),
			bot.NewListenerFunc(b.onMessageUpdate),
			bot.NewListenerFunc(b.onMessageDelete),
		),
	)
	if err != nil {
		return nil, err
	}
	b.client = client
	b.rest = client.Rest
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
	go b.postServerCounts(ctx, b.client.ID())

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
	b.closeTicketsOfLeftGuilds(ctx)
	go b.closeMissingTickets(0)
}

// leftReason is the close reason of tickets the bot could no longer serve.
const leftReason = "The bot was removed from the server"

// closeTicketsOfLeftGuilds closes the open tickets of every server the bot
// has left, so they leave the queues, the loops and the ticket cache. Their
// channels are out of reach, so they're not touched.
func (b *Bot) closeTicketsOfLeftGuilds(ctx context.Context) {
	n, err := b.store.CloseTicketsOfLeftGuilds(ctx, leftReason)
	if err != nil {
		b.log.Error("failed to close tickets of left guilds", slog.Any("err", err))
		return
	}
	if n > 0 {
		b.log.Info("closed tickets in servers the bot has left", slog.Int64("count", n))
	}
}

// closeMissingTickets closes tickets whose channel was deleted while the bot
// was away (a restart or deploy, or between leaving and rejoining a server),
// since Discord's delete event never reached it. guildID limits it to one
// server; 0 checks every tracked ticket.
func (b *Bot) closeMissingTickets(guildID snowflake.ID) {
	closed := 0
	for _, id := range b.tickets.channelIDs(guildID) {
		if !b.channelExists(id) {
			b.closeDeletedChannel(id)
			closed++
		}
	}
	if closed > 0 {
		b.log.Info("closed tickets whose channel was deleted", slog.Int("count", closed))
	}
}

// channelExists reports whether a channel or thread still exists. Anything
// but a clear "unknown channel" from Discord (no access, a timeout) counts
// as existing, so a ticket is never closed by mistake.
func (b *Bot) channelExists(id snowflake.ID) bool {
	// Archived threads aren't cached, so a miss is checked with Discord.
	if _, ok := b.client.Caches.Channel(id); ok {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := b.rest.GetChannel(id, rest.WithCtx(ctx))
	return !discordx.IsCode(err, discordx.CodeUnknownChannel)
}

func (b *Bot) onGuildJoin(e *events.GuildJoin) {
	b.log.Info("joined guild", slog.String("guild_id", e.Guild.ID.String()), slog.String("name", e.Guild.Name))
	b.upsertGuild(e.Guild.Guild)
	// Tickets from before the bot left are closed by now; tickets opened
	// while it was here before a restart may have lost their channel.
	go b.closeMissingTickets(e.Guild.ID)
	if e.Guild.SystemChannelID != nil {
		go b.welcome(e.Guild.ID, *e.Guild.SystemChannelID)
	}
}

// welcome points whoever added the bot to the dashboard, in the server's
// system channel. Best effort: the bot may not be allowed to post there.
func (b *Bot) welcome(guildID, channelID snowflake.ID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	url := fmt.Sprintf("%s/servers/%s", b.cfg.PublicURL, guildID)
	desc := "Members will be able to open a private ticket with your team from a button, and every conversation " +
		"is saved as a transcript.\n\nTo get started, choose where tickets open and who handles them, then post " +
		"your ticket panel. It takes about a minute in the dashboard."
	msg := discord.NewMessageCreate()
	// Discord rejects link buttons to non-https URLs.
	if strings.HasPrefix(url, "https://") {
		msg = msg.AddActionRow(discord.NewLinkButton("Set up tickets", url))
	} else {
		desc += "\n" + url
	}
	msg = msg.WithEmbeds(discord.NewEmbed().
		WithTitle("Thanks for adding " + b.cfg.AppName).
		WithDescription(desc).
		WithColor(colorAccent))
	if _, err := b.rest.CreateMessage(channelID, msg, rest.WithCtx(ctx)); err != nil {
		b.log.Info("couldn't post welcome message", slog.String("guild_id", guildID.String()), slog.Any("err", err))
	}
}

func (b *Bot) onGuildLeave(e *events.GuildLeave) {
	b.log.Info("left guild", slog.String("guild_id", e.GuildID.String()))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.store.MarkGuildLeft(ctx, e.GuildID); err != nil {
		b.log.Error("failed to mark guild left", slog.Any("err", err))
		return
	}
	b.tickets.removeGuild(e.GuildID)
	b.closeTicketsOfLeftGuilds(ctx)
}

func (b *Bot) onChannelDelete(e *events.GuildChannelDelete) { b.closeDeletedChannel(e.ChannelID) }

// onThreadUpdate tracks a ticket thread again when it's unarchived, which
// happens when a ticket is reopened from the dashboard (a different process
// from this cache).
func (b *Bot) onThreadUpdate(e *events.ThreadUpdate) {
	if e.Thread.ThreadMetadata.Archived || !e.OldThread.ThreadMetadata.Archived {
		return
	}
	if _, ok := b.tickets.get(e.ThreadID); ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	t, err := b.store.GetTicketByChannel(ctx, e.ThreadID)
	if err != nil || t.Status != store.StatusOpen {
		return
	}
	b.tickets.put(store.TicketRef{ID: t.ID, GuildID: t.GuildID, ChannelID: t.ChannelID, OpenerID: t.OpenerID, HasFirstResponse: t.FirstResponseAt != nil})
}
func (b *Bot) onThreadDelete(e *events.ThreadDelete) { b.closeDeletedChannel(e.ThreadID) }

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
		// Not a ticket, or already closed: if its channel was being kept,
		// it can't be reopened any more.
		b.forgetKeptChannel(ctx, channelID)
		return
	}
	if t, err := b.store.GetTicketByChannel(ctx, channelID); err == nil {
		go b.logEvent(t.GuildID, b.closedLog(t))
	}
}
