package ticketbot

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// ticketCache tracks open ticket channels so message events from ordinary
// channels never touch the database.
type ticketCache struct {
	mu   sync.RWMutex
	refs map[snowflake.ID]*store.TicketRef
}

func newTicketCache() *ticketCache {
	return &ticketCache{refs: make(map[snowflake.ID]*store.TicketRef)}
}

func (c *ticketCache) get(channelID snowflake.ID) (store.TicketRef, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.refs[channelID]
	if !ok {
		return store.TicketRef{}, false
	}
	return *r, true
}

func (c *ticketCache) put(r store.TicketRef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refs[r.ChannelID] = &r
}

func (c *ticketCache) remove(channelID snowflake.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.refs, channelID)
}

// channelIDs lists the tracked channels, or only a guild's if guildID isn't 0.
func (c *ticketCache) channelIDs(guildID snowflake.ID) []snowflake.ID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ids := make([]snowflake.ID, 0, len(c.refs))
	for id, r := range c.refs {
		if guildID == 0 || r.GuildID == guildID {
			ids = append(ids, id)
		}
	}
	return ids
}

// removeGuild forgets every ticket of a guild, e.g. when the bot leaves it.
func (c *ticketCache) removeGuild(guildID snowflake.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, r := range c.refs {
		if r.GuildID == guildID {
			delete(c.refs, id)
		}
	}
}

func (c *ticketCache) replace(refs []store.TicketRef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refs = make(map[snowflake.ID]*store.TicketRef, len(refs))
	for i := range refs {
		c.refs[refs[i].ChannelID] = &refs[i]
	}
}

// markResponded records a staff reply, reporting true only the first time.
func (c *ticketCache) markResponded(channelID snowflake.ID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.refs[channelID]
	if !ok || r.HasFirstResponse {
		return false
	}
	r.HasFirstResponse = true
	return true
}

// captured reports whether a message type is kept in transcripts: ordinary
// messages, replies and the responses to slash and context menu commands
// (the bot's own "Ticket closed" and "X added Y" notices from /close and
// /ticket, and any other bot's used in the ticket). System messages such as
// "user joined the thread" are skipped.
func captured(t discord.MessageType) bool {
	switch t {
	case discord.MessageTypeDefault, discord.MessageTypeReply,
		discord.MessageTypeSlashCommand, discord.MessageTypeContextMenuCommand:
		return true
	}
	return false
}

func (b *Bot) onMessageCreate(e *events.GuildMessageCreate) {
	ref, ok := b.tickets.get(e.ChannelID)
	if !ok || !captured(e.Message.Type) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := e.Message
	staff := b.isStaffMessage(ctx, e.GuildID, ref, m)
	saved := toTicketMessage(ref.ID, m)
	saved.AuthorStaff = staff
	if err := b.store.InsertTicketMessage(ctx, saved); err != nil {
		b.log.Error("failed to save ticket message", slog.Any("err", err))
	}
	// The team's first message counts as the first response.
	if staff && b.tickets.markResponded(e.ChannelID) {
		if err := b.store.SetFirstResponse(ctx, ref.ID, m.CreatedAt); err != nil {
			b.log.Error("failed to record first response", slog.Any("err", err))
		}
	}
	// Any human message restarts the auto-close clock. Anyone who isn't
	// staff (the opener, or someone added to the ticket) is the member's
	// side, so their message means the team owes a reply.
	if !m.Author.Bot {
		if err := b.store.RecordActivity(ctx, ref.ID, m.CreatedAt, !staff); err != nil {
			b.log.Error("failed to record ticket activity", slog.Any("err", err))
		}
	}
}

// isStaffMessage reports whether a captured message is the team's: written
// by someone with one of the ticket type's support roles, or who manages the
// server. The opener and bots never are. It looks the ticket up only for
// messages that could be, so the member's own messages cost nothing extra.
func (b *Bot) isStaffMessage(ctx context.Context, guildID snowflake.ID, ref store.TicketRef, m discord.Message) bool {
	if m.Author.Bot || m.Author.ID == ref.OpenerID || m.Member == nil {
		return false
	}
	t, err := b.store.GetTicket(ctx, ref.ID)
	if err != nil {
		b.log.Warn("failed to load ticket for a message", slog.Int64("ticket_id", ref.ID), slog.Any("err", err))
		return false
	}
	var supportRoles []snowflake.ID
	if t.TicketTypeID != nil {
		if tt, err := b.store.GetTicketType(ctx, t.GuildID, *t.TicketTypeID); err == nil {
			supportRoles = tt.SupportRoleIDs
		}
	}
	return authorIsStaff(m.Member.RoleIDs, supportRoles, b.managesGuild(guildID, m.Author.ID, m.Member.RoleIDs))
}

