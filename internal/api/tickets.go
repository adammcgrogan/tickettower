package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/disgoorg/disgo/rest"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/store"
	"github.com/adammcgrogan/tickettower/internal/ticketbot"
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

type moveTicketInput struct {
	TypeID int64 `json:"type_id"`
}

// moveTicket moves a ticket to another ticket type, as /ticket move does.
func (s *Server) moveTicket(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	var in moveTicketInput
	if !decodeJSON(w, r, &in) {
		return
	}
	user := auth.FromContext(r.Context()).User
	t, err := s.dashboard.Move(r.Context(), t, in.TypeID, user.ID)
	if errors.Is(err, ticketbot.ErrChannelDeleted) {
		writeError(w, http.StatusConflict, "This ticket's channel was deleted in Discord, so the ticket has been closed.")
		return
	} else if msg, ok := ticketbot.UserMessage(err); ok {
		s.writeFailure(w, invalid("type_id", msg))
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// reopenTicket reopens a closed thread ticket from the dashboard.
func (s *Server) reopenTicket(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	user := auth.FromContext(r.Context()).User
	t, err := s.dashboard.Reopen(r.Context(), t, user.ID)
	if msg, ok := ticketbot.UserMessage(err); ok {
		s.writeFailure(w, invalid("", msg))
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type holdInput struct {
	Reason string `json:"reason"`
}

// holdTicket puts a ticket on hold from the dashboard.
func (s *Server) holdTicket(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	var in holdInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > ticketbot.MaxHoldReason {
		s.writeFailure(w, invalid("reason", "Keep the reason to 200 characters or fewer."))
		return
	}
	user := auth.FromContext(r.Context()).User
	t, err := s.dashboard.Hold(r.Context(), t, user.ID, in.Reason)
	s.writeTicketAction(w, t, err)
}

// resumeTicket takes a ticket off hold from the dashboard.
func (s *Server) resumeTicket(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	user := auth.FromContext(r.Context()).User
	t, err := s.dashboard.Resume(r.Context(), t, user.ID)
	s.writeTicketAction(w, t, err)
}

// writeTicketAction writes the result of a dashboard action on a ticket:
// the bot's user-facing reasons become 422s, and on success the ticket is
// re-read so the response is complete.
func (s *Server) writeTicketAction(w http.ResponseWriter, t store.Ticket, err error) {
	if msg, ok := ticketbot.UserMessage(err); ok {
		s.writeFailure(w, invalid("", msg))
		return
	} else if err != nil {
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
	if errors.Is(err, ticketbot.ErrChannelDeleted) {
		writeError(w, http.StatusConflict, "This ticket's channel was deleted in Discord, so the ticket has been closed.")
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	if t, err = s.store.GetGuildTicket(r.Context(), t.GuildID, t.ID); err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"message": msg, "ticket": t})
}
