package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/ticketsbot/internal/auth"
	"github.com/adammcgrogan/ticketsbot/internal/store"
)

type transcriptGuild struct {
	ID      snowflake.ID `json:"id"`
	Name    string       `json:"name"`
	IconURL *string      `json:"icon_url"`
	CanManage bool       `json:"can_manage"`
}

// getTranscript returns a ticket and its messages to anyone allowed to see
// it: the opener, server managers and the ticket type's support staff.
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
	manager := s.managesGuild(r.Context(), sess, t.GuildID)
	// Respond 404 rather than 403 so ticket IDs can't be probed.
	if !manager && sess.User.ID != t.OpenerID && !s.isSupportStaff(r.Context(), sess.User.ID, t) {
		notFound()
		return
	}

	messages, err := s.store.ListTicketMessages(r.Context(), t.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	guild := transcriptGuild{ID: t.GuildID, CanManage: manager}
	if g, err := s.store.GetGuild(r.Context(), t.GuildID); err == nil {
		guild.Name = g.Name
		if g.Icon != nil {
			url := fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.png", g.ID, *g.Icon)
			guild.IconURL = &url
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "messages": messages, "guild": guild})
}

func (s *Server) managesGuild(ctx context.Context, sess *auth.Session, guildID snowflake.ID) bool {
	guilds, err := s.auth.Guilds(ctx, sess)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(guilds, func(g discordGuild) bool { return g.ID == guildID && canManage(g) })
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
	member, err := s.discord.GetMember(t.GuildID, userID, rest.WithCtx(ctx))
	if err != nil {
		return false
	}
	return slices.ContainsFunc(member.RoleIDs, func(id snowflake.ID) bool { return slices.Contains(tt.SupportRoleIDs, id) })
}

var retentionOptions = []int{7, 30, 90, 180, 365}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetGuildSettings(r.Context(), guildFrom(r).ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var in store.GuildSettings
	if !decodeJSON(w, r, &in) {
		return
	}
	if d := in.TranscriptRetentionDays; d != nil && !slices.Contains(retentionOptions, *d) {
		s.writeFailure(w, invalid("transcript_retention_days", "Choose one of the listed retention periods."))
		return
	}
	if err := s.store.UpdateGuildSettings(r.Context(), guildFrom(r).ID, in); err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}
