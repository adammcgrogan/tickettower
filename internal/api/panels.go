package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/ticketsbot/internal/discordx"
	"github.com/adammcgrogan/ticketsbot/internal/panels"
	"github.com/adammcgrogan/ticketsbot/internal/store"
)

type panelInput struct {
	Title         string           `json:"title"`
	Description   string           `json:"description"`
	Color         int              `json:"color"`
	Style         store.PanelStyle `json:"style"`
	TicketTypeIDs []int64          `json:"ticket_type_ids"`
}

func (in panelInput) apply(p *store.Panel) {
	p.Title = in.Title
	p.Description = in.Description
	p.Color = in.Color
	p.Style = in.Style
	p.TicketTypeIDs = in.TicketTypeIDs
}

func (s *Server) validatePanel(ctx context.Context, guildID snowflake.ID, in *panelInput) error {
	in.Title = strings.TrimSpace(in.Title)
	if n := utf8.RuneCountInString(in.Title); n == 0 || n > 256 {
		return invalid("title", "Give the panel a title of up to 256 characters.")
	}
	in.Description = strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(in.Description) > 4000 {
		return invalid("description", "Keep the description to 4,000 characters or fewer.")
	}
	if in.Color < 0 || in.Color > 0xFFFFFF {
		return invalid("color", "Choose a valid colour.")
	}
	if in.Style == "" {
		in.Style = store.PanelButtons
	}
	if in.Style != store.PanelButtons && in.Style != store.PanelDropdown {
		return invalid("style", "Choose buttons or a dropdown.")
	}

	// De-duplicate while keeping the chosen order.
	seen := make(map[int64]bool, len(in.TicketTypeIDs))
	ids := make([]int64, 0, len(in.TicketTypeIDs))
	for _, id := range in.TicketTypeIDs {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	in.TicketTypeIDs = ids
	if len(ids) > panels.MaxTypesPerPanel {
		return invalid("ticket_type_ids", fmt.Sprintf("A panel can show up to %d ticket types.", panels.MaxTypesPerPanel))
	}
	types, err := s.store.ListTicketTypes(ctx, guildID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !slices.ContainsFunc(types, func(t store.TicketType) bool { return t.ID == id }) {
			return invalid("ticket_type_ids", "One of the selected ticket types no longer exists.")
		}
	}
	return nil
}

func (s *Server) listPanels(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListPanels(r.Context(), guildFrom(r).ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createPanel(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	var in panelInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.validatePanel(r.Context(), g.ID, &in); err != nil {
		s.writeFailure(w, err)
		return
	}
	count, err := s.store.CountPanels(r.Context(), g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	if max := s.limits(r.Context(), g.ID).MaxPanels; count >= max {
		s.writeFailure(w, invalid("", fmt.Sprintf("You can have up to %d panels.", max)))
		return
	}

	p := store.Panel{GuildID: g.ID}
	in.apply(&p)
	if err := s.store.CreatePanel(r.Context(), &p); err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// loadPanel fetches the panel in the URL, writing a 404 if it doesn't exist.
func (s *Server) loadPanel(w http.ResponseWriter, r *http.Request) (store.Panel, bool) {
	id, ok := pathID(r, "panelID")
	if !ok {
		writeError(w, http.StatusNotFound, "panel not found")
		return store.Panel{}, false
	}
	p, err := s.store.GetPanel(r.Context(), guildFrom(r).ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "panel not found")
		return p, false
	} else if err != nil {
		s.writeFailure(w, err)
		return p, false
	}
	return p, true
}

func (s *Server) updatePanel(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	p, ok := s.loadPanel(w, r)
	if !ok {
		return
	}
	var in panelInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.validatePanel(r.Context(), g.ID, &in); err != nil {
		s.writeFailure(w, err)
		return
	}
	in.apply(&p)
	if err := s.store.UpdatePanel(r.Context(), p); err != nil {
		s.writeFailure(w, err)
		return
	}

	warning := ""
	if p.MessageID != nil {
		warning = s.refreshPublished(r.Context(), g.ID, []store.Panel{p})
		// Re-read in case the message was found deleted and the panel unpublished.
		if fresh, err := s.store.GetPanel(r.Context(), g.ID, p.ID); err == nil {
			p = fresh
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"panel": p, "warning": warning})
}

func (s *Server) deletePanel(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadPanel(w, r)
	if !ok {
		return
	}
	if p.ChannelID != nil && p.MessageID != nil {
		err := s.discord.DeleteMessage(*p.ChannelID, *p.MessageID, rest.WithCtx(r.Context()))
		if err != nil && !discordx.IsCode(err, discordx.CodeUnknownMessage, discordx.CodeUnknownChannel) {
			s.log.Warn("delete panel message", slog.Any("err", err))
		}
	}
	if err := s.store.DeletePanel(r.Context(), p.GuildID, p.ID); err != nil {
		s.writeFailure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type publishInput struct {
	ChannelID snowflake.ID `json:"channel_id"`
}

// publishPanel posts the panel to a channel. Re-publishing to the same
// channel edits the existing message; moving channels posts a new message
// and removes the old one.
func (s *Server) publishPanel(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	p, ok := s.loadPanel(w, r)
	if !ok {
		return
	}
	var in publishInput
	if !decodeJSON(w, r, &in) {
		return
	}

	channels, err := s.guildChannels(r.Context(), g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	if !slices.ContainsFunc(channels, func(c channelResponse) bool {
		return c.ID == in.ChannelID && (c.Kind == "text" || c.Kind == "announcement")
	}) {
		s.writeFailure(w, invalid("channel_id", "Choose a text channel to post the panel in."))
		return
	}
	types, err := s.store.ListTicketTypes(r.Context(), g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	if len(panels.Ordered(p, types)) == 0 {
		s.writeFailure(w, invalid("ticket_type_ids", "Add at least one ticket type to the panel before publishing."))
		return
	}

	published := p.ChannelID != nil && p.MessageID != nil
	if published && *p.ChannelID == in.ChannelID {
		_, err := s.discord.UpdateMessage(in.ChannelID, *p.MessageID, panels.Update(s.cfg.AppName, p, types), rest.WithCtx(r.Context()))
		if err == nil {
			writeJSON(w, http.StatusOK, p)
			return
		}
		if !discordx.IsCode(err, discordx.CodeUnknownMessage) {
			s.writeFailure(w, err)
			return
		}
		// The old message was deleted; fall through and post a new one.
	}

	msg, err := s.discord.CreateMessage(in.ChannelID, panels.Create(s.cfg.AppName, p, types), rest.WithCtx(r.Context()))
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	if published && *p.ChannelID != in.ChannelID {
		if err := s.discord.DeleteMessage(*p.ChannelID, *p.MessageID, rest.WithCtx(r.Context())); err != nil &&
			!discordx.IsCode(err, discordx.CodeUnknownMessage, discordx.CodeUnknownChannel) {
			s.log.Warn("delete old panel message", slog.Any("err", err))
		}
	}

	channelID, messageID := in.ChannelID, msg.ID
	if err := s.store.SetPanelMessage(r.Context(), g.ID, p.ID, &channelID, &messageID); err != nil {
		s.writeFailure(w, err)
		return
	}
	p.ChannelID, p.MessageID = &channelID, &messageID
	writeJSON(w, http.StatusOK, p)
}
