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
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// --- /ticket open ---

// handleOpenCommand opens a ticket without the ticket buttons, for members
// who can't find them. Staff can open one for a member with "for".
func (b *Bot) handleOpenCommand(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	guildID, m := e.GuildID(), e.Member()
	if guildID == nil || m == nil {
		return e.CreateMessage(ephemeral("Tickets can only be opened in a server."))
	}
	if target, ok := data.OptUser("for"); ok && target.ID != m.User.ID {
		var targetMember *discord.ResolvedMember
		if rm, ok := data.OptMember("for"); ok {
			targetMember = &rm
		}
		return b.openForMember(e, *guildID, m, target, targetMember, data.String("type"))
	}

	// A form has to be the first response, within Discord's 3 seconds.
	ctx, cancel := context.WithTimeout(e.Ctx, formTimeout)
	tt, err := b.typeToOpen(ctx, *guildID, m, data.String("type"))
	var modal *discord.ModalCreate
	if err == nil {
		modal, err = b.formFor(ctx, guildID, m, tt.ID, formFromCommand)
	}
	cancel()
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	if modal != nil {
		return e.Modal(*modal)
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	content := b.openForUser(e.Ctx, guildID, m, tt.ID, nil)
	_, err = e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent(content))
	return err
}

// openForMember opens a ticket for a member at a staff member's request. The
// form is skipped, since the member isn't there to fill it in.
func (b *Bot) openForMember(e *handler.CommandEvent, guildID snowflake.ID, by *discord.ResolvedMember,
	target discord.User, targetMember *discord.ResolvedMember, value string) error {
	// Opening a ticket takes several requests to Discord, so defer first.
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(e.Ctx, 30*time.Second)
	defer cancel()
	content, err := b.openFor(ctx, guildID, by, target, targetMember, value)
	if err != nil {
		content = b.describe(err)
	}
	_, err = e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent(content))
	return err
}

// openFor opens a ticket of the type named by value for target and returns
// what to tell the staff member. targetMember is nil if target isn't in the
// server.
func (b *Bot) openFor(ctx context.Context, guildID snowflake.ID, by *discord.ResolvedMember,
	target discord.User, targetMember *discord.ResolvedMember, value string) (string, error) {
	types, err := b.store.ListTicketTypes(ctx, guildID)
	if err != nil {
		return "", err
	}
	tt, ok := typeByValue(types, value)
	switch {
	case !ok:
		return "", userErr("There's no ticket type called %q. Pick one from the list.", value)
	case !isStaff(by, &tt):
		return "", userErr("Only staff who handle %s tickets can open one for someone else.", tt.Name)
	case target.Bot:
		return "", userErr("Bots can't have tickets.")
	case targetMember == nil:
		return "", userErr("%s isn't in this server.", discord.UserMention(target.ID))
	}
	channelID, err := b.openTicket(ctx, guildID, target, targetMember.RoleIDs, tt.ID, nil, &by.User)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Opened a %s ticket for %s: %s", tt.Name, discord.UserMention(target.ID), discord.ChannelMention(channelID)), nil
}

// typeToOpen finds the ticket type a member asked for with /ticket open.
// Members can only open types whose ticket buttons they can see, as with the
// buttons themselves; other types are reported as not existing, so hidden
// ones stay hidden. Their roles are checked as the ticket opens.
func (b *Bot) typeToOpen(ctx context.Context, guildID snowflake.ID, m *discord.ResolvedMember, value string) (store.TicketType, error) {
	types, err := b.store.ListTicketTypes(ctx, guildID)
	if err != nil {
		return store.TicketType{}, err
	}
	visible, err := b.visibleTypeIDs(ctx, guildID, m)
	if err != nil {
		return store.TicketType{}, err
	}
	if len(types) == 0 || visible != nil && len(visible) == 0 {
		return store.TicketType{}, userErr("There aren't any tickets you can open here yet.")
	}
	tt, ok := typeByValue(types, value)
	if !ok || visible != nil && !visible[tt.ID] {
		return store.TicketType{}, userErr("There's no ticket type called %q. Pick one from the list.", value)
	}
	return tt, nil
}

