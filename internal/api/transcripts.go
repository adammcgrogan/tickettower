package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

type transcriptGuild struct {
	ID        snowflake.ID `json:"id"`
	Name      string       `json:"name"`
	IconURL   *string      `json:"icon_url"`
	CanManage bool         `json:"can_manage"`
}

// getTranscript returns a ticket and its messages to anyone allowed to see
// it: the opener, anyone with dashboard access and the ticket type's support
// staff.
func (s *Server) getTranscript(w http.ResponseWriter, r *http.Request) {
	notFound := func() { writeError(w, http.StatusNotFound, "transcript not found") }
	id, ok := pathID(r, "ticketID")
	if !ok {
		notFound()
		return
	}
	t, err := s.store.GetTicket(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		notFound()
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}

	sess := auth.FromContext(r.Context())
	_, dashboard, err := s.access(r.Context(), sess, t.GuildID)
	if err != nil {
		s.log.Warn("check transcript access", slog.Any("err", err))
	}
	opener := sess.User.ID == t.OpenerID
	staff := dashboard || !opener && s.isSupportStaff(r.Context(), sess.User.ID, t)
	// Respond 404 rather than 403 so ticket IDs can't be probed.
	if !staff && !opener {
		notFound()
		return
	}

	messages, err := s.store.ListTicketMessages(r.Context(), t.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	s.refreshAttachments(r.Context(), messages)
	guild := transcriptGuild{ID: t.GuildID, CanManage: dashboard}
	if g, err := s.store.GetGuild(r.Context(), t.GuildID); err == nil {
		guild.Name = g.Name
		if g.Icon != nil {
			url := fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.png", g.ID, *g.Icon)
			guild.IconURL = &url
		}
	}
	roles, channels := s.mentionNames(r.Context(), t.GuildID)
	resp := map[string]any{"ticket": t, "messages": messages, "guild": guild, "roles": roles, "channels": channels}
	// Staff notes are for the team, never for the member who opened the
	// ticket, even if they're on the team themselves.
	if staff && !opener {
		notes, err := s.store.ListTicketNotes(r.Context(), t.GuildID, t.ID)
		if err != nil {
			s.writeFailure(w, err)
			return
		}
		resp["notes"] = notes
	}
	writeJSON(w, http.StatusOK, resp)
}

// mentionNames returns the guild's current role and channel names by ID, so
// the transcript can show "@Moderators" and "#billing" instead of bare
// mentions. Best effort: if the bot has left the guild the maps are empty
// and the viewer sees generic placeholders.
func (s *Server) mentionNames(ctx context.Context, guildID snowflake.ID) (roles, channels map[string]string) {
	roles, channels = map[string]string{}, map[string]string{}
	if rs, err := s.rawRoles(ctx, guildID); err == nil {
		for _, r := range rs {
			roles[r.ID.String()] = r.Name
		}
	} else {
		s.log.Debug("roles for transcript", slog.Any("err", err))
	}
	if cs, err := s.rawChannels(ctx, guildID); err == nil {
		for _, c := range cs {
			channels[c.ID().String()] = c.Name()
		}
	} else {
		s.log.Debug("channels for transcript", slog.Any("err", err))
	}
	return roles, channels
}

// isSupportStaff checks the user's current roles against the ticket type's
// support roles.
func (s *Server) isSupportStaff(ctx context.Context, userID snowflake.ID, t store.Ticket) bool {
	if t.TicketTypeID == nil {
		return false
	}
	tt, err := s.store.GetTicketType(ctx, t.GuildID, *t.TicketTypeID)
	if err != nil || len(tt.SupportRoleIDs) == 0 {
		return false
	}
	roles, err := s.memberRoles(ctx, t.GuildID, userID)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(roles, func(id snowflake.ID) bool { return slices.Contains(tt.SupportRoleIDs, id) })
}

// attachmentRefreshLead is how close to expiry an attachment link is renewed,
// so a transcript open in a tab keeps working for a while.
const attachmentRefreshLead = time.Hour

// refreshAttachments swaps expired or expiring Discord CDN links in a
// transcript for freshly signed ones, since Discord's attachment links stop
// working after a day or so. Fresh links are cached by their original URL.
// Best effort: if Discord won't refresh a link, the stored one is kept.
func (s *Server) refreshAttachments(ctx context.Context, messages []store.TicketMessage) {
	refresh := func(urls []string) (map[string]string, error) {
		return discordx.RefreshAttachmentURLs(s.discord, urls, rest.WithCtx(ctx))
	}
	if err := renewAttachmentLinks(messages, time.Now(), s.cache, refresh); err != nil {
		s.log.Warn("failed to refresh transcript attachment links", slog.Any("err", err))
	}
}

// renewAttachmentLinks is refreshAttachments without the server, so it can
// be tested with a fake refresh call.
func renewAttachmentLinks(messages []store.TicketMessage, now time.Time, cache *ttlCache,
	refresh func([]string) (map[string]string, error)) error {
	cutoff := now.Add(attachmentRefreshLead)
	fresh := map[string]string{}
	var stale []string
	for _, m := range messages {
		for _, a := range m.Attachments {
			exp := discordx.AttachmentExpiry(a.URL)
			if exp.IsZero() || exp.After(cutoff) {
				continue
			}
			if v, ok := cache.get("attachment:" + a.URL); ok {
				fresh[a.URL] = v.(string)
			} else if _, seen := fresh[a.URL]; !seen {
				stale = append(stale, a.URL)
				fresh[a.URL] = "" // placeholder so each URL is requested once
			}
		}
	}
	var err error
	if len(stale) > 0 {
		var renewed map[string]string
		renewed, err = refresh(stale)
		for orig, u := range renewed {
			fresh[orig] = u
			ttl := time.Until(discordx.AttachmentExpiry(u)) - attachmentRefreshLead
			if ttl > 0 {
				cache.set("attachment:"+orig, u, ttl)
			}
		}
	}
	for i := range messages {
		for j, a := range messages[i].Attachments {
			if u := fresh[a.URL]; u != "" {
				messages[i].Attachments[j].URL = u
			}
		}
	}
	return err
}
