package ticketbot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// MaxHoldReason keeps a hold reason to one line.
const MaxHoldReason = 200

// holdTicket puts an open ticket on hold for a staff member and returns the
// message to post in the ticket. m is nil for dashboard users, who have
// already been checked.
func (b *Bot) holdTicket(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, byID snowflake.ID, reason string) (store.Ticket, discord.MessageCreate, error) {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return t, discord.MessageCreate{}, err
	}
	if m != nil && !isStaff(m, tt) {
		return t, discord.MessageCreate{}, userErr("Only support staff can put a ticket on hold.")
	}
	reason = strings.TrimSpace(reason)
	if r := []rune(reason); len(r) > MaxHoldReason {
		return t, discord.MessageCreate{}, userErr("Keep the reason to %d characters or fewer.", MaxHoldReason)
	}
	ok, err := b.store.HoldTicket(ctx, t.ID, reason)
	if err != nil {
		return t, discord.MessageCreate{}, err
	}
	if !ok {
		return t, discord.MessageCreate{}, userErr("This ticket is already on hold. Use `/ticket resume` to take it off.")
	}
	t.OnHold, t.HoldReason = true, reason
	go b.logEvent(t.GuildID, holdLog(t, byID, true))
	return t, heldMessage(byID, reason), nil
}

// resumeTicket takes a ticket off hold.
func (b *Bot) resumeTicket(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, byID snowflake.ID) (store.Ticket, discord.MessageCreate, error) {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return t, discord.MessageCreate{}, err
	}
	if m != nil && !isStaff(m, tt) {
		return t, discord.MessageCreate{}, userErr("Only support staff can resume a ticket.")
	}
	ok, err := b.store.ResumeTicket(ctx, t.ID, time.Now())
	if err != nil {
		return t, discord.MessageCreate{}, err
	}
	if !ok {
		return t, discord.MessageCreate{}, userErr("This ticket isn't on hold.")
	}
	t.OnHold, t.HoldReason = false, ""
	go b.logEvent(t.GuildID, holdLog(t, byID, false))
	return t, public("▶️ " + discord.UserMention(byID) + " resumed this ticket."), nil
}

func heldMessage(byID snowflake.ID, reason string) discord.MessageCreate {
	desc := discord.UserMention(byID) + " put this ticket on hold."
	if reason != "" {
		desc += "\n**Waiting on:** " + reason
	}
	desc += "\n\nIt's out of the team's queue and won't close for inactivity. A message from the member, or `/ticket resume`, picks it back up."
	return discord.NewMessageCreate().
		WithEmbeds(discord.NewEmbed().WithTitle("On hold").WithDescription(desc).WithColor(colorMuted)).
		WithAllowedMentions(&discord.AllowedMentions{})
}

func holdLog(t store.Ticket, by snowflake.ID, held bool) discord.MessageCreate {
	action := "resumed"
	if held {
		action = "put on hold"
	}
	embed := discord.NewEmbed().
		WithTitle(ticketTitle(t, action)).
		WithColor(colorMuted).
		AddField("By", discord.UserMention(by), true).
		AddField("Ticket", discord.ChannelMention(t.ChannelID), true).
		WithTimestamp(time.Now())
	if held && t.HoldReason != "" {
		embed = embed.AddField("Waiting on", t.HoldReason, false)
	}
	return discord.NewMessageCreate().WithEmbeds(embed)
}

// --- /ticket hold, /ticket resume ---

func (b *Bot) handleHoldCommand(e *handler.CommandEvent) error {
	reason, _ := e.SlashCommandInteractionData().OptString("reason")
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	_, msg, err := b.holdTicket(ctx, e.Channel().ID(), e.Member(), e.User().ID, reason)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.CreateMessage(msg)
}

func (b *Bot) handleResumeCommand(e *handler.CommandEvent) error {
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	_, msg, err := b.resumeTicket(ctx, e.Channel().ID(), e.Member(), e.User().ID)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.CreateMessage(msg)
}

// --- Dashboard ---

// Hold puts a ticket on hold for a dashboard user and posts the notice in
// the ticket.
func (d *Dashboard) Hold(ctx context.Context, t store.Ticket, byID snowflake.ID, reason string) (store.Ticket, error) {
	t, msg, err := d.b.holdTicket(ctx, t.ChannelID, nil, byID, reason)
	if err != nil {
		return t, err
	}
	d.post(ctx, t, msg)
	return t, nil
}

// Resume takes a ticket off hold for a dashboard user.
func (d *Dashboard) Resume(ctx context.Context, t store.Ticket, byID snowflake.ID) (store.Ticket, error) {
	t, msg, err := d.b.resumeTicket(ctx, t.ChannelID, nil, byID)
	if err != nil {
		return t, err
	}
	d.post(ctx, t, msg)
	return t, nil
}

func (d *Dashboard) post(ctx context.Context, t store.Ticket, msg discord.MessageCreate) {
	if _, err := d.b.rest.CreateMessage(t.ChannelID, msg, rest.WithCtx(ctx)); err != nil {
		d.b.log.Warn("failed to post hold message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
}
