package ticketbot

import (
	"context"
	"errors"
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

// UserMessage returns the text of an error meant to be shown to people, if
// err is one.
func UserMessage(err error) (string, bool) {
	var ue *userError
	if errors.As(err, &ue) {
		return ue.msg, true
	}
	return "", false
}

var modeNames = map[store.TicketMode]string{
	store.ModeChannel: "private channels",
	store.ModeThread:  "private threads",
}

// checkMove reports why a ticket can't move to a ticket type, if it can't.
// Moving between channels and threads isn't supported.
func checkMove(t store.Ticket, to store.TicketType) error {
	switch {
	case t.Status != store.StatusOpen:
		return userErr("This ticket is already closed.")
	case t.TicketTypeID != nil && *t.TicketTypeID == to.ID:
		return userErr("This is already a %s ticket.", to.Name)
	case t.Mode != to.Mode:
		return userErr("%s tickets open as %s, so this ticket can't move there. Tickets can only move between types that open the same way.",
			to.Name, modeNames[to.Mode])
	}
	return nil
}

// roleChanges returns the support roles a moved ticket gains and loses.
func roleChanges(from, to []snowflake.ID) (added, removed []snowflake.ID) {
	for _, id := range to {
		if !slices.Contains(from, id) {
			added = append(added, id)
		}
	}
	for _, id := range from {
		if !slices.Contains(to, id) {
			removed = append(removed, id)
		}
	}
	return added, removed
}

// moveTicket moves an open ticket to another ticket type. A channel moves to
// the new type's category and its support roles are swapped; in a thread,
// mentioning the new roles in the note adds them. from is nil if the
// ticket's type has been deleted.
func (b *Bot) moveTicket(ctx context.Context, t store.Ticket, from *store.TicketType, to store.TicketType, by snowflake.ID) (store.Ticket, error) {
	if err := checkMove(t, to); err != nil {
		return t, err
	}
	var fromRoles []snowflake.ID
	if from != nil {
		fromRoles = from.SupportRoleIDs
	}
	added, removed := roleChanges(fromRoles, to.SupportRoleIDs)

	if t.Mode == store.ModeChannel {
		// The new team gets access before the old one loses it, so a failure
		// part way through never leaves the ticket without a team.
		allow := ticketMemberPerms
		for _, id := range added {
			err := b.rest.UpdatePermissionOverwrite(t.ChannelID, id,
				discord.RolePermissionOverwriteUpdate{Allow: &allow}, rest.WithCtx(ctx))
			if err != nil {
				return t, err
			}
		}
		if to.ParentID != nil {
			// Changing the category keeps the channel's own permissions.
			_, err := b.rest.UpdateChannel(t.ChannelID, discord.GuildTextChannelUpdate{ParentID: to.ParentID}, rest.WithCtx(ctx))
			if err != nil {
				return t, err
			}
		}
		for _, id := range removed {
			if err := b.rest.DeletePermissionOverwrite(t.ChannelID, id, rest.WithCtx(ctx)); err != nil {
				b.log.Warn("failed to remove old support role from moved ticket",
					slog.Int64("ticket_id", t.ID), slog.String("role_id", id.String()), slog.Any("err", err))
			}
		}
	}

	ok, err := b.store.MoveTicket(ctx, t.GuildID, t.ID, to.ID, to.Name)
	if err != nil {
		return t, err
	}
	if !ok {
		return t, userErr("This ticket is already closed.")
	}
	fromName := t.TypeName
	t.TicketTypeID, t.TypeName = &to.ID, to.Name
	// A claimed ticket keeps its lock, now against the new type's team.
	if t.ClaimedBy != nil {
		b.applyClaimLock(ctx, t, &to, *t.ClaimedBy)
	}

	if _, err := b.rest.CreateMessage(t.ChannelID, movedMessage(fromName, to.Name, by, added), rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to post move message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	go b.logEvent(t.GuildID, movedLog(t, fromName, by))
	b.log.Info("ticket moved", slog.Int64("ticket_id", t.ID), slog.Int64("type_id", to.ID))
	return t, nil
}

// movedMessage is posted in a ticket when it moves to another type. It pings
// the support roles new to the ticket, which also adds them to a private
// thread.
func movedMessage(fromName, toName string, by snowflake.ID, added []snowflake.ID) discord.MessageCreate {
	content := fmt.Sprintf("🔀 %s moved this ticket from **%s** to **%s**.", discord.UserMention(by), fromName, toName)
	if len(added) > 0 {
		mentions := make([]string, len(added))
		for i, id := range added {
			mentions[i] = discord.RoleMention(id)
		}
		content += " " + strings.Join(mentions, " ") + " can help from here."
	}
	return discord.NewMessageCreate().
		WithContent(content).
		WithAllowedMentions(&discord.AllowedMentions{Roles: added})
}

func movedLog(t store.Ticket, fromName string, by snowflake.ID) discord.MessageCreate {
	embed := discord.NewEmbed().
		WithTitle(ticketTitle(t, "moved")).
		WithColor(colorAccent).
		AddField("From", fromName, true).
		AddField("Moved by", discord.UserMention(by), true).
		AddField("Ticket", discord.ChannelMention(t.ChannelID), true).
		WithTimestamp(time.Now())
	return discord.NewMessageCreate().WithEmbeds(embed)
}

// --- /ticket move ---

func (b *Bot) handleMoveCommand(e *handler.CommandEvent) error {
	// Moving a channel takes several requests to Discord, so defer first.
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(e.Ctx, 15*time.Second)
	defer cancel()
	var content string
	if name, err := b.moveFromDiscord(ctx, e.Channel().ID(), e.Member(), e.SlashCommandInteractionData().String("type")); err != nil {
		content = b.describe(err)
	} else {
		content = "Moved to " + name + "."
	}
	_, err := e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent(content))
	return err
}

// moveFromDiscord moves the ticket in a channel for a staff member. value is
// a ticket type ID picked from autocomplete, or a type's name typed out.
func (b *Bot) moveFromDiscord(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, value string) (string, error) {
	t, from, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return "", err
	}
	if !isStaff(m, from) {
		return "", userErr("Only support staff can move tickets.")
	}
	types, err := b.store.ListTicketTypes(ctx, t.GuildID)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(types, func(tt store.TicketType) bool {
		return strconv.FormatInt(tt.ID, 10) == value || strings.EqualFold(tt.Name, strings.TrimSpace(value))
	})
	if i < 0 {
		return "", userErr("There's no ticket type called %q. Pick one from the list.", value)
	}
	if _, err := b.moveTicket(ctx, t, from, types[i], m.User.ID); err != nil {
		return "", err
	}
	return types[i].Name, nil
}

