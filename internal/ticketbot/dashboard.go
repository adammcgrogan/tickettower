package ticketbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/config"
	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// Dashboard acts on tickets for dashboard users: closing them (the close
// message, log entry, DM and cleanup, as in Discord) and replying to them.
// It uses Discord's REST API, so it works in the API process without a
// gateway connection.
//
// The bot notices a deleted ticket channel and forgets it. An archived thread
// stays in the bot's ticketCache until it restarts, which only matters if
// someone posts in the locked thread.
type Dashboard struct{ b *Bot }

// ErrChannelDeleted means a ticket's channel was deleted in Discord without
// the bot noticing. The ticket has been closed.
var ErrChannelDeleted = errors.New("ticket channel was deleted")

func NewDashboard(cfg config.Config, st *store.Store, r rest.Rest, log *slog.Logger) *Dashboard {
	return &Dashboard{b: &Bot{cfg: cfg, store: st, rest: r, log: log, tickets: newTicketCache()}}
}

// Close closes an open ticket for a dashboard user, reporting false if it
// was already closed. The channel is deleted (or the thread archived) in the
// background after the usual delay.
func (d *Dashboard) Close(ctx context.Context, t store.Ticket, byID snowflake.ID, byName, reason string) (bool, error) {
	ok, err := d.b.store.CloseTicket(ctx, t.ID, byID, byName, reason)
	if err != nil || !ok {
		return false, err
	}
	now := time.Now()
	t.Status, t.ClosedBy, t.ClosedByName, t.CloseReason, t.ClosedAt = store.StatusClosed, &byID, &byName, reason, &now

	_, err = d.b.rest.CreateMessage(t.ChannelID, closedMessage(t, d.b.typeOf(ctx, t)), rest.WithCtx(ctx))
	if err != nil && !discordx.IsCode(err, discordx.CodeUnknownChannel) {
		d.b.log.Warn("failed to post close message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	go d.b.finishClose(t)
	return true, nil
}

// Reopen reopens a closed ticket for a dashboard user and posts the reopen
// message in it. The bot picks a thread up again when it sees it
// unarchived, and a kept channel when the ticket's next message arrives.
func (d *Dashboard) Reopen(ctx context.Context, t store.Ticket, byID snowflake.ID) (store.Ticket, error) {
	t, err := d.b.reopenTicket(ctx, t, byID, nil)
	if err != nil {
		return t, err
	}
	if _, err := d.b.rest.CreateMessage(t.ChannelID, reopenedMessage(byID), rest.WithCtx(ctx)); err != nil {
		d.b.log.Warn("failed to post reopen message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	return t, nil
}

// Claim claims an open, unclaimed ticket for a dashboard user, as the Claim
// button in Discord does. The caller has checked they may.
func (d *Dashboard) Claim(ctx context.Context, t store.Ticket, byID snowflake.ID, byName string) (store.Ticket, error) {
	t, tt, err := d.b.loadTicket(ctx, t.ChannelID)
	if err != nil {
		return t, err
	}
	if t.ClaimedBy != nil {
		if *t.ClaimedBy == byID {
			return t, userErr("You've already claimed this ticket.")
		}
		return t, userErr("This ticket is already claimed by %s.", nameOr(t.ClaimedByName, "someone else"))
	}
	ok, err := d.b.store.ClaimTicket(ctx, t.ID, byID, byName)
	if err != nil {
		return t, err
	}
	if !ok {
		return t, userErr("Someone else claimed this ticket just now.")
	}
	t.ClaimedBy, t.ClaimedByName = &byID, &byName
	d.b.applyClaimLock(ctx, t, tt, byID)
	go d.b.logEvent(t.GuildID, claimedLog(t, byID))
	d.post(ctx, t, public(claimedMessage(t, tt, byID)))
	return t, nil
}

// Assign hands an open ticket to a staff member, taking it from whoever
// holds it. The caller has checked the assignee is support staff for it.
func (d *Dashboard) Assign(ctx context.Context, t store.Ticket, byID, toID snowflake.ID, toName string) (store.Ticket, error) {
	t, tt, err := d.b.loadTicket(ctx, t.ChannelID)
	if err != nil {
		return t, err
	}
	if t.ClaimedBy != nil && *t.ClaimedBy == toID {
		return t, userErr("%s already has this ticket.", toName)
	}
	prev, ok, err := d.b.store.AssignTicket(ctx, t.ID, toID, toName)
	if err != nil {
		return t, err
	}
	if !ok {
		return t, userErr("This ticket is already closed.")
	}
	t.ClaimedBy, t.ClaimedByName = &toID, &toName
	if prev != nil {
		d.b.releaseClaimLock(ctx, t, tt, *prev)
	}
	d.b.applyClaimLock(ctx, t, tt, toID)
	go d.b.logEvent(t.GuildID, claimedLog(t, toID))
	msg := "🙋 " + discord.UserMention(byID) + " assigned this ticket to " + discord.UserMention(toID) +
		", who will help you from here." + claimLockNote(t, tt, toID)
	d.post(ctx, t, public(msg))
	return t, nil
}

// Unclaim releases an open ticket's claim for a dashboard user, whoever
// holds it.
func (d *Dashboard) Unclaim(ctx context.Context, t store.Ticket, byID snowflake.ID) (store.Ticket, error) {
	t, tt, err := d.b.loadTicket(ctx, t.ChannelID)
	if err != nil {
		return t, err
	}
	prev, ok, err := d.b.store.ReleaseTicket(ctx, t.ID)
	if err != nil {
		return t, err
	}
	if !ok {
		return t, userErr("This ticket isn't claimed.")
	}
	t.ClaimedBy, t.ClaimedByName = nil, nil
	d.b.releaseClaimLock(ctx, t, tt, prev)
	msg := discord.UserMention(byID) + " unclaimed this ticket."
	if prev != byID {
		msg = discord.UserMention(byID) + " released " + discord.UserMention(prev) + "'s claim on this ticket."
	}
	d.post(ctx, t, public(msg))
	return t, nil
}

func nameOr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

// Move moves an open ticket to another ticket type for a dashboard user, as
// /ticket move does, and returns the updated ticket.
func (d *Dashboard) Move(ctx context.Context, t store.Ticket, typeID int64, byID snowflake.ID) (store.Ticket, error) {
	to, err := d.b.store.GetTicketType(ctx, t.GuildID, typeID)
	if errors.Is(err, store.ErrNotFound) {
		return t, userErr("That ticket type no longer exists.")
	} else if err != nil {
		return t, err
	}
	var from *store.TicketType
	if t.TicketTypeID != nil {
		if v, err := d.b.store.GetTicketType(ctx, t.GuildID, *t.TicketTypeID); err == nil {
			from = &v
		}
	}
	moved, err := d.b.moveTicket(ctx, t, from, to, byID)
	if discordx.IsCode(err, discordx.CodeUnknownChannel) {
		d.b.closeDeletedChannel(t.ChannelID)
		return t, ErrChannelDeleted
	}
	return moved, err
}

// Reply posts a dashboard user's message in an open ticket, with its
// placeholders filled in, and returns it as saved in the transcript. byName
// and avatarURL are how the member knows the staff member, ideally their
// server nickname and avatar.
func (d *Dashboard) Reply(ctx context.Context, t store.Ticket, byID snowflake.ID, byName, avatarURL, text string) (store.TicketMessage, error) {
	server, serverIcon := d.b.guildBrand(ctx, t.GuildID)
	reply := replyMessage(d.b.cfg.AppName, byName, avatarURL, server, serverIcon, replyText(text, t, server, byName))
	m, err := d.b.rest.CreateMessage(t.ChannelID, reply, rest.WithCtx(ctx))
	if discordx.IsCode(err, discordx.CodeUnknownChannel) {
		// Deleted while the bot was offline, so the ticket was never closed.
		d.b.closeDeletedChannel(t.ChannelID)
		return store.TicketMessage{}, ErrChannelDeleted
	} else if err != nil {
		return store.TicketMessage{}, err
	}
	msg := toTicketMessage(t.ID, *m)
	msg.SentBy = &byID
	msg.AuthorStaff = true

	// The bot captures the message too; whichever insert comes second is
	// ignored, apart from noting who sent it. Saving it here means the
	// transcript has it straight away.
	if err := d.b.store.InsertTicketMessage(ctx, msg); err != nil {
		d.b.log.Error("failed to save dashboard reply", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	d.b.recordReply(ctx, t, byID, m.CreatedAt)
	return msg, nil
}

// guildBrand returns a guild's name and icon URL, for reply footers.
func (b *Bot) guildBrand(ctx context.Context, id snowflake.ID) (name, iconURL string) {
	name = b.guildName(ctx, id)
	if g, err := b.store.GetGuild(ctx, id); err == nil && g.Icon != nil {
		iconURL = fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.png", g.ID, *g.Icon)
	}
	return name, iconURL
}

// replyMessage is a staff reply the bot posts, from the dashboard or with
// /reply. The embed names the staff member at the top and says at the bottom
// that the server's staff sent it, so members know it's a real reply from
// the team. appName is set for replies from the dashboard, to say so.
func replyMessage(appName, staff, staffAvatar, server, serverIcon, text string) discord.MessageCreate {
	footer := fmt.Sprintf("Sent by %s staff", server)
	if appName != "" {
		footer += fmt.Sprintf(" from the %s Dashboard", appName)
	}
	return discord.NewMessageCreate().
		WithEmbeds(discord.NewEmbed().
			WithAuthor(staff, "", staffAvatar).
			WithDescription(text).
			WithColor(colorAccent).
			WithFooter(footer, serverIcon)).
		WithAllowedMentions(&discord.AllowedMentions{})
}

// TicketMember is someone with their own access to a ticket.
type TicketMember struct {
	ID        snowflake.ID `json:"id"`
	Name      string       `json:"name"`
	AvatarURL string       `json:"avatar_url"`
	// Opener is whoever opened the ticket, who can't be removed from it.
	Opener bool `json:"opener"`
}

const (
	// maxThreadMembers is the most members Discord lists in one request.
	maxThreadMembers = 100
	// maxChannelMembers caps the member lookups for a channel's list. A
	// ticket rarely has more than a few people added.
	maxChannelMembers = 25
)

// Members lists who has their own access to an open ticket, the opener
// first: for a channel, the people in its permissions (the support team sees
// it through its roles, so only people added one by one are listed); for a
// thread, its members. The bot is left out.
func (d *Dashboard) Members(ctx context.Context, t store.Ticket) ([]TicketMember, error) {
	out := []TicketMember{}
	if t.Mode == store.ModeThread {
		page := d.b.rest.GetThreadMembersPage(t.ChannelID, 0, maxThreadMembers, rest.WithCtx(ctx))
		if !page.Next() && !errors.Is(page.Err, rest.ErrNoMorePages) {
			return nil, d.channelErr(t, page.Err)
		}
		for _, tm := range page.Items {
			if tm.UserID == d.b.botID() {
				continue
			}
			m := TicketMember{ID: tm.UserID, Opener: tm.UserID == t.OpenerID}
			if tm.Member != nil {
				m.Name, m.AvatarURL = tm.Member.EffectiveName(), tm.Member.EffectiveAvatarURL()
			}
			out = append(out, m)
		}
	} else {
		ch, err := d.b.rest.GetChannel(t.ChannelID, rest.WithCtx(ctx))
		if err != nil {
			return nil, d.channelErr(t, err)
		}
		gc, ok := ch.(discord.GuildChannel)
		if !ok {
			return nil, fmt.Errorf("ticket channel %s isn't a server channel", t.ChannelID)
		}
		ids := channelMemberIDs(gc.PermissionOverwrites(), d.b.botID())
		for _, id := range ids[:min(len(ids), maxChannelMembers)] {
			m := TicketMember{ID: id, Opener: id == t.OpenerID}
			// People who have left the server keep their permissions, unnamed.
			if mem, err := d.b.rest.GetMember(t.GuildID, id, rest.WithCtx(ctx)); err == nil {
				m.Name, m.AvatarURL = mem.EffectiveName(), mem.EffectiveAvatarURL()
			}
			out = append(out, m)
		}
	}
	for i := range out {
		if out[i].Opener && out[i].Name == "" {
			out[i].Name = t.OpenerName
		}
	}
	sortMembers(out)
	return out, nil
}

// channelMemberIDs returns who has a ticket channel's own permission to see
// it, apart from the bot: its opener and the people added to it.
func channelMemberIDs(overwrites discord.PermissionOverwrites, botID snowflake.ID) []snowflake.ID {
	var ids []snowflake.ID
	for _, o := range overwrites {
		if mo, ok := o.(discord.MemberPermissionOverwrite); ok && mo.UserID != botID && mo.Allow.Has(discord.PermissionViewChannel) {
			ids = append(ids, mo.UserID)
		}
	}
	return ids
}

// sortMembers puts the opener first, then everyone else by name.
func sortMembers(ms []TicketMember) {
	slices.SortStableFunc(ms, func(a, b TicketMember) int {
		if a.Opener != b.Opener {
			if a.Opener {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

// AddMember gives someone access to an open ticket for a dashboard user, as
// /ticket add does, and says so in the ticket.
func (d *Dashboard) AddMember(ctx context.Context, t store.Ticket, byID snowflake.ID, target discord.User) error {
	if err := d.b.addToTicket(ctx, t.ChannelID, nil, target); err != nil {
		return d.channelErr(t, err)
	}
	d.post(ctx, t, addedMessage(byID, target.ID))
	return nil
}

// RemoveMember takes someone's access to an open ticket away for a dashboard
// user, as /ticket remove does, and says so in the ticket. targetMember is
// nil if they've left the server.
func (d *Dashboard) RemoveMember(ctx context.Context, t store.Ticket, byID snowflake.ID, target discord.User, targetMember *discord.ResolvedMember) error {
	if err := d.b.removeFromTicket(ctx, t.ChannelID, nil, target, targetMember); err != nil {
		return d.channelErr(t, err)
	}
	d.post(ctx, t, removedMessage(byID, target.ID))
	return nil
}

// Rename renames an open ticket's channel or thread for a dashboard user.
func (d *Dashboard) Rename(ctx context.Context, t store.Ticket, name string) error {
	// Past Discord's two renames every 10 minutes the request waits for the
	// limit to reset, so give up long before that; renameTicket explains.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return d.channelErr(t, d.b.renameTicket(ctx, t.ChannelID, nil, name))
}

// channelErr turns Discord's "unknown channel" for a ticket into
// ErrChannelDeleted, closing the ticket, since the bot never noticed.
func (d *Dashboard) channelErr(t store.Ticket, err error) error {
	if discordx.IsCode(err, discordx.CodeUnknownChannel) {
		d.b.closeDeletedChannel(t.ChannelID)
		return ErrChannelDeleted
	}
	return err
}
