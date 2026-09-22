package ticketbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// checkMerge reports why a ticket can't be merged into another, if it can't.
// Only two open tickets from the same member, neither the other, can merge.
func checkMerge(t, to store.Ticket) error {
	switch {
	case t.Status != store.StatusOpen:
		return userErr("This ticket is already closed.")
	case to.Status != store.StatusOpen:
		return userErr("Ticket #%d is already closed.", to.Number)
	case t.ID == to.ID:
		return userErr("Pick a different ticket to merge into.")
	case t.OpenerID != to.OpenerID:
		return userErr("Tickets can only be merged into another open ticket from the same member.")
	}
	return nil
}

// mergeTicket closes t as merged into to: it's marked closed with a reason
// pointing at to, and a note on each ticket links the other. Posting the
// in-channel messages and cleaning up t's channel is left to the caller,
// since it differs between the bot (which posts itself) and the dashboard.
func (b *Bot) mergeTicket(ctx context.Context, t, to store.Ticket, by snowflake.ID, byName string) (store.Ticket, error) {
	if err := checkMerge(t, to); err != nil {
		return t, err
	}
	reason := fmt.Sprintf("Merged into #%d", to.Number)
	ok, err := b.store.CloseTicket(ctx, t.ID, by, byName, reason)
	if err != nil {
		return t, err
	}
	if !ok {
		return t, userErr("This ticket is already closed.")
	}
	now := time.Now()
	t.Status, t.ClosedBy, t.ClosedByName, t.CloseReason, t.ClosedAt = store.StatusClosed, &by, &byName, reason, &now

	fromNote := fmt.Sprintf("Merged into #%d (%s).%s", to.Number, to.TypeName, transcriptSuffix(b.cfg.PublicURL, to.ID))
	if err := b.store.AddTicketNote(ctx, t.GuildID, &store.TicketNote{TicketID: t.ID, AuthorID: by, AuthorName: byName, Content: fromNote}); err != nil {
		b.log.Warn("failed to add merge note", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	toNote := fmt.Sprintf("Merged from #%d (%s).%s", t.Number, t.TypeName, transcriptSuffix(b.cfg.PublicURL, t.ID))
	if err := b.store.AddTicketNote(ctx, to.GuildID, &store.TicketNote{TicketID: to.ID, AuthorID: by, AuthorName: byName, Content: toNote}); err != nil {
		b.log.Warn("failed to add merge note", slog.Int64("ticket_id", to.ID), slog.Any("err", err))
	}
	return t, nil
}

// transcriptSuffix appends a transcript link, if the dashboard is publicly
// hosted (Discord rejects link buttons to local addresses, and a note isn't
// worth much if the link can't work).
func transcriptSuffix(publicURL string, ticketID int64) string {
	if !strings.HasPrefix(publicURL, "https://") {
		return ""
	}
	return fmt.Sprintf(" Transcript: %s/transcripts/%d", publicURL, ticketID)
}

// mergedMessage is posted in the merged (now closed) ticket's channel.
func mergedMessage(to store.Ticket, by snowflake.ID) discord.MessageCreate {
	return discord.NewMessageCreate().
		WithEmbeds(discord.NewEmbed().
			WithTitle("Ticket merged").
			WithDescription(fmt.Sprintf("%s merged this ticket into #%d. This channel will be deleted in a few seconds.",
				discord.UserMention(by), to.Number)).
			WithColor(colorMuted)).
		WithAllowedMentions(&discord.AllowedMentions{})
}

// mergedArrivalMessage is posted in the surviving ticket's channel.
func mergedArrivalMessage(from store.Ticket, by snowflake.ID) discord.MessageCreate {
	content := fmt.Sprintf("🔀 %s merged ticket #%d (%s) into this one. %s can continue here.",
		discord.UserMention(by), from.Number, from.TypeName, discord.UserMention(from.OpenerID))
	return discord.NewMessageCreate().
		WithContent(content).
		WithAllowedMentions(&discord.AllowedMentions{})
}

func mergedLog(t, to store.Ticket, by snowflake.ID) discord.MessageCreate {
	embed := discord.NewEmbed().
		WithTitle(ticketTitle(t, "merged")).
		WithColor(colorMuted).
		AddField("Opened by", discord.UserMention(t.OpenerID), true).
		AddField("Merged by", discord.UserMention(by), true).
		AddField("Merged into", fmt.Sprintf("#%d", to.Number), true).
		WithTimestamp(time.Now())
	return discord.NewMessageCreate().WithEmbeds(embed)
}

// notifyOpenerMerged tells the member their ticket was merged and where to
// continue, mirroring notifyOpener's closing DM.
func (b *Bot) notifyOpenerMerged(ctx context.Context, t, to store.Ticket) {
	dm, err := b.rest.CreateDMChannel(t.OpenerID, rest.WithCtx(ctx))
	if err != nil {
		return
	}
	desc := fmt.Sprintf("Your ticket in %s has been merged into ticket #%d. Continue the conversation there.",
		b.guildName(ctx, t.GuildID), to.Number)
	embed := discord.NewEmbed().WithTitle("Ticket merged").WithDescription(desc).WithColor(colorMuted)
	msg := discord.NewMessageCreate().WithEmbeds(embed).
		AddActionRow(discord.NewLinkButton("Continue in ticket", fmt.Sprintf("https://discord.com/channels/%d/%d", t.GuildID, to.ChannelID)))
	if _, err := b.rest.CreateMessage(dm.ID(), msg, rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to DM merged ticket opener", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
}

// finishMerge notifies the opener and cleans up the merged ticket's channel,
// as finishClose does for an ordinary close.
func (b *Bot) finishMerge(t, to store.Ticket) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b.tickets.remove(t.ChannelID)
	b.logEvent(t.GuildID, mergedLog(t, to, *t.ClosedBy))
	b.notifyOpenerMerged(ctx, t, to)

	if t.Mode != store.ModeThread {
		time.Sleep(closeDelay)
	}
	b.cleanUpChannel(ctx, t)
}

// --- /ticket merge ---

func (b *Bot) handleMergeCommand(e *handler.CommandEvent) error {
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	content := b.mergeFromDiscord(ctx, e.Channel().ID(), e.Member(), e.SlashCommandInteractionData().String("ticket"))
	_, err := e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent(content))
	return err
}

// mergeFromDiscord merges the ticket in a channel into another of the same
// opener's open tickets for a staff member. value is a ticket ID picked from
// autocomplete.
func (b *Bot) mergeFromDiscord(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, value string) string {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return b.describe(err)
	}
	if !isStaff(m, tt) {
		return b.describe(userErr("Only support staff can merge tickets."))
	}
	toID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return b.describe(userErr("Pick a ticket from the list."))
	}
	to, err := b.store.GetGuildTicket(ctx, t.GuildID, toID)
	if errors.Is(err, store.ErrNotFound) {
		return b.describe(userErr("Couldn't find that ticket. Pick one from the list."))
	} else if err != nil {
		return b.describe(err)
	}

	t, err = b.mergeTicket(ctx, t, to, m.User.ID, m.User.EffectiveName())
	if err != nil {
		return b.describe(err)
	}
	if _, err := b.rest.CreateMessage(t.ChannelID, mergedMessage(to, m.User.ID), rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to post merge message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	if _, err := b.rest.CreateMessage(to.ChannelID, mergedArrivalMessage(t, m.User.ID), rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to post merge arrival message", slog.Int64("ticket_id", to.ID), slog.Any("err", err))
	}
	go b.finishMerge(t, to)
	return fmt.Sprintf("Merged into #%d.", to.Number)
}

