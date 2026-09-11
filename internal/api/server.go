// Package api serves the dashboard's REST API and static frontend.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/config"
	"github.com/adammcgrogan/tickettower/internal/store"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	auth    *auth.Manager
	discord rest.Rest // authenticated as the bot
	cache   *ttlCache
	log     *slog.Logger
}

func NewServer(cfg config.Config, st *store.Store, am *auth.Manager, discordRest rest.Rest, log *slog.Logger) *Server {
	return &Server{cfg: cfg, store: st, auth: am, discord: discordRest, cache: newTTLCache(), log: log}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP, middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	r.Route("/api", func(r chi.Router) {
		r.Use(requireJSON)
		r.Get("/config", s.getConfig)
		r.Get("/invite", s.invite)
		r.Get("/auth/login", s.login)
		r.Get("/auth/callback", s.callback)
		r.Post("/auth/logout", s.logout)

		r.Group(func(r chi.Router) {
			r.Use(s.auth.Middleware)
			r.Get("/me", s.getMe)
			r.Get("/guilds", s.listGuilds)
			r.Get("/transcripts/{ticketID}", s.getTranscript)
			r.Route("/guilds/{guildID}", func(r chi.Router) {
				r.Use(s.requireGuild)
				r.Get("/", s.getGuild)
				r.Get("/channels", s.listChannels)
				r.Get("/roles", s.listRoles)
				r.Get("/stats", s.getStats)
				r.Get("/analytics", s.getAnalytics)
				r.Get("/tickets", s.listTickets)
				r.Get("/settings", s.getSettings)
				r.Patch("/settings", s.updateSettings)

				r.Get("/ticket-types", s.listTicketTypes)
				r.Post("/ticket-types", s.createTicketType)
				r.Patch("/ticket-types/{typeID}", s.updateTicketType)
				r.Delete("/ticket-types/{typeID}", s.deleteTicketType)

				r.Get("/panels", s.listPanels)
				r.Post("/panels", s.createPanel)
				r.Patch("/panels/{panelID}", s.updatePanel)
				r.Delete("/panels/{panelID}", s.deletePanel)
				r.Post("/panels/{panelID}/publish", s.publishPanel)
			})
		})

		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "not found")
		})
	})

	r.Handle("/*", spaHandler(s.cfg.StaticDir))
	return r
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"app_name":  s.cfg.AppName,
		"client_id": s.cfg.DiscordClientID,
	})
}

// invite redirects to Discord's bot authorization page, optionally
// preselecting a guild.
func (s *Server) invite(w http.ResponseWriter, r *http.Request) {
	values := discord.QueryValues{
		"client_id":   s.cfg.DiscordClientID,
		"scope":       "bot applications.commands",
		"permissions": config.BotPermissions,
	}
	if id, err := snowflake.Parse(r.URL.Query().Get("guild_id")); err == nil {
		values["guild_id"] = id
		values["disable_guild_select"] = true
	}
	if sess, err := s.auth.FromRequest(r); err == nil {
		s.auth.InvalidateGuilds(r.Context(), sess.User.ID)
	}
	http.Redirect(w, r, discord.AuthorizeURL(values), http.StatusFound)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, s.auth.LoginURL(), http.StatusFound)
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("error") != "" {
		// The user cancelled the Discord consent screen.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if _, err := s.auth.Complete(r.Context(), w, q.Get("code"), q.Get("state")); err != nil {
		s.log.Warn("login failed", slog.Any("err", err))
		http.Redirect(w, r, "/?error=login_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/servers", http.StatusFound)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.Logout(w, r); err != nil {
		s.log.Error("logout failed", slog.Any("err", err))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, auth.FromContext(r.Context()).User)
}

type guildResponse struct {
	ID         snowflake.ID `json:"id"`
	Name       string       `json:"name"`
	IconURL    *string      `json:"icon_url"`
	BotPresent bool         `json:"bot_present"`
	// CanManage is false for members who only have a dashboard role.
	CanManage bool `json:"can_manage"`
}

// listGuilds returns the servers the user can use the dashboard for,
// flagging which ones already have the bot. Servers without the bot are only
// listed for managers, who can add it.
func (s *Server) listGuilds(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	guilds, err := s.auth.Guilds(r.Context(), sess)
	if err != nil {
		s.log.Error("fetch guilds", slog.Any("err", err))
		writeError(w, http.StatusBadGateway, "could not load your servers from Discord")
		return
	}

	ids := make([]snowflake.ID, len(guilds))
	for i, g := range guilds {
		ids[i] = g.ID
	}
	active, err := s.store.ActiveGuilds(r.Context(), ids)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	// Only servers with the bot can have dashboard roles.
	var others []snowflake.ID
	for _, g := range guilds {
		if !canManage(g) && active[g.ID] {
			others = append(others, g.ID)
		}
	}
	dashboardRoles, err := s.store.DashboardRoles(r.Context(), others)
	if err != nil {
		s.writeFailure(w, err)
		return
	}

	out := []guildResponse{}
	for _, g := range guilds {
		allowed, manager, err := canAccess(g, dashboardRoles[g.ID], func() ([]snowflake.ID, error) {
			return s.memberRoles(r.Context(), g.ID, sess.User.ID)
		})
		if err != nil {
			// Leave the server out rather than failing the whole list.
			s.log.Warn("check dashboard role", slog.String("guild_id", g.ID.String()), slog.Any("err", err))
			continue
		}
		if allowed {
			out = append(out, guildResponse{ID: g.ID, Name: g.Name, IconURL: g.IconURL(), BotPresent: active[g.ID], CanManage: manager})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type guildCtxKey struct{}

// requireGuild ensures the user can use the dashboard for the guild in the
// URL and that the bot is in it.
func (s *Server) requireGuild(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := snowflake.Parse(chi.URLParam(r, "guildID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid server id")
			return
		}
		acc, allowed, err := s.access(r.Context(), auth.FromContext(r.Context()), id)
		if err != nil {
			s.log.Warn("check guild access", slog.Any("err", err))
			writeError(w, http.StatusBadGateway, "could not check your access to this server with Discord")
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "you do not have access to this server")
			return
		}
		active, err := s.store.ActiveGuilds(r.Context(), []snowflake.ID{id})
		if err != nil {
			s.writeFailure(w, err)
			return
		}
		if !active[id] {
			writeError(w, http.StatusNotFound, "the bot is not in this server")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), guildCtxKey{}, acc)))
	})
}

func guildFrom(r *http.Request) discord.OAuth2Guild {
	return r.Context().Value(guildCtxKey{}).(guildAccess).guild
}

// isManager reports whether the user has Manage Server (or more) in the
// request's guild, as opposed to only a dashboard role.
func isManager(r *http.Request) bool {
	return r.Context().Value(guildCtxKey{}).(guildAccess).manager
}

func (s *Server) getGuild(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	writeJSON(w, http.StatusOK, guildResponse{ID: g.ID, Name: g.Name, IconURL: g.IconURL(), BotPresent: true, CanManage: isManager(r)})
}

// spaHandler serves the built frontend, falling back to index.html so
// client-side routes work on refresh.
func spaHandler(dir string) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
