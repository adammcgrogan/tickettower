package ticketbot

import (
	"context"
	"errors"
	"fmt"
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

// ErrChannelDeleted means a ticket's channel was deleted in Discord without
// the bot noticing. The ticket has been closed.
var ErrChannelDeleted = errors.New("ticket channel was deleted")

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

// Reopen reopens a closed thread ticket for a dashboard user and posts the
// reopen message in the thread. The bot picks the thread up again when it
// sees it unarchived.
func (d *Dashboard) Reopen(ctx context.Context, t store.Ticket, byID snowflake.ID) (store.Ticket, error) {
	t, err := d.b.reopenTicket(ctx, t, byID, nil)
	if err != nil {
		return t, err
	}
	if _, err := d.b.rest.CreateMessage(t.ChannelID, reopenedMessage(byID), rest.WithCtx(ctx)); err != nil {
		d.b.log.Warn("failed to post reopen message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	return t, nil
}

// Move moves an open ticket to another ticket type for a dashboard user, as
// /ticket move does, and returns the updated ticket.
func (d *Dashboard) Move(ctx context.Context, t store.Ticket, typeID int64, byID snowflake.ID) (store.Ticket, error) {
	to, err := d.b.store.GetTicketType(ctx, t.GuildID, typeID)
	if errors.Is(err, store.ErrNotFound) {
		return t, userErr("That ticket type no longer exists.")
	} else if err != nil {
		return t, err
	}
	var from *store.TicketType
	if t.TicketTypeID != nil {
		if v, err := d.b.store.GetTicketType(ctx, t.GuildID, *t.TicketTypeID); err == nil {
			from = &v
		}
	}
	moved, err := d.b.moveTicket(ctx, t, from, to, byID)
	if discordx.IsCode(err, discordx.CodeUnknownChannel) {
		d.b.closeDeletedChannel(t.ChannelID)
		return t, ErrChannelDeleted
	}
	return moved, err
}

// Reply posts a dashboard user's message in an open ticket, with its
// placeholders filled in, and returns it as saved in the transcript. byName
// and avatarURL are how the member knows the staff member, ideally their
// server nickname and avatar.
func (d *Dashboard) Reply(ctx context.Context, t store.Ticket, byID snowflake.ID, byName, avatarURL, text string) (store.TicketMessage, error) {
	server, serverIcon := d.b.guildBrand(ctx, t.GuildID)
	reply := replyMessage(d.b.cfg.AppName, byName, avatarURL, server, serverIcon, replyText(text, t, server, byName))
	m, err := d.b.rest.CreateMessage(t.ChannelID, reply, rest.WithCtx(ctx))
	if discordx.IsCode(err, discordx.CodeUnknownChannel) {
		// Deleted while the bot was offline, so the ticket was never closed.
		d.b.closeDeletedChannel(t.ChannelID)
		return store.TicketMessage{}, ErrChannelDeleted
	} else if err != nil {
		return store.TicketMessage{}, err
	}
	msg := toTicketMessage(t.ID, *m)

	// The bot captures the message too; whichever insert comes second is
	// ignored. Saving it here means the transcript has it straight away.
	if err := d.b.store.InsertTicketMessage(ctx, msg); err != nil {
		d.b.log.Error("failed to save dashboard reply", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	d.b.recordReply(ctx, t, byID, m.CreatedAt)
	return msg, nil
}

// guildBrand returns a guild's name and icon URL, for reply footers.
func (b *Bot) guildBrand(ctx context.Context, id snowflake.ID) (name, iconURL string) {
	name = b.guildName(ctx, id)
	if g, err := b.store.GetGuild(ctx, id); err == nil && g.Icon != nil {
		iconURL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.png", g.ID, *g.Icon)
	}
	return name, iconURL
}

// replyMessage is a staff reply the bot posts, from the dashboard or with
// /reply. The embed names the staff member at the top and says at the bottom
// that the server's staff sent it, so members know it's a real reply from
// the team. appName is set for replies from the dashboard, to say so.
func replyMessage(appName, staff, staffAvatar, server, serverIcon, text string) discord.MessageCreate {
	footer := fmt.Sprintf("Sent by %s staff", server)
	if appName != "" {
		footer += fmt.Sprintf(" from the %s Dashboard", appName)
	}
	return discord.NewMessageCreate().
		WithEmbeds(discord.NewEmbed().
			WithAuthor(staff, "", staffAvatar).
			WithDescription(text).
			WithColor(colorAccent).
			WithFooter(footer, serverIcon)).
		WithAllowedMentions(&discord.AllowedMentions{})
}
