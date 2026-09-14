package ticketbot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

const (
	autoCloseInterval = 5 * time.Minute
	keepOpenButtonID  = "/ticket-btn/keep-open"
)

// autoCloseTickets warns members about inactive tickets and later closes
// them, checking every few minutes. Every step is a conditional update, so a
// reply that lands at the last moment always wins. The same loop retries the
// channel cleanup of closed tickets whose channel was left behind.
func (b *Bot) autoCloseTickets(ctx context.Context) {
	ticker := time.NewTicker(autoCloseInterval)
	defer ticker.Stop()
	for {
		b.runAutoClose(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (b *Bot) runAutoClose(ctx context.Context, now time.Time) {
	warn, err := b.store.TicketsToWarn(ctx, now)
	if err != nil && ctx.Err() == nil {
		b.log.Error("failed to find tickets to warn", slog.Any("err", err))
	}
	for _, it := range warn {
		b.warnInactive(ctx, it, now)
	}

	due, err := b.store.TicketsToAutoClose(ctx, now)
	if err != nil && ctx.Err() == nil {
		b.log.Error("failed to find tickets to auto-close", slog.Any("err", err))
	}
	for _, it := range due {
		b.autoClose(ctx, it, now)
	}

	b.remindStaff(ctx, now)
	b.cleanUpClosedTickets(ctx, now)
	b.deleteKeptChannels(ctx, now)
}

func (b *Bot) warnInactive(ctx context.Context, it store.InactiveTicket, now time.Time) {
	ok, err := b.store.MarkAutoCloseWarned(ctx, it.ID, now)
	if err != nil {
		b.log.Error("failed to mark ticket warned", slog.Any("err", err))
	}
	if !ok {
		return
	}
	if _, err := b.rest.CreateMessage(it.ChannelID, inactivityWarning(it), rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to warn inactive ticket", slog.Int64("ticket_id", it.ID), slog.Any("err", err))
		// Try again next time rather than closing without a warning.
		if err := b.store.ClearAutoCloseWarning(ctx, it.ID); err != nil {
			b.log.Error("failed to clear ticket warning", slog.Any("err", err))
		}
	}
}

func inactivityWarning(it store.InactiveTicket) discord.MessageCreate {
	embed := discord.NewEmbed().
		WithTitle("Still need help?").
		WithDescription(fmt.Sprintf("This ticket has been quiet for a while, so it will close automatically <t:%d:R>.\n"+
			"Send a message or click below to keep it open.", it.CloseAt.Unix())).
		WithColor(colorAccent)
	return discord.NewMessageCreate().
		WithContent(discord.UserMention(it.OpenerID)).
		WithEmbeds(embed).
		AddActionRow(
			discord.NewPrimaryButton("I still need help", keepOpenButtonID).WithEmoji(discord.ComponentEmoji{Name: "👋"}),
			discord.NewDangerButton("Close ticket", closeButtonID).WithEmoji(discord.ComponentEmoji{Name: "🔒"}),
		).
		WithAllowedMentions(&discord.AllowedMentions{Users: []snowflake.ID{it.OpenerID}})
}

func (b *Bot) autoClose(ctx context.Context, it store.InactiveTicket, now time.Time) {
	ok, err := b.store.AutoCloseTicket(ctx, it.ID, "No activity for "+autoCloseLabel(it.Hours), now)
	if err != nil {
		b.log.Error("failed to auto-close ticket", slog.Any("err", err))
	}
	if !ok {
		return // someone replied just in time
	}
	t, err := b.store.GetTicket(ctx, it.ID)
	if err != nil {
		b.log.Error("failed to load auto-closed ticket", slog.Any("err", err))
		return
	}
	_, err = b.rest.CreateMessage(t.ChannelID, closedMessage(t, b.typeOf(ctx, t)), rest.WithCtx(ctx))
	if err != nil && !discordx.IsCode(err, discordx.CodeUnknownChannel) {
		b.log.Warn("failed to post auto-close message", slog.Any("err", err))
	}
	go b.finishClose(t)
	b.log.Info("ticket auto-closed", slog.String("guild_id", t.GuildID.String()), slog.Int("number", t.Number))
}

// handleKeepOpen answers the warning's "I still need help" button.
func (b *Bot) handleKeepOpen(e *handler.ComponentEvent) error {
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	t, _, err := b.loadTicket(ctx, e.Channel().ID())
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	user := e.User()
	if err := b.store.RecordActivity(ctx, t.ID, time.Now(), user.ID == t.OpenerID); err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.UpdateMessage(discord.NewMessageUpdate().
		ClearContent().
		WithEmbeds(discord.NewEmbed().
			WithTitle("Ticket kept open").
			WithDescription(discord.UserMention(user.ID) + " kept this ticket open.").
			WithColor(colorAccent)).
		ClearComponents())
}

// autoCloseLabel describes an auto-close window, e.g. "12 hours", "2 days"
// or "1 week".
func autoCloseLabel(hours int) string {
	n, unit := hours, "hour"
	switch {
	case hours%168 == 0:
		n, unit = hours/168, "week"
	case hours%24 == 0:
		n, unit = hours/24, "day"
	}
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
