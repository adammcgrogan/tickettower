package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
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

const (
	// maxBulkClose is the most tickets one request can close.
	maxBulkClose = 50
	// bulkCloseGap spaces closes out, since each posts a message and later
	// deletes a channel, so a big batch doesn't hit Discord all at once.
	bulkCloseGap = 300 * time.Millisecond
	// bulkCloseBudget is how long one request keeps closing. Tickets it
	// doesn't reach are returned as pending, for the dashboard to send again.
	bulkCloseBudget = 15 * time.Second
)

type bulkCloseInput struct {
	IDs    []int64 `json:"ids"`
	Reason string  `json:"reason"`
}

type bulkCloseFailure struct {
	ID    int64  `json:"id"`
	Error string `json:"error"`
}

type bulkCloseResult struct {
	Closed  []store.Ticket     `json:"closed"`
	Failed  []bulkCloseFailure `json:"failed"`
	Pending []int64            `json:"pending"`
}

// validateBulkClose tidies a bulk close request: the reason trimmed, and
// the IDs de-duplicated in the order given.
func validateBulkClose(in *bulkCloseInput) error {
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > 500 {
		return invalid("reason", "Keep the reason to 500 characters or fewer.")
	}
	seen := make(map[int64]bool, len(in.IDs))
	ids := in.IDs[:0]
	for _, id := range in.IDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	in.IDs = ids
	switch {
	case len(in.IDs) == 0:
		return invalid("", "Choose at least one ticket to close.")
	case len(in.IDs) > maxBulkClose:
		return invalid("", fmt.Sprintf("Close up to %d tickets at a time.", maxBulkClose))
	}
	return nil
}

// closeTickets closes several tickets at once from the dashboard, each just
// as closeTicket does. It reports which closed, which couldn't and why, and
// which it ran out of time for.
func (s *Server) closeTickets(w http.ResponseWriter, r *http.Request) {
	var in bulkCloseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := validateBulkClose(&in); err != nil {
		s.writeFailure(w, err)
		return
	}

	ctx := r.Context()
	guildID := guildFrom(r).ID
	user := auth.FromContext(ctx).User
	res := bulkCloseResult{Closed: []store.Ticket{}, Failed: []bulkCloseFailure{}, Pending: []int64{}}
	deadline := time.Now().Add(bulkCloseBudget)
	for i, id := range in.IDs {
		if i > 0 {
			if time.Now().Add(bulkCloseGap).After(deadline) || ctx.Err() != nil {
				res.Pending = in.IDs[i:]
				break
			}
			time.Sleep(bulkCloseGap)
		}
		fail := func(msg string) { res.Failed = append(res.Failed, bulkCloseFailure{ID: id, Error: msg}) }

		t, err := s.store.GetGuildTicket(ctx, guildID, id)
		if errors.Is(err, store.ErrNotFound) {
			fail("Ticket not found.")
			continue
		} else if err != nil {
			s.log.Error("bulk close: failed to load ticket", slog.Int64("ticket_id", id), slog.Any("err", err))
			fail("Something went wrong. Try again.")
			continue
		}
		closed := false
		if t.Status == store.StatusOpen {
			if closed, err = s.dashboard.Close(ctx, t, user.ID, user.DisplayName, in.Reason); err != nil {
				s.log.Error("bulk close: failed to close ticket", slog.Int64("ticket_id", id), slog.Any("err", err))
				fail("Something went wrong. Try again.")
				continue
			}
		}
		if !closed {
			fail("Already closed.")
			continue
		}
		if t, err = s.store.GetGuildTicket(ctx, guildID, id); err == nil {
			res.Closed = append(res.Closed, t)
		}
	}
	writeJSON(w, http.StatusOK, res)
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
		writeError(w, http.StatusConflict, channelDeleted)
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
		writeError(w, http.StatusConflict, channelDeleted)
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

type noteInput struct {
	Content string `json:"content"`
}

// addTicketNote saves a private note on a ticket from the dashboard, open or
// closed, and returns it.
func (s *Server) addTicketNote(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	var in noteInput
	if !decodeJSON(w, r, &in) {
		return
	}
	user := auth.FromContext(r.Context()).User
	// Notes are signed with the staff member's server nickname, as notes
	// left in Discord are.
	name := user.DisplayName
	if m, err := s.discord.GetMember(t.GuildID, user.ID, rest.WithCtx(r.Context())); err == nil {
		name = m.EffectiveName()
	}
	note, err := s.dashboard.AddNote(r.Context(), t, user.ID, name, in.Content)
	if s.actionFailed(w, err, "content") {
		return
	}
	writeJSON(w, http.StatusCreated, note)
}
