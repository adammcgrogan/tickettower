package ticketbot

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// lockedRoles are the support roles a claim lock applies to: every support
// role that isn't exempt.
func lockedRoles(tt *store.TicketType) []snowflake.ID {
	if tt == nil || tt.ClaimLock == store.ClaimLockOff || tt.ClaimLock == "" {
		return nil
	}
	var out []snowflake.ID
	for _, id := range tt.SupportRoleIDs {
		if !slices.Contains(tt.ClaimLockExemptRoleIDs, id) {
			out = append(out, id)
		}
	}
	return out
}

// applyClaimLock narrows the rest of the support team's access to a channel
// ticket once it's claimed, as the type asks: read only, or hidden. The
// claimer keeps full access through their own overwrite. Threads can't
// restrict a role, so nothing happens there. Failures are logged: the claim
// itself has already gone through.
func (b *Bot) applyClaimLock(ctx context.Context, t store.Ticket, tt *store.TicketType, claimer snowflake.ID) {
	roles := lockedRoles(tt)
	if t.Mode != store.ModeChannel || len(roles) == 0 {
		return
	}
	allow := ticketMemberPerms
	if err := b.rest.UpdatePermissionOverwrite(t.ChannelID, claimer,
		discord.MemberPermissionOverwriteUpdate{Allow: &allow}, rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to give the claimer access", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
		return
	}
	var roleAllow, roleDeny discord.Permissions
	switch tt.ClaimLock {
	case store.ClaimLockReadOnly:
		roleAllow = ticketMemberPerms &^ discord.PermissionSendMessages
		roleDeny = discord.PermissionSendMessages
	case store.ClaimLockHidden:
		roleDeny = discord.PermissionViewChannel
	}
	for _, id := range roles {
		err := b.rest.UpdatePermissionOverwrite(t.ChannelID, id,
			discord.RolePermissionOverwriteUpdate{Allow: &roleAllow, Deny: &roleDeny}, rest.WithCtx(ctx))
		if err != nil {
			b.log.Warn("failed to apply claim lock", slog.Int64("ticket_id", t.ID), slog.String("role_id", id.String()), slog.Any("err", err))
		}
	}
}

// releaseClaimLock gives the support team their access back when a ticket
// is unclaimed.
func (b *Bot) releaseClaimLock(ctx context.Context, t store.Ticket, tt *store.TicketType, claimer snowflake.ID) {
	roles := lockedRoles(tt)
	if t.Mode != store.ModeChannel || len(roles) == 0 {
		return
	}
	allow, deny := ticketMemberPerms, discord.Permissions(0)
	for _, id := range roles {
		err := b.rest.UpdatePermissionOverwrite(t.ChannelID, id,
			discord.RolePermissionOverwriteUpdate{Allow: &allow, Deny: &deny}, rest.WithCtx(ctx))
		if err != nil {
			b.log.Warn("failed to release claim lock", slog.Int64("ticket_id", t.ID), slog.String("role_id", id.String()), slog.Any("err", err))
		}
	}
	// The claimer's own overwrite only existed for the lock. The opener is
	// never a claimer, so this can't remove the member's access.
	if claimer != t.OpenerID {
		if err := b.rest.DeletePermissionOverwrite(t.ChannelID, claimer, rest.WithCtx(ctx)); err != nil {
			b.log.Warn("failed to remove claimer overwrite", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
		}
	}
}

// claimLockNote explains a claim lock to the team in the claim message.
func claimLockNote(t store.Ticket, tt *store.TicketType, claimer snowflake.ID) string {
	if t.Mode != store.ModeChannel || len(lockedRoles(tt)) == 0 {
		return ""
	}
	switch tt.ClaimLock {
	case store.ClaimLockReadOnly:
		return fmt.Sprintf(" Other staff can read along, but only %s will reply.", discord.UserMention(claimer))
	case store.ClaimLockHidden:
		return fmt.Sprintf(" This ticket is now only visible to %s and the member.", discord.UserMention(claimer))
	}
	return ""
}
