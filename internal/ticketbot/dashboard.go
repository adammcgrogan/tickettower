package ticketbot

import (
	"context"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/config"
	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// Closer closes tickets from the dashboard. It does what the bot does when a
// ticket closes (the close message, log entry, DM and cleanup) over Discord's
// REST API, so it works in the API process without a gateway connection.
//
// The bot notices a deleted ticket channel and forgets it. An archived thread
// stays in the bot's ticketCache until it restarts, which only matters if
// someone posts in the locked thread.
type Closer struct{ b *Bot }

func NewCloser(cfg config.Config, st *store.Store, r rest.Rest, log *slog.Logger) *Closer {
	return &Closer{b: &Bot{cfg: cfg, store: st, rest: r, log: log, tickets: newTicketCache()}}
}

// Close closes an open ticket for a dashboard user, reporting false if it
// was already closed. The channel is deleted (or the thread archived) in the
// background after the usual delay.
func (c *Closer) Close(ctx context.Context, t store.Ticket, byID snowflake.ID, byName, reason string) (bool, error) {
	ok, err := c.b.store.CloseTicket(ctx, t.ID, byID, byName, reason)
	if err != nil || !ok {
		return false, err
	}
	now := time.Now()
	t.Status, t.ClosedBy, t.ClosedByName, t.CloseReason, t.ClosedAt = store.StatusClosed, &byID, &byName, reason, &now

	_, err = c.b.rest.CreateMessage(t.ChannelID, closedMessage(t), rest.WithCtx(ctx))
	if err != nil && !discordx.IsCode(err, discordx.CodeUnknownChannel) {
		c.b.log.Warn("failed to post close message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	go c.b.finishClose(t)
	return true, nil
}
