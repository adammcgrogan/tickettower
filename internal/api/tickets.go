package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/store"
)

type closeTicketInput struct {
	Reason string `json:"reason"`
}

// closeTicket closes a ticket from the dashboard, just as the Close button
// in Discord does.
func (s *Server) closeTicket(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	id, ok := pathID(r, "ticketID")
	if !ok {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}
	t, err := s.store.GetGuildTicket(r.Context(), g.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	} else if err != nil {
		s.writeFailure(w, err)
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
		if closed, err = s.closer.Close(r.Context(), t, user.ID, user.DisplayName, in.Reason); err != nil {
			s.writeFailure(w, err)
			return
		}
	}
	if !closed {
		s.writeFailure(w, invalid("", "This ticket is already closed."))
		return
	}

	if t, err = s.store.GetGuildTicket(r.Context(), g.ID, id); err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}