// visibleTypeIDs returns the ticket types on ticket buttons the member can
// see, from the gateway cache, or nil for server managers, who can open any.
func (b *Bot) visibleTypeIDs(ctx context.Context, guildID snowflake.ID, m *discord.ResolvedMember) (map[int64]bool, error) {
	if isStaff(m, nil) {
		return nil, nil
	}
	panels, err := b.store.ListPanels(ctx, guildID)
	if err != nil {
		return nil, err
	}
	roles := slices.Collect(b.client.Caches.Roles(guildID))
	return visibleTypes(panels, func(channelID snowflake.ID) bool {
		ch, ok := b.client.Caches.Channel(channelID)
		return ok && discordx.Permissions(guildID, m.User.ID, m.RoleIDs, roles, ch.PermissionOverwrites()).
			Has(discord.PermissionViewChannel)
	}), nil
}

// visibleTypes returns the ticket types on published ticket buttons in the
// channels canView allows.
func visibleTypes(panels []store.Panel, canView func(channelID snowflake.ID) bool) map[int64]bool {
	out := map[int64]bool{}
	checked := map[snowflake.ID]bool{}
	for _, p := range panels {
		if p.ChannelID == nil || p.MessageID == nil {
			continue
		}
		ok, seen := checked[*p.ChannelID]
		if !seen {
			ok = canView(*p.ChannelID)
			checked[*p.ChannelID] = ok
		}
		if ok {
			for _, id := range p.TicketTypeIDs {
				out[id] = true
			}
		}
	}
	return out
}

func (b *Bot) handleOpenAutocomplete(e *handler.AutocompleteEvent) error {
	choices := []discord.AutocompleteChoice{}
	if g, m := e.GuildID(), e.Member(); g != nil && m != nil {
		ctx, cancel := context.WithTimeout(e.Ctx, 2*time.Second)
		defer cancel()
		_, forSomeone := e.Data.OptSnowflake("for")
		types, err := b.store.ListTicketTypes(ctx, *g)
		var visible map[int64]bool
		if err == nil && !forSomeone {
			visible, err = b.visibleTypeIDs(ctx, *g, m)
		}
		if err != nil {
			b.log.Warn("failed to list ticket types to open", slog.Any("err", err))
		} else {
			choices = typeChoices(openable(types, m, forSomeone, visible), e.Data.String("type"))
		}
	}
	return e.AutocompleteResult(choices)
}

// openable filters types down to those a member could open with /ticket
// open: set up, on ticket buttons they can see (when visible isn't nil) and
// not ruled out by their roles. Staff opening a ticket for someone else
// (forSomeone) get every set-up type they handle instead.
func openable(types []store.TicketType, m *discord.ResolvedMember, forSomeone bool, visible map[int64]bool) []store.TicketType {
	roles := memberRoles(m)
	out := []store.TicketType{}
	for _, tt := range types {
		switch {
		case tt.Mode == store.ModeThread && tt.ParentID == nil:
			continue // not set up yet
		case forSomeone:
			if !isStaff(m, &tt) {
				continue
			}
		case visible != nil && !visible[tt.ID],
			hasAny(roles, tt.BlockedRoleIDs),
			len(tt.RequiredRoleIDs) > 0 && !hasAny(roles, tt.RequiredRoleIDs):
			continue
		}
		out = append(out, tt)
	}
	return out
}

// typeChoices lists the types whose name matches what's been typed.
func typeChoices(types []store.TicketType, typed string) []discord.AutocompleteChoice {
	typed = strings.ToLower(strings.TrimSpace(typed))
	choices := []discord.AutocompleteChoice{}
	for _, tt := range types {
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

// typeByValue picks the ticket type with value as its ID, as autocomplete
// sends it, or as its name typed out.
func typeByValue(types []store.TicketType, value string) (store.TicketType, bool) {
	i := slices.IndexFunc(types, func(tt store.TicketType) bool {
		return strconv.FormatInt(tt.ID, 10) == value || strings.EqualFold(tt.Name, strings.TrimSpace(value))
	})
	if i < 0 {
		return store.TicketType{}, false
	}
	return types[i], true
}
