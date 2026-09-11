package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/auth"
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
	// Respond 404 rather than 403 so ticket IDs can't be probed.
	if !dashboard && sess.User.ID != t.OpenerID && !s.isSupportStaff(r.Context(), sess.User.ID, t) {
		notFound()
		return
	}

	messages, err := s.store.ListTicketMessages(r.Context(), t.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	guild := transcriptGuild{ID: t.GuildID, CanManage: dashboard}
	if g, err := s.store.GetGuild(r.Context(), t.GuildID); err == nil {
		guild.Name = g.Name
		if g.Icon != nil {
			url := fmt.Sprintf("https://cdn.discordapp.com/icons/%s/%s.png", g.ID, *g.Icon)
			guild.IconURL = &url
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "messages": messages, "guild": guild})
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
