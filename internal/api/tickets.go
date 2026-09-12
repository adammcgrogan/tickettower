package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/disgoorg/disgo/rest"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// maxReply is Discord's limit on the text of a message.
const maxReply = 2000

// guildTicket loads the ticket in the URL, writing a 404 if the guild has no
// such ticket.
func (s *Server) guildTicket(w http.ResponseWriter, r *http.Request) (store.Ticket, bool) {
	id, ok := pathID(r, "ticketID")
	if !ok {
		writeError(w, http.StatusNotFound, "ticket not found")
		return store.Ticket{}, false
	}
	t, err := s.store.GetGuildTicket(r.Context(), guildFrom(r).ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ticket not found")
		return t, false
	} else if err != nil {
		s.writeFailure(w, err)
		return t, false
	}
	return t, true
}

type closeTicketInput struct {
	Reason string `json:"reason"`
}

// closeTicket closes a ticket from the dashboard, just as the Close button
// in Discord does.
func (s *Server) closeTicket(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	var in closeTicketInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > 500 {
		s.writeFailure(w, invalid("reason", "Keep the reason to 500 characters or fewer."))
		return
	}

	user := auth.FromContext(r.Context()).User
	closed := false
	if t.Status == store.StatusOpen {
		var err error
		if closed, err = s.dashboard.Close(r.Context(), t, user.ID, user.DisplayName, in.Reason); err != nil {
			s.writeFailure(w, err)
			return
		}
	}
	if !closed {
		s.writeFailure(w, invalid("", "This ticket is already closed."))
		return
	}

	t, err := s.store.GetGuildTicket(r.Context(), t.GuildID, t.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type replyInput struct {
	Content string `json:"content"`
}

// replyTicket posts a message in a ticket from the dashboard. It returns the
// message as saved in the transcript, and the updated ticket.
func (s *Server) replyTicket(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	var in replyInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Content = strings.TrimSpace(in.Content)
	switch {
	case in.Content == "":
		s.writeFailure(w, invalid("content", "Write a message first."))
		return
	case utf8.RuneCountInString(in.Content) > maxReply:
		s.writeFailure(w, invalid("content", "Keep replies to 2,000 characters or fewer."))
		return
	case t.Status != store.StatusOpen:
		s.writeFailure(w, invalid("", "This ticket is closed, so it can't take replies."))
		return
	}

	user := auth.FromContext(r.Context()).User
	// Members know staff by their server nickname and avatar, if they have
	// them, so use those when Discord can tell us.
	name, avatar := user.DisplayName, user.AvatarURL
	if m, err := s.discord.GetMember(t.GuildID, user.ID, rest.WithCtx(r.Context())); err == nil {
		name, avatar = m.EffectiveName(), m.EffectiveAvatarURL()
	}
	msg, err := s.dashboard.Reply(r.Context(), t, user.ID, name, avatar, in.Content)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	if t, err = s.store.GetGuildTicket(r.Context(), t.GuildID, t.ID); err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"message": msg, "ticket": t})
}
