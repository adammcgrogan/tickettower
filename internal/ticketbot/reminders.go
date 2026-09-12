package ticketbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// remindStaff posts reminders for tickets the team has left waiting longer
// than their type allows. MarkReminded is conditional, so a reply that lands
// first, or a second process, stops the reminder.
func (b *Bot) remindStaff(ctx context.Context, now time.Time) {
	due, err := b.store.TicketsToRemind(ctx, now)
	if err != nil {
		if ctx.Err() == nil {
			b.log.Error("failed to find tickets to remind about", slog.Any("err", err))
		}
		return
	}
	for _, w := range due {
		ok, err := b.store.MarkReminded(ctx, w.ID, w.Minutes, now)
		if err != nil {
			b.log.Error("failed to mark ticket reminded", slog.Any("err", err))
		}
		if !ok {
			continue
		}
		waited := now.Sub(w.WaitingSince)
		if w.Where != store.RemindInLog {
			if _, err := b.rest.CreateMessage(w.ChannelID, reminderMessage(w, waited), rest.WithCtx(ctx)); err != nil {
				b.log.Warn("failed to post staff reminder", slog.Int64("ticket_id", w.ID), slog.Any("err", err))
			}
		}
		if w.Where != store.RemindInTicket {
			b.logEvent(w.GuildID, reminderLog(w, waited))
		}
	}
	if len(due) > 0 {
		b.log.Info("reminded staff about waiting tickets", slog.Int("count", len(due)))
	}
}

// reminderMentions is who a reminder pings, per the type's setting.
func reminderMentions(w store.WaitingTicket) (content string, allowed discord.AllowedMentions) {
	switch w.Ping {
	case store.PingNobody:
		return "", discord.AllowedMentions{}
	case store.PingClaimer:
		if w.ClaimedBy != nil {
			return discord.UserMention(*w.ClaimedBy), discord.AllowedMentions{Users: []snowflake.ID{*w.ClaimedBy}}
		}
	}
	roles := make([]string, len(w.SupportRoleIDs))
	for i, id := range w.SupportRoleIDs {
		roles[i] = discord.RoleMention(id)
	}
	return strings.Join(roles, " "), discord.AllowedMentions{Roles: w.SupportRoleIDs}
}

func reminderMessage(w store.WaitingTicket, waited time.Duration) discord.MessageCreate {
	content, allowed := reminderMentions(w)
	desc := fmt.Sprintf("%s has been waiting for a reply for **%s**.", discord.UserMention(w.OpenerID), humanDuration(waited))
	if w.RemindersSent > 0 {
		desc += fmt.Sprintf(" This is reminder %d.", w.RemindersSent+1)
	}
	msg := discord.NewMessageCreate().
		WithEmbeds(discord.NewEmbed().WithTitle("Still waiting on the team").WithDescription(desc).WithColor(colorAccent)).
		WithAllowedMentions(&allowed)
	if content != "" {
		msg = msg.WithContent(content)
	}
	if w.ClaimedBy == nil {
		msg = msg.AddActionRow(discord.NewSecondaryButton("Claim", claimButtonID).WithEmoji(discord.ComponentEmoji{Name: "🙋"}))
	}
	return msg
}

func reminderLog(w store.WaitingTicket, waited time.Duration) discord.MessageCreate {
	t := store.Ticket{TypeName: w.TypeName, Number: w.Number}
	embed := discord.NewEmbed().
		WithTitle(ticketTitle(t, "waiting "+humanDuration(waited))).
		WithColor(colorAccent).
		AddField("Opened by", discord.UserMention(w.OpenerID), true).
		AddField("Ticket", discord.ChannelMention(w.ChannelID), true)
	if w.ClaimedBy != nil {
		embed = embed.AddField("Claimed by", discord.UserMention(*w.ClaimedBy), true)
	} else {
		embed = embed.AddField("Claimed by", "Nobody yet", true)
	}
	return discord.NewMessageCreate().WithEmbeds(embed.WithTimestamp(time.Now()))
}
