package ticketbot

import (
	"context"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/config"
	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// Dashboard acts on tickets for dashboard users: closing them (the close
// message, log entry, DM and cleanup, as in Discord) and replying to them.
// It uses Discord's REST API, so it works in the API process without a
// gateway connection.
//
// The bot notices a deleted ticket channel and forgets it. An archived thread
// stays in the bot's ticketCache until it restarts, which only matters if
// someone posts in the locked thread.
type Dashboard struct{ b *Bot }

func NewDashboard(cfg config.Config, st *store.Store, r rest.Rest, log *slog.Logger) *Dashboard {
	return &Dashboard{b: &Bot{cfg: cfg, store: st, rest: r, log: log, tickets: newTicketCache()}}
}

// Close closes an open ticket for a dashboard user, reporting false if it
// was already closed. The channel is deleted (or the thread archived) in the
// background after the usual delay.
func (d *Dashboard) Close(ctx context.Context, t store.Ticket, byID snowflake.ID, byName, reason string) (bool, error) {
	ok, err := d.b.store.CloseTicket(ctx, t.ID, byID, byName, reason)
	if err != nil || !ok {
		return false, err
	}
	now := time.Now()
	t.Status, t.ClosedBy, t.ClosedByName, t.CloseReason, t.ClosedAt = store.StatusClosed, &byID, &byName, reason, &now

	_, err = d.b.rest.CreateMessage(t.ChannelID, closedMessage(t), rest.WithCtx(ctx))
	if err != nil && !discordx.IsCode(err, discordx.CodeUnknownChannel) {
		d.b.log.Warn("failed to post close message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	go d.b.finishClose(t)
	return true, nil
}

// Reply posts a dashboard user's message in an open ticket and returns it as
// saved in the transcript. The bot skips its own messages when tracking
// replies, so this records the team's first response and restarts the
// auto-close clock itself. Once the message is posted the reply has worked,
// so bookkeeping failures are only logged.
func (d *Dashboard) Reply(ctx context.Context, t store.Ticket, byID snowflake.ID, byName, avatarURL, text string) (store.TicketMessage, error) {
	m, err := d.b.rest.CreateMessage(t.ChannelID, replyMessage(byName, avatarURL, text), rest.WithCtx(ctx))
	if err != nil {
		return store.TicketMessage{}, err
	}
	msg := toTicketMessage(t.ID, *m)
	log := d.b.log.With(slog.Int64("ticket_id", t.ID))

	// The bot captures the message too; whichever insert comes second is
	// ignored. Saving it here means the transcript has it straight away.
	if err := d.b.store.InsertTicketMessage(ctx, msg); err != nil {
		log.Error("failed to save dashboard reply", slog.Any("err", err))
	}
	byOpener := byID == t.OpenerID
	if !byOpener {
		if err := d.b.store.SetFirstResponse(ctx, t.ID, m.CreatedAt); err != nil {
			log.Error("failed to record first response", slog.Any("err", err))
		}
	}
	if err := d.b.store.RecordActivity(ctx, t.ID, m.CreatedAt, byOpener); err != nil {
		log.Error("failed to record ticket activity", slog.Any("err", err))
	}
	return msg, nil
}

// replyMessage is a reply sent from the dashboard. The bot posts it, with the
// staff member's name and avatar as the embed's author.
func replyMessage(name, avatarURL, text string) discord.MessageCreate {
	return discord.NewMessageCreate().
		WithEmbeds(discord.NewEmbed().
			WithAuthor(name, "", avatarURL).
			WithDescription(text).
			WithColor(colorAccent).
			WithFooterText("Sent from the dashboard")).
		WithAllowedMentions(&discord.AllowedMentions{})
}
