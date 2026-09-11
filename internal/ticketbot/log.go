package ticketbot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// logEvent posts a ticket event to the guild's log channel, if it has one.
// The log is best effort: failures are logged and never reach members.
func (b *Bot) logEvent(guildID snowflake.ID, msg discord.MessageCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := b.store.GetGuildSettings(ctx, guildID)
	if err != nil {
		b.log.Error("failed to load log channel", slog.Any("err", err))
		return
	}
	if st.LogChannelID == nil {
		return
	}
	// Log entries mention people for context but never ping them.
	msg = msg.WithAllowedMentions(&discord.AllowedMentions{})
	if _, err := b.rest.CreateMessage(*st.LogChannelID, msg, rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to post to log channel", slog.String("guild_id", guildID.String()), slog.Any("err", err))
	}
}

func ticketTitle(t store.Ticket, action string) string {
	return fmt.Sprintf("%s #%d %s", t.TypeName, t.Number, action)
}

func openedLog(t store.Ticket) discord.MessageCreate {
	embed := discord.NewEmbed().
		WithTitle(ticketTitle(t, "opened")).
		WithColor(colorAccent).
		AddField("Opened by", discord.UserMention(t.OpenerID), true).
		AddField("Ticket", discord.ChannelMention(t.ChannelID), true).
		WithTimestamp(t.OpenedAt)
	return discord.NewMessageCreate().WithEmbeds(embed)
}

func claimedLog(t store.Ticket, claimer snowflake.ID) discord.MessageCreate {
	embed := discord.NewEmbed().
		WithTitle(ticketTitle(t, "claimed")).
		WithColor(colorAccent).
		AddField("Claimed by", discord.UserMention(claimer), true).
		AddField("Opened by", discord.UserMention(t.OpenerID), true).
		AddField("Ticket", discord.ChannelMention(t.ChannelID), true).
		WithTimestamp(time.Now())
	return discord.NewMessageCreate().WithEmbeds(embed)
}

func (b *Bot) closedLog(t store.Ticket) discord.MessageCreate {
	closedAt := time.Now()
	if t.ClosedAt != nil {
		closedAt = *t.ClosedAt
	}
	embed := discord.NewEmbed().
		WithTitle(ticketTitle(t, "closed")).
		WithColor(colorMuted).
		AddField("Opened by", discord.UserMention(t.OpenerID), true)
	if t.ClosedBy != nil {
		embed = embed.AddField("Closed by", discord.UserMention(*t.ClosedBy), true)
	}
	embed = embed.AddField("Open for", humanDuration(closedAt.Sub(t.OpenedAt)), true)
	if t.CloseReason != "" {
		embed = embed.AddField("Reason", t.CloseReason, false)
	}
	return discord.NewMessageCreate().
		WithEmbeds(embed.WithTimestamp(closedAt)).
		WithComponents(b.transcriptRow(t.ID)...)
}

// humanDuration formats d briefly, e.g. "45m", "3h 12m" or "2d 4h".
func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}
