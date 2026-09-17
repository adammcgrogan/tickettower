package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/entitlements"
	"github.com/adammcgrogan/tickettower/internal/panels"
	"github.com/adammcgrogan/tickettower/internal/store"
)

const discordCacheTTL = 15 * time.Second

type channelResponse struct {
	ID       snowflake.ID  `json:"id"`
	Name     string        `json:"name"`
	Kind     string        `json:"kind"` // "text", "announcement" or "category"
	ParentID *snowflake.ID `json:"parent_id"`
	Position int           `json:"position"`
}

type roleResponse struct {
	ID       snowflake.ID `json:"id"`
	Name     string       `json:"name"`
	Color    int          `json:"color"`
	Position int          `json:"position"`
}

// rawChannels returns all of the guild's channels, permission overwrites
// included.
func (s *Server) rawChannels(ctx context.Context, guildID snowflake.ID) ([]discord.GuildChannel, error) {
	key := "channels:" + guildID.String()
	if v, ok := s.cache.get(key); ok {
		return v.([]discord.GuildChannel), nil
	}
	channels, err := s.discord.GetGuildChannels(guildID, rest.WithCtx(ctx))
	if err != nil {
		return nil, err
	}
	s.cache.set(key, channels, discordCacheTTL)
	return channels, nil
}

// rawRoles returns all of the guild's roles, permissions included.
func (s *Server) rawRoles(ctx context.Context, guildID snowflake.ID) ([]discord.Role, error) {
	key := "roles:" + guildID.String()
	if v, ok := s.cache.get(key); ok {
		return v.([]discord.Role), nil
	}
	roles, err := s.discord.GetRoles(guildID, rest.WithCtx(ctx))
	if err != nil {
		return nil, err
	}
	s.cache.set(key, roles, discordCacheTTL)
	return roles, nil
}

// guildEmojis returns the guild's custom emoji.
func (s *Server) guildEmojis(ctx context.Context, guildID snowflake.ID) ([]discord.Emoji, error) {
	key := "emojis:" + guildID.String()
	if v, ok := s.cache.get(key); ok {
		return v.([]discord.Emoji), nil
	}
	emojis, err := s.discord.GetEmojis(guildID, rest.WithCtx(ctx))
	if err != nil {
		return nil, err
	}
	s.cache.set(key, emojis, discordCacheTTL)
	return emojis, nil
}

// guildChannels returns the guild's text, announcement and category channels.
func (s *Server) guildChannels(ctx context.Context, guildID snowflake.ID) ([]channelResponse, error) {
	channels, err := s.rawChannels(ctx, guildID)
	if err != nil {
		return nil, err
	}
	out := make([]channelResponse, 0, len(channels))
	for _, c := range channels {
		var kind string
		switch c.Type() {
		case discord.ChannelTypeGuildText:
			kind = "text"
		case discord.ChannelTypeGuildNews:
			kind = "announcement"
		case discord.ChannelTypeGuildCategory:
			kind = "category"
		default:
			continue
		}
		out = append(out, channelResponse{ID: c.ID(), Name: c.Name(), Kind: kind, ParentID: c.ParentID(), Position: c.Position()})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

// guildRoles returns roles that can be assigned as support roles, highest
// first. @everyone and integration-managed roles are excluded.
func (s *Server) guildRoles(ctx context.Context, guildID snowflake.ID) ([]roleResponse, error) {
	roles, err := s.rawRoles(ctx, guildID)
	if err != nil {
		return nil, err
	}
	out := make([]roleResponse, 0, len(roles))
	for _, r := range roles {
		if r.ID == guildID || r.Managed {
			continue
		}
		out = append(out, roleResponse{ID: r.ID, Name: r.Name, Color: r.Color, Position: r.Position})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position > out[j].Position })
	return out, nil
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := s.guildChannels(r.Context(), guildFrom(r).ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.guildRoles(r.Context(), guildFrom(r).ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (s *Server) limits(ctx context.Context, guildID snowflake.ID) entitlements.Limits {
	tier, err := s.store.GuildTier(ctx, guildID)
	if err != nil {
		s.log.Error("load guild tier", slog.Any("err", err))
	}
	return entitlements.ForTier(tier)
}

// panelFooter is the "Powered by" line for a guild's panels, or "" when its
// plan removes branding.
func (s *Server) panelFooter(ctx context.Context, guildID snowflake.ID) string {
	if !s.limits(ctx, guildID).Branding {
		return ""
	}
	return panels.Footer(s.cfg.AppName, s.cfg.PublicURL)
}

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	stats, err := s.store.TicketStats(r.Context(), g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	tier, err := s.store.GuildTier(r.Context(), g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	_, err = s.store.GuildBillingCustomer(r.Context(), g.ID)
	hasBillingCustomer := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tickets":              stats,
		"tier":                 tier,
		"limits":               entitlements.ForTier(tier),
		"has_billing_customer": hasBillingCustomer,
		"billing_enabled":      s.cfg.BillingEnabled(),
		"review_url":           s.cfg.ReviewURL(),
	})
}

func (s *Server) getAnalytics(w http.ResponseWriter, r *http.Request) {
	q := store.AnalyticsQuery{Days: 30, Timezone: r.URL.Query().Get("tz")}
	switch r.URL.Query().Get("days") {
	case "7":
		q.Days = 7
	case "90":
		q.Days = 90
	case "365":
		q.Days = 365
	case "all":
		q.Days = 0
	}
	if v, err := strconv.ParseInt(r.URL.Query().Get("type"), 10, 64); err == nil {
		q.TypeID = &v
	}
	a, err := s.store.Analytics(r.Context(), guildFrom(r).ID, q)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// maxTicketPage is the most tickets one request returns.
const maxTicketPage = 200

// ticketQuery reads the ticket list's filters: status (open or closed), type
// (a ticket type ID), q (search), before (a ticket ID to page past) and limit.
func ticketQuery(v url.Values) store.TicketQuery {
	q := store.TicketQuery{Status: store.TicketStatus(v.Get("status")), Limit: maxTicketPage}
	if q.Status != store.StatusOpen && q.Status != store.StatusClosed {
		q.Status = ""
	}
	if id, err := strconv.ParseInt(v.Get("type"), 10, 64); err == nil {
		q.TypeID = &id
	}
	if id, err := strconv.ParseInt(v.Get("before"), 10, 64); err == nil {
		q.Before = &id
	}
	if n, err := strconv.Atoi(v.Get("limit")); err == nil && n > 0 && n < maxTicketPage {
		q.Limit = n
	}
	// Ticket numbers are shown as "#0042", so that finds ticket 42.
	search := strings.TrimSpace(v.Get("q"))
	if rest, ok := strings.CutPrefix(search, "#"); ok {
		search = strings.TrimLeft(rest, "0")
	}
	if r := []rune(search); len(r) > 100 {
		search = string(r[:100])
	}
	q.Search = search
	return q
}

func (s *Server) listTickets(w http.ResponseWriter, r *http.Request) {
	tickets, err := s.store.ListTickets(r.Context(), guildFrom(r).ID, ticketQuery(r.URL.Query()))
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tickets)
}
