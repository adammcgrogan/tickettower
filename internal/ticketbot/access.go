package ticketbot

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// openerState is everything about a member that decides whether they can
// open a ticket of a type right now.
type openerState struct {
	roles      []snowflake.ID
	block      *store.Block
	open       []snowflake.ID // channels of their open tickets of the type
	lastClosed *time.Time     // when their last ticket of the type closed
}

// loadOpener gathers a member's state for accessErr. It runs before a form is
// shown, inside Discord's 3 second window, so every query is a cheap indexed
// lookup.
func (b *Bot) loadOpener(ctx context.Context, guildID snowflake.ID, userID snowflake.ID, roles []snowflake.ID, tt store.TicketType) (openerState, error) {
	st := openerState{roles: roles}
	block, err := b.store.GetBlock(ctx, guildID, userID)
	if err == nil {
		st.block = &block
	} else if !errors.Is(err, store.ErrNotFound) {
		return st, err
	}
	if st.open, err = b.store.OpenTicketChannels(ctx, guildID, tt.ID, userID); err != nil {
		return st, err
	}
	if tt.CooldownMinutes > 0 {
		if st.lastClosed, err = b.store.LastClosedAt(ctx, guildID, tt.ID, userID); err != nil {
			return st, err
		}
	}
	return st, nil
}

// accessErr reports why a member can't open a ticket of a type, or nil if
// they can. Blocks come first, so a blocked member never learns anything
// else about the type.
func accessErr(tt store.TicketType, st openerState, now time.Time) error {
	if st.block != nil {
		msg := "You can't open tickets in this server."
		if st.block.ExpiresAt != nil {
			msg += fmt.Sprintf(" You can again %s.", discord.FormattedTimestampMention(st.block.ExpiresAt.Unix(), discord.TimestampStyleRelative))
		}
		if st.block.Reason != "" {
			msg += "\n**Reason:** " + st.block.Reason
		}
		return userErr("%s", msg)
	}
	if hasAny(st.roles, tt.BlockedRoleIDs) {
		return userErr("You can't open %s tickets right now.", tt.Name)
	}
	if len(tt.RequiredRoleIDs) > 0 && !hasAny(st.roles, tt.RequiredRoleIDs) {
		return userErr("You need %s to open a %s ticket.", roleList(tt.RequiredRoleIDs), tt.Name)
	}
	if len(st.open) >= tt.MaxOpenPerUser {
		return userErr("You already have an open %s ticket: %s", tt.Name, discord.ChannelMention(st.open[len(st.open)-1]))
	}
	if tt.CooldownMinutes > 0 && st.lastClosed != nil {
		until := st.lastClosed.Add(time.Duration(tt.CooldownMinutes) * time.Minute)
		if until.After(now) {
			return userErr("Your last %s ticket closed recently. You can open another %s.",
				tt.Name, discord.FormattedTimestampMention(until.Unix(), discord.TimestampStyleRelative))
		}
	}
	return nil
}

// onBehalfErr reports why staff can't open a ticket of a type for a member,
// or nil if they can. The type's rules on who can open it are for members
// choosing for themselves, so staff aren't held to them, but the member's
// open ticket limit still applies: the ticket staff want is usually open.
func onBehalfErr(tt store.TicketType, st openerState, memberID snowflake.ID) error {
	if len(st.open) >= tt.MaxOpenPerUser {
		return userErr("%s already has an open %s ticket: %s",
			discord.UserMention(memberID), tt.Name, discord.ChannelMention(st.open[len(st.open)-1]))
	}
	return nil
}

func hasAny(have, want []snowflake.ID) bool {
	return slices.ContainsFunc(have, func(id snowflake.ID) bool { return slices.Contains(want, id) })
}

// roleList renders role mentions as "the @A role" or "the @A or @B role".
// Mentions in ephemeral replies are shown by name without pinging anyone.
func roleList(ids []snowflake.ID) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = discord.RoleMention(id)
	}
	if len(names) == 1 {
		return "the " + names[0] + " role"
	}
	return "the " + strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1] + " role"
}

