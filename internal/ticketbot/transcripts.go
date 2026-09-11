package ticketbot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/ticketsbot/internal/store"
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

// capturedTypes are the message types kept in transcripts; system messages
// such as "user joined the thread" are skipped.
func captured(t discord.MessageType) bool {
	return t == discord.MessageTypeDefault || t == discord.MessageTypeReply
}

func (b *Bot) onMessageCreate(e *events.GuildMessageCreate) {
	ref, ok := b.tickets.get(e.ChannelID)
	if !ok || !captured(e.Message.Type) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := e.Message
	if err := b.store.InsertTicketMessage(ctx, toTicketMessage(ref.ID, m)); err != nil {
		b.log.Error("failed to save ticket message", slog.Any("err", err))
	}
	// The first reply from someone other than the opener counts as the
	// first response.
	if !m.Author.Bot && m.Author.ID != ref.OpenerID && b.tickets.markResponded(e.ChannelID) {
		if err := b.store.SetFirstResponse(ctx, ref.ID, m.CreatedAt); err != nil {
			b.log.Error("failed to record first response", slog.Any("err", err))
		}
	}
	// Any human message restarts the auto-close clock.
	if !m.Author.Bot {
		if err := b.store.RecordActivity(ctx, ref.ID, m.CreatedAt, m.Author.ID == ref.OpenerID); err != nil {
			b.log.Error("failed to record ticket activity", slog.Any("err", err))
		}
	}
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
		AuthorName:   m.Author.EffectiveName(),
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

func toEmbeds(in []discord.Embed) []store.Embed {
	out := make([]store.Embed, 0, len(in))
	for _, e := range in {
		se := store.Embed{Title: e.Title, Description: e.Description, Color: e.Color}
		if e.Footer != nil {
			se.Footer = e.Footer.Text
		}
		for _, f := range e.Fields {
			se.Fields = append(se.Fields, store.EmbedField{Name: f.Name, Value: f.Value})
		}
		out = append(out, se)
	}
	return out
}

// purgeTranscripts applies each guild's transcript retention setting, once at
// startup and then hourly.
func (b *Bot) purgeTranscripts(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		n, err := b.store.PurgeExpiredTranscripts(ctx)
		if err != nil && ctx.Err() == nil {
			b.log.Error("failed to purge transcripts", slog.Any("err", err))
		} else if n > 0 {
			b.log.Info("purged expired transcript messages", slog.Int64("count", n))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