func (b *Bot) handleMoveAutocomplete(e *handler.AutocompleteEvent) error {
	choices := []discord.AutocompleteChoice{}
	if g := e.GuildID(); g != nil {
		ctx, cancel := context.WithTimeout(e.Ctx, 2*time.Second)
		defer cancel()
		types, err := b.store.ListTicketTypes(ctx, *g)
		if err != nil {
			b.log.Warn("failed to list ticket types", slog.Any("err", err))
		}
		t, err := b.store.GetTicketByChannel(ctx, e.Channel().ID())
		choices = moveChoices(types, t, err == nil, e.Data.String("type"))
	}
	return e.AutocompleteResult(choices)
}

// moveChoices lists the ticket types a ticket can move to whose name matches
// what's been typed. Outside a ticket (inTicket false) every type matches.
func moveChoices(types []store.TicketType, t store.Ticket, inTicket bool, typed string) []discord.AutocompleteChoice {
	typed = strings.ToLower(strings.TrimSpace(typed))
	choices := []discord.AutocompleteChoice{}
	for _, tt := range types {
		if inTicket && checkMove(t, tt) != nil {
			continue
		}
		if !strings.Contains(strings.ToLower(tt.Name), typed) {
			continue
		}
		choices = append(choices, discord.AutocompleteChoiceString{Name: tt.Name, Value: strconv.FormatInt(tt.ID, 10)})
		if len(choices) == 25 { // Discord's limit
			break
		}
	}
	return choices
}
