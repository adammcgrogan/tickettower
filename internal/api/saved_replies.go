package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/adammcgrogan/tickettower/internal/store"
)

type savedReplyInput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func validateSavedReply(in *savedReplyInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > store.MaxSavedReplyName {
		return invalid("name", fmt.Sprintf("Give the reply a name of up to %d characters.", store.MaxSavedReplyName))
	}
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" {
		return invalid("content", "Write the message to send.")
	}
	if utf8.RuneCountInString(in.Content) > store.MaxSavedReplyContent {
		return invalid("content", "Keep the message to 2,000 characters or fewer.")
	}
	return nil
}

// savedReplyFailure explains a taken name, and otherwise writes err as usual.
func (s *Server) savedReplyFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrDuplicateName) {
		err = invalid("name", "There's already a saved reply with this name.")
	}
	s.writeFailure(w, err)
}

func (s *Server) listSavedReplies(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSavedReplies(r.Context(), guildFrom(r).ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createSavedReply(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	var in savedReplyInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := validateSavedReply(&in); err != nil {
		s.writeFailure(w, err)
		return
	}
	count, err := s.store.CountSavedReplies(r.Context(), g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	if max := s.limits(r.Context(), g.ID).MaxSavedReplies; count >= max {
		s.writeFailure(w, invalid("", fmt.Sprintf("You can have up to %d saved replies.", max)))
		return
	}

	reply := store.SavedReply{GuildID: g.ID, Name: in.Name, Content: in.Content}
	if err := s.store.CreateSavedReply(r.Context(), &reply); err != nil {
		s.savedReplyFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, reply)
}

func (s *Server) updateSavedReply(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "replyID")
	if !ok {
		writeError(w, http.StatusNotFound, "saved reply not found")
		return
	}
	var in savedReplyInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := validateSavedReply(&in); err != nil {
		s.writeFailure(w, err)
		return
	}
	reply := store.SavedReply{ID: id, GuildID: guildFrom(r).ID, Name: in.Name, Content: in.Content}
	err := s.store.UpdateSavedReply(r.Context(), &reply)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "saved reply not found")
		return
	} else if err != nil {
		s.savedReplyFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

func (s *Server) deleteSavedReply(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "replyID")
	if !ok {
		writeError(w, http.StatusNotFound, "saved reply not found")
		return
	}
	err := s.store.DeleteSavedReply(r.Context(), guildFrom(r).ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "saved reply not found")
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
