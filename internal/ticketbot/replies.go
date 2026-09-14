package ticketbot

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// embedDescriptionLimit is Discord's cap on an embed's description.
const embedDescriptionLimit = 4096

// replyText fills in the placeholders in a staff reply, such as a saved
// reply. Mentions inside an embed are shown but don't ping anyone.
func replyText(text string, t store.Ticket, server, staff string) string {
	text = strings.NewReplacer(
		"{user}", discord.UserMention(t.OpenerID),
		"{username}", t.OpenerName,
		"{number}", fmt.Sprintf("%04d", t.Number),
		"{type}", t.TypeName,
		"{server}", server,
		"{staff}", staff,
	).Replace(text)
	return truncate(text, embedDescriptionLimit)
}

// recordReply records a reply the bot posted for a staff member. The bot
// skips its own messages when tracking replies, so this sets the team's first
// response and restarts the auto-close clock. The reply has been posted by
// now, so failures are only logged.
func (b *Bot) recordReply(ctx context.Context, t store.Ticket, byID snowflake.ID, at time.Time) {
	log := b.log.With(slog.Int64("ticket_id", t.ID))
	byOpener := byID == t.OpenerID
	if !byOpener {
		if err := b.store.SetFirstResponse(ctx, t.ID, at); err != nil {
			log.Error("failed to record first response", slog.Any("err", err))
		}
	}
	if err := b.store.RecordActivity(ctx, t.ID, at, byOpener); err != nil {
		log.Error("failed to record ticket activity", slog.Any("err", err))
	}
}

// --- /reply ---

func (b *Bot) handleReplyCommand(e *handler.CommandEvent) error {
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	t, msg, err := b.savedReplyMessage(ctx, e.Channel().ID(), e.Member(), e.SlashCommandInteractionData().String("name"))
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	if err := e.CreateMessage(msg); err != nil {
		return err
	}
	by := e.User().ID
	b.recordReply(ctx, t, by, time.Now())
	// The response is captured as a bot message; note who it was sent for so
	// analytics credit them (the capture may land first, so this is an
	// upsert of that one field).
	if m, err := e.GetInteractionResponse(rest.WithCtx(ctx)); err == nil {
		saved := toTicketMessage(t.ID, *m)
		saved.SentBy = &by
		if err := b.store.InsertTicketMessage(ctx, saved); err != nil {
			b.log.Error("failed to save who sent a saved reply", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
		}
	} else {
		b.log.Warn("failed to read the saved reply message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	return nil
}

// savedReplyMessage builds a staff member's saved reply for the ticket in a
// channel. value is a saved reply's ID picked from autocomplete, or its name
// typed out.
func (b *Bot) savedReplyMessage(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, value string) (store.Ticket, discord.MessageCreate, error) {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return t, discord.MessageCreate{}, err
	}
	if !isStaff(m, tt) {
		return t, discord.MessageCreate{}, userErr("Only support staff can send saved replies.")
	}
	replies, err := b.store.ListSavedReplies(ctx, t.GuildID)
	if err != nil {
		return t, discord.MessageCreate{}, err
	}
	reply, err := findSavedReply(replies, value)
	if err != nil {
		return t, discord.MessageCreate{}, err
	}
	server, serverIcon := b.guildBrand(ctx, t.GuildID)
	staff := m.EffectiveName()
	return t, replyMessage("", staff, m.EffectiveAvatarURL(), server, serverIcon, replyText(reply.Content, t, server, staff)), nil
}

// findSavedReply picks the saved reply with value as its ID or name.
func findSavedReply(replies []store.SavedReply, value string) (store.SavedReply, error) {
	if len(replies) == 0 {
		return store.SavedReply{}, userErr("This server has no saved replies yet. Add them in the dashboard, under Saved replies.")
	}
	i := slices.IndexFunc(replies, func(r store.SavedReply) bool {
		return strconv.FormatInt(r.ID, 10) == value || strings.EqualFold(r.Name, strings.TrimSpace(value))
	})
	if i < 0 {
		return store.SavedReply{}, userErr("There's no saved reply called %q. Pick one from the list.", value)
	}
	return replies[i], nil
}

func (b *Bot) handleReplyAutocomplete(e *handler.AutocompleteEvent) error {
	choices := []discord.AutocompleteChoice{}
	if g := e.GuildID(); g != nil {
		ctx, cancel := context.WithTimeout(e.Ctx, 2*time.Second)
		defer cancel()
		replies, err := b.store.ListSavedReplies(ctx, *g)
		if err != nil {
			b.log.Warn("failed to list saved replies", slog.Any("err", err))
		}
		choices = replyChoices(replies, e.Data.String("name"))
	}
	return e.AutocompleteResult(choices)
}

// replyChoices lists the saved replies whose name matches what's been typed.
func replyChoices(replies []store.SavedReply, typed string) []discord.AutocompleteChoice {
	typed = strings.ToLower(strings.TrimSpace(typed))
	choices := []discord.AutocompleteChoice{}
	for _, r := range replies {
		if !strings.Contains(strings.ToLower(r.Name), typed) {
			continue
		}
		choices = append(choices, discord.AutocompleteChoiceString{Name: r.Name, Value: strconv.FormatInt(r.ID, 10)})
		if len(choices) == 25 { // Discord's limit
			break
		}
	}
	return choices
}
