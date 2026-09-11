package ticketbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/adammcgrogan/tickettower/internal/store"
)

const (
	rateButtonPrefix    = "/rate/"         // + {ticketID}/{rating}
	commentButtonPrefix = "/rate-comment/" // + {ticketID}
	commentModalPrefix  = "/rate-modal/"   // + {ticketID}
)

func stars(rating int) string {
	return strings.Repeat("★", rating) + strings.Repeat("☆", 5-rating)
}

// feedbackComponents are the rows on the closing DM: a 1–5 rating scale and,
// when the dashboard is public, a transcript link.
func (b *Bot) feedbackComponents(ticketID int64) []discord.LayoutComponent {
	buttons := make([]discord.InteractiveComponent, 0, 5)
	for i := 1; i <= 5; i++ {
		buttons = append(buttons, discord.NewSecondaryButton(strings.Repeat("★", i),
			fmt.Sprintf("%s%d/%d", rateButtonPrefix, ticketID, i)))
	}
	return append([]discord.LayoutComponent{discord.NewActionRow(buttons...)}, b.transcriptRow(ticketID)...)
}

// transcriptRow links to the ticket's transcript, if the dashboard is
// publicly hosted (Discord rejects link buttons to local addresses).
func (b *Bot) transcriptRow(ticketID int64) []discord.LayoutComponent {
	if !strings.HasPrefix(b.cfg.PublicURL, "https://") {
		return nil
	}
	url := fmt.Sprintf("%s/transcripts/%d", b.cfg.PublicURL, ticketID)
	return []discord.LayoutComponent{discord.NewActionRow(discord.NewLinkButton("View transcript", url))}
}

// feedbackTicket loads the ticket in a feedback custom ID and checks the
// user opened it.
func (b *Bot) feedbackTicket(ctx context.Context, rawID string, userID interface{ String() string }) (store.Ticket, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return store.Ticket{}, userErr("This button is out of date.")
	}
	t, err := b.store.GetTicket(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return t, userErr("This ticket no longer exists.")
	} else if err != nil {
		return t, err
	}
	if t.OpenerID.String() != userID.String() {
		return t, userErr("Only the person who opened this ticket can rate it.")
	}
	return t, nil
}

func (b *Bot) handleRate(e *handler.ComponentEvent) error {
	ctx, cancel := context.WithTimeout(e.Ctx, 10*time.Second)
	defer cancel()
	rating, err := strconv.Atoi(e.Vars["rating"])
	if err != nil || rating < 1 || rating > 5 {
		return e.CreateMessage(ephemeral("This button is out of date."))
	}
	t, err := b.feedbackTicket(ctx, e.Vars["ticketID"], e.User().ID)
	if err == nil {
		err = b.store.SetFeedbackRating(ctx, t.ID, rating)
	}
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}

	rows := append([]discord.LayoutComponent{discord.NewActionRow(
		discord.NewSecondaryButton("Add a comment", fmt.Sprintf("%s%d", commentButtonPrefix, t.ID)).
			WithEmoji(discord.ComponentEmoji{Name: "💬"}),
	)}, b.transcriptRow(t.ID)...)
	return e.UpdateMessage(discord.NewMessageUpdate().
		WithContent("Thanks for your feedback! You rated this ticket " + stars(rating) + ".").
		WithComponents(rows...))
}

func (b *Bot) handleCommentButton(e *handler.ComponentEvent) error {
	ctx, cancel := context.WithTimeout(e.Ctx, 10*time.Second)
	defer cancel()
	t, err := b.feedbackTicket(ctx, e.Vars["ticketID"], e.User().ID)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.Modal(discord.NewModalCreate(fmt.Sprintf("%s%d", commentModalPrefix, t.ID), "Tell us more").
		AddLabel("What went well, or what could be better?", discord.NewParagraphTextInput("comment").
			WithRequired(true).
			WithMaxLength(1000)))
}

func (b *Bot) handleCommentModal(e *handler.ModalEvent) error {
	ctx, cancel := context.WithTimeout(e.Ctx, 10*time.Second)
	defer cancel()
	t, err := b.feedbackTicket(ctx, e.Vars["ticketID"], e.User().ID)
	if err == nil {
		var ok bool
		ok, err = b.store.SetFeedbackComment(ctx, t.ID, strings.TrimSpace(e.Data.Text("comment")))
		if err == nil && !ok {
			err = userErr("Please choose a rating first.")
		}
	}
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.UpdateMessage(discord.NewMessageUpdate().
		WithContent("Thanks for your feedback! Your comment has been passed on to the team.").
		WithComponents(b.transcriptRow(t.ID)...))
}