// authorIsStaff is the rule behind isStaffMessage: a support role, or
// managing the server.
func authorIsStaff(roles, supportRoles []snowflake.ID, manages bool) bool {
	if manages {
		return true
	}
	for _, id := range roles {
		if slices.Contains(supportRoles, id) {
			return true
		}
	}
	return false
}

// managesGuild reports, from the gateway cache, whether a member owns the
// server or has a role with Administrator or Manage Server. Without a
// gateway (the dashboard) it reports false; dashboard users are checked by
// the API.
func (b *Bot) managesGuild(guildID, userID snowflake.ID, roles []snowflake.ID) bool {
	if b.client == nil {
		return false
	}
	if g, ok := b.client.Caches.Guild(guildID); ok && g.OwnerID == userID {
		return true
	}
	for _, id := range roles {
		if r, ok := b.client.Caches.Role(guildID, id); ok &&
			(r.Permissions.Has(discord.PermissionAdministrator) || r.Permissions.Has(discord.PermissionManageGuild)) {
			return true
		}
	}
	return false
}

func (b *Bot) onMessageUpdate(e *events.GuildMessageUpdate) {
	if _, ok := b.tickets.get(e.ChannelID); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := e.Message
	if err := b.store.UpdateTicketMessage(ctx, m.ID, m.Content, toEmbeds(m.Embeds), m.EditedTimestamp); err != nil {
		b.log.Error("failed to update ticket message", slog.Any("err", err))
	}
}

func (b *Bot) onMessageDelete(e *events.GuildMessageDelete) {
	if _, ok := b.tickets.get(e.ChannelID); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.store.MarkTicketMessageDeleted(ctx, e.MessageID); err != nil {
		b.log.Error("failed to mark ticket message deleted", slog.Any("err", err))
	}
}

func toTicketMessage(ticketID int64, m discord.Message) store.TicketMessage {
	out := store.TicketMessage{
		ID:           m.ID,
		TicketID:     ticketID,
		AuthorID:     m.Author.ID,
		AuthorName:   authorName(m),
		AuthorAvatar: m.Author.EffectiveAvatarURL(),
		AuthorBot:    m.Author.Bot,
		Content:      m.Content,
		Embeds:       toEmbeds(m.Embeds),
		CreatedAt:    m.CreatedAt,
	}
	for _, a := range m.Attachments {
		att := store.Attachment{Name: a.Filename, URL: a.URL, Size: a.Size}
		if a.ContentType != nil {
			att.ContentType = *a.ContentType
		}
		out.Attachments = append(out.Attachments, att)
	}
	return out
}

// authorName is the name people saw in the server: the author's nickname
// there if they have one, otherwise their display name. Gateway messages
// carry a partial member (no user), so the nickname is read directly.
func authorName(m discord.Message) string {
	if m.Member != nil && m.Member.Nick != nil && strings.TrimSpace(*m.Member.Nick) != "" {
		return *m.Member.Nick
	}
	return m.Author.EffectiveName()
}

func toEmbeds(in []discord.Embed) []store.Embed {
	out := make([]store.Embed, 0, len(in))
	for _, e := range in {
		se := store.Embed{Title: e.Title, Description: e.Description, Color: e.Color}
		if e.Footer != nil {
			se.Footer = e.Footer.Text
		}
		// Dashboard replies name the staff member who wrote them here.
		if e.Author != nil {
			se.Author = &store.EmbedAuthor{Name: e.Author.Name, IconURL: e.Author.IconURL}
		}
		for _, f := range e.Fields {
			se.Fields = append(se.Fields, store.EmbedField{Name: f.Name, Value: f.Value})
		}
		out = append(out, se)
	}
	return out
}

// leftGuildRetention is how long a server's data is kept after the bot is
// removed from it, so re-adding the bot soon after restores the setup. The
// privacy page states it.
const leftGuildRetention = 30 * 24 * time.Hour

// purgeTranscripts applies each guild's transcript retention setting and
// deletes the data of servers the bot left over leftGuildRetention ago, once
// at startup and then hourly.
func (b *Bot) purgeTranscripts(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		n, err := b.store.PurgeExpiredTranscripts(ctx)
		if err != nil && ctx.Err() == nil {
			b.log.Error("failed to purge transcripts", slog.Any("err", err))
		} else if n > 0 {
			b.log.Info("purged expired transcript messages and notes", slog.Int64("count", n))
		}
		n, err = b.store.PurgeLeftGuilds(ctx, leftGuildRetention)
		if err != nil && ctx.Err() == nil {
			b.log.Error("failed to purge left guilds", slog.Any("err", err))
		} else if n > 0 {
			b.log.Info("deleted the data of servers the bot left", slog.Int64("count", n))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
