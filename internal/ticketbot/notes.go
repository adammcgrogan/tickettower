package ticketbot

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// --- /ticket note ---

func (b *Bot) handleNoteCommand(e *handler.CommandEvent) error {
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	msg, err := b.addNote(ctx, e.Channel().ID(), e.Member(), e.SlashCommandInteractionData().String("text"))
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.CreateMessage(ephemeral(msg))
}

// addNote saves a private note on the ticket in a channel for a staff member
// and returns what to tell them. The reply is ephemeral, so the member never
// sees the note.
func (b *Bot) addNote(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, text string) (string, error) {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return "", err
	}
	if !isStaff(m, tt) {
		return "", userErr("Only support staff can add notes to tickets.")
	}
	if text, err = noteText(text); err != nil {
		return "", err
	}
	note := store.TicketNote{TicketID: t.ID, AuthorID: m.User.ID, AuthorName: m.EffectiveName(), Content: text}
	if err := b.store.AddTicketNote(ctx, t.GuildID, &note); err != nil {
		return "", err
	}
	return "📝 Note saved. Only your team can see it, on the Tickets page and in the transcript.", nil
}

// noteText tidies a staff note, or explains why it can't be saved.
func noteText(text string) (string, error) {
	text = strings.TrimSpace(text)
	switch n := utf8.RuneCountInString(text); {
	case n == 0:
		return "", userErr("Write the note first.")
	case n > store.MaxNoteLength:
		return "", userErr("Keep notes to %d characters or fewer.", store.MaxNoteLength)
	}
	return text, nil
}

// --- Dashboard ---

// AddNote saves a private note on a ticket for a dashboard user. Closed
// tickets can take notes too, for whoever reviews them later.
func (d *Dashboard) AddNote(ctx context.Context, t store.Ticket, byID snowflake.ID, byName, text string) (store.TicketNote, error) {
	text, err := noteText(text)
	if err != nil {
		return store.TicketNote{}, err
	}
	note := store.TicketNote{TicketID: t.ID, AuthorID: byID, AuthorName: byName, Content: text}
	err = d.b.store.AddTicketNote(ctx, t.GuildID, &note)
	return note, err
}
