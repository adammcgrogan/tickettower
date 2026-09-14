package ticketbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

const (
	closeRequestAcceptID = "/ticket-btn/close-request/accept"
	closeRequestKeepID   = "/ticket-btn/close-request/keep"
)

// closeAfterChoices are how long a close request waits for the member
// before closing the ticket anyway, in hours; 0 waits for ever.
var closeAfterChoices = []discord.ApplicationCommandOptionChoiceInt{
	{Name: "1 hour", Value: 1},
	{Name: "6 hours", Value: 6},
	{Name: "12 hours", Value: 12},
	{Name: "1 day (default)", Value: 24},
	{Name: "3 days", Value: 72},
	{Name: "Only when they confirm", Value: 0},
}

const defaultCloseAfterHours = 24

// handleCloseRequestCommand asks the member to confirm their ticket can be
// closed, with buttons only they can press. If they don't answer in time
// the ticket closes on the staff member's behalf.
func (b *Bot) handleCloseRequestCommand(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	reason, _ := data.OptString("reason")
	hours := defaultCloseAfterHours
	if h, ok := data.OptInt("close_after"); ok {
		hours = h
	}
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	msg, err := b.requestClose(ctx, e.Channel().ID(), e.Member(), reason, hours)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.CreateMessage(msg)
}

// requestClose records the request and returns the message to post in the
// ticket.
func (b *Bot) requestClose(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, reason string, hours int) (discord.MessageCreate, error) {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return discord.MessageCreate{}, err
	}
	if !isStaff(m, tt) {
		return discord.MessageCreate{}, userErr("Only support staff can ask to close a ticket. If it's your ticket, use `/close`.")
	}
	reason = strings.TrimSpace(reason)
	if r := []rune(reason); len(r) > 500 {
		return discord.MessageCreate{}, userErr("Keep the reason to 500 characters or fewer.")
	}
	req := store.CloseRequest{TicketID: t.ID, By: m.User.ID, ByName: m.User.EffectiveName(), Reason: reason}
	if hours > 0 {
		at := time.Now().Add(time.Duration(hours) * time.Hour)
		req.ClosesAt = &at
	}
	ok, err := b.store.RequestClose(ctx, req)
	if err != nil {
		return discord.MessageCreate{}, err
	}
	if !ok {
		return discord.MessageCreate{}, userErr("This ticket is already closed.")
	}
	return closeRequestMessage(t, req), nil
}

func closeRequestMessage(t store.Ticket, r store.CloseRequest) discord.MessageCreate {
	desc := fmt.Sprintf("%s thinks this ticket is resolved.", discord.UserMention(r.By))
	if r.Reason != "" {
		desc += "\n**Reason:** " + r.Reason
	}
	desc += "\n\n" + discord.UserMention(t.OpenerID) + ", is everything sorted? Close the ticket below, or keep it open if you still need help."
	if r.ClosesAt != nil {
		desc += fmt.Sprintf(" If you don't answer, it closes <t:%d:R>.", r.ClosesAt.Unix())
	}
	return discord.NewMessageCreate().
		WithContent(discord.UserMention(t.OpenerID)).
		WithEmbeds(discord.NewEmbed().WithTitle("Can we close this ticket?").WithDescription(desc).WithColor(colorAccent)).
		AddActionRow(
			discord.NewSuccessButton("Close ticket", closeRequestAcceptID).WithEmoji(discord.ComponentEmoji{Name: "✅"}),
			discord.NewSecondaryButton("Keep open", closeRequestKeepID).WithEmoji(discord.ComponentEmoji{Name: "👋"}),
		).
		WithAllowedMentions(&discord.AllowedMentions{Users: []snowflake.ID{t.OpenerID}})
}

// handleCloseRequestAccept answers the request's "Close ticket" button: the
// opener closes their ticket with the staff member's reason.
func (b *Bot) handleCloseRequestAccept(e *handler.ComponentEvent) error {
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	t, err := b.closeRequestTicket(ctx, e)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	req, err := b.store.GetCloseRequest(ctx, t.ID)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	t, msg, err := b.closeTicket(ctx, t.ChannelID, e.Member(), req.Reason)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	if err := e.UpdateMessage(settledCloseRequest("Ticket closed", discord.UserMention(e.User().ID)+" confirmed the ticket is resolved.")); err != nil {
		return err
	}
	if _, err := b.rest.CreateMessage(t.ChannelID, msg, rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to post close message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	go b.finishClose(t)
	return nil
}

// handleCloseRequestKeep answers the request's "Keep open" button.
func (b *Bot) handleCloseRequestKeep(e *handler.ComponentEvent) error {
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	t, err := b.closeRequestTicket(ctx, e)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	if _, err := b.store.ClearCloseRequest(ctx, t.ID); err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	if err := b.store.RecordActivity(ctx, t.ID, time.Now(), true); err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.UpdateMessage(settledCloseRequest("Ticket kept open", discord.UserMention(e.User().ID)+" still needs help, so the ticket stays open."))
}

// closeRequestTicket loads the ticket behind a close request button and
// checks the person pressing it is the opener.
func (b *Bot) closeRequestTicket(ctx context.Context, e *handler.ComponentEvent) (store.Ticket, error) {
	t, _, err := b.loadTicket(ctx, e.Channel().ID())
	if err != nil {
		return t, err
	}
	if e.User().ID != t.OpenerID {
		return t, userErr("Only %s can answer this. Use `/close` to close the ticket yourself.", discord.UserMention(t.OpenerID))
	}
	return t, nil
}

func settledCloseRequest(title, desc string) discord.MessageUpdate {
	return discord.NewMessageUpdate().
		ClearContent().
		WithEmbeds(discord.NewEmbed().WithTitle(title).WithDescription(desc).WithColor(colorMuted)).
		ClearComponents()
}

// closeUnansweredRequests closes tickets whose member never answered a
// timed close request, on behalf of the staff member who asked.
func (b *Bot) closeUnansweredRequests(ctx context.Context, now time.Time) {
	due, err := b.store.CloseRequestsDue(ctx, now)
	if err != nil {
		if ctx.Err() == nil {
			b.log.Error("failed to find close requests due", slog.Any("err", err))
		}
		return
	}
	for _, r := range due {
		ok, err := b.store.CloseUnansweredRequest(ctx, r.TicketID, now)
		if err != nil {
			b.log.Error("failed to close unanswered request", slog.Int64("ticket_id", r.TicketID), slog.Any("err", err))
			continue
		}
		if !ok {
			continue // the member answered just in time
		}
		t, err := b.store.GetTicket(ctx, r.TicketID)
		if err != nil {
			b.log.Error("failed to load closed ticket", slog.Int64("ticket_id", r.TicketID), slog.Any("err", err))
			continue
		}
		msg := closedMessage(t, b.typeOf(ctx, t))
		msg.Embeds[0].Description = "No answer to the close request, so " + lowerFirst(msg.Embeds[0].Description)
		if _, err := b.rest.CreateMessage(t.ChannelID, msg, rest.WithCtx(ctx)); err != nil {
			b.log.Warn("failed to post close message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
		}
		go b.finishClose(t)
		b.log.Info("ticket closed after an unanswered close request", slog.String("guild_id", t.GuildID.String()), slog.Int("number", t.Number))
	}
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