// memberRoles returns a member's role IDs, or nil outside a server.
func memberRoles(m *discord.ResolvedMember) []snowflake.ID {
	if m == nil {
		return nil
	}
	return m.RoleIDs
}

// isSupport reports whether a member is on any ticket type's support team,
// or manages the server. It's for commands that aren't tied to one ticket.
func (b *Bot) isSupport(ctx context.Context, guildID snowflake.ID, m *discord.ResolvedMember) (bool, error) {
	if isStaff(m, nil) {
		return true, nil
	}
	types, err := b.store.ListTicketTypes(ctx, guildID)
	if err != nil {
		return false, err
	}
	for i := range types {
		if isStaff(m, &types[i]) {
			return true, nil
		}
	}
	return false, nil
}

// blockMember blocks target from opening tickets and returns the message to
// show the staff member. duration is how long the block lasts, or 0 to block
// until someone unblocks them.
func (b *Bot) blockMember(ctx context.Context, guildID snowflake.ID, by *discord.ResolvedMember, target discord.User, targetMember *discord.ResolvedMember, reason string, duration time.Duration) (string, error) {
	ok, err := b.isSupport(ctx, guildID, by)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", userErr("Only support staff can block members.")
	}
	if target.Bot {
		return "", userErr("Bots can't open tickets anyway.")
	}
	if target.ID == by.User.ID {
		return "", userErr("You can't block yourself.")
	}
	if isStaff(targetMember, nil) {
		return "", userErr("%s manages this server, so they can't be blocked.", target.EffectiveName())
	}
	reason = strings.TrimSpace(reason)
	if r := []rune(reason); len(r) > store.MaxBlockReason {
		return "", userErr("Keep the reason to %d characters or fewer.", store.MaxBlockReason)
	}
	name := target.EffectiveName()
	if targetMember != nil {
		name = targetMember.EffectiveName()
	}
	block := store.Block{
		GuildID: guildID, UserID: target.ID, UserName: name, Reason: reason,
		BlockedBy: by.User.ID, BlockedByName: by.EffectiveName(),
	}
	if duration > 0 {
		until := time.Now().Add(duration)
		block.ExpiresAt = &until
	}
	if err := b.store.BlockMember(ctx, &block); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("%s can no longer open tickets.", discord.UserMention(target.ID))
	if block.ExpiresAt != nil {
		msg += fmt.Sprintf(" This lifts %s.", discord.FormattedTimestampMention(block.ExpiresAt.Unix(), discord.TimestampStyleRelative))
	}
	if reason != "" {
		msg += " They'll see the reason if they try: " + reason
	}
	return msg, nil
}

// unblockMember lifts a block and returns the message to show the staff
// member.
func (b *Bot) unblockMember(ctx context.Context, guildID snowflake.ID, by *discord.ResolvedMember, target discord.User) (string, error) {
	ok, err := b.isSupport(ctx, guildID, by)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", userErr("Only support staff can unblock members.")
	}
	err = b.store.UnblockMember(ctx, guildID, target.ID)
	if errors.Is(err, store.ErrNotFound) {
		return "", userErr("%s isn't blocked.", discord.UserMention(target.ID))
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s can open tickets again.", discord.UserMention(target.ID)), nil
}

// setAvailable opts a support member in or out of a guild's auto-assign
// pool, used by ticket types with auto-assign on. It returns the message to
// show them.
func (b *Bot) setAvailable(ctx context.Context, guildID snowflake.ID, m *discord.ResolvedMember, on bool) (string, error) {
	ok, err := b.isSupport(ctx, guildID, m)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", userErr("Only support staff can opt in to auto-assignment.")
	}
	if err := b.store.SetAvailable(ctx, guildID, m.User.ID, m.EffectiveName(), on); err != nil {
		return "", err
	}
	if !on {
		return "You won't get tickets assigned to you automatically anymore.", nil
	}
	return "You'll now get new tickets assigned to you automatically, sharing them round robin with everyone else who's opted in.", nil
}