func (b *Bot) handleMergeAutocomplete(e *handler.AutocompleteEvent) error {
	choices := []discord.AutocompleteChoice{}
	g := e.GuildID()
	if g == nil {
		return e.AutocompleteResult(choices)
	}
	ctx, cancel := context.WithTimeout(e.Ctx, 2*time.Second)
	defer cancel()
	t, err := b.store.GetTicketByChannel(ctx, e.Channel().ID())
	if err != nil {
		return e.AutocompleteResult(choices)
	}
	others, err := b.store.OpenTicketsByOpener(ctx, t.GuildID, t.OpenerID, t.ID)
	if err != nil {
		b.log.Warn("failed to list merge targets", slog.Any("err", err))
		return e.AutocompleteResult(choices)
	}
	choices = mergeChoices(others, e.Data.String("ticket"))
	return e.AutocompleteResult(choices)
}

// mergeChoices lists a member's other open tickets whose number or type
// matches what's been typed.
func mergeChoices(tickets []store.Ticket, typed string) []discord.AutocompleteChoice {
	typed = strings.ToLower(strings.TrimSpace(typed))
	choices := []discord.AutocompleteChoice{}
	for _, t := range tickets {
		name := fmt.Sprintf("#%d — %s", t.Number, t.TypeName)
		if typed != "" && !strings.Contains(strings.ToLower(name), typed) {
			continue
		}
		choices = append(choices, discord.AutocompleteChoiceString{Name: name, Value: strconv.FormatInt(t.ID, 10)})
		if len(choices) == 25 { // Discord's limit
			break
		}
	}
	return choices
}
