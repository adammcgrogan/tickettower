// Package api serves the dashboard's REST API and static frontend.
package api

import (
	"context"
	"errors"
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
	"github.com/adammcgrogan/tickettower/internal/ticketbot"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	auth    *auth.Manager
	discord rest.Rest // authenticated as the bot
	// dashboard closes and replies to tickets as the bot does in Discord.
	dashboard *ticketbot.Dashboard
	stripe    stripeClient
	cache     *ttlCache
	log       *slog.Logger
}

func NewServer(cfg config.Config, st *store.Store, am *auth.Manager, discordRest rest.Rest, log *slog.Logger) *Server {
	return &Server{
		cfg:       cfg,
		store:     st,
		auth:      am,
		discord:   discordRest,
		dashboard: ticketbot.NewDashboard(cfg, st, discordRest, log),
		stripe:    newStripeClient(cfg),
		cache:     newTTLCache(),
		log:       log,
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(clientIP(s.cfg.TrustedProxies), middleware.Recoverer, securityHeaders)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	// Stripe calls this directly, not a browser, so it sits outside the
	// /api group: no session cookie, and no requireJSON (it needs the raw
	// body to verify Stripe's signature before anything else touches it).
	r.Post("/stripe/webhook", s.stripeWebhook)

	r.Route("/api", func(r chi.Router) {
		r.Use(requireJSON)
		r.Get("/config", s.getConfig)
		r.Get("/invite", s.invite)
		r.Group(func(r chi.Router) {
			r.Use(newRateLimiter(authLimit, authWindow).middleware(byIP))
			r.Get("/auth/login", s.login)
			r.Get("/auth/callback", s.callback)
			r.Post("/auth/logout", s.logout)
		})

		r.Group(func(r chi.Router) {
			r.Use(s.auth.Middleware, newRateLimiter(userLimit, userWindow).middleware(byUser))
			r.Get("/me", s.getMe)
			r.Get("/guilds", s.listGuilds)
			r.Get("/transcripts/{ticketID}", s.getTranscript)
			r.Get("/transcripts/{ticketID}/download", s.downloadTranscript)

			r.Route("/admin", func(r chi.Router) {
				r.Use(s.requireSuperadmin)
				r.Get("/overview", s.getAdminOverview)
				r.Get("/guilds", s.listAdminGuilds)
				r.Post("/guilds/{guildID}/tier", s.setAdminGuildTier)
			})
			r.Route("/guilds/{guildID}", func(r chi.Router) {
				r.Use(s.requireGuild)
				r.Get("/", s.getGuild)
				r.Get("/channels", s.listChannels)
				r.Get("/roles", s.listRoles)
				r.Get("/stats", s.getStats)
				r.Get("/setup-check", s.getSetupCheck)
				r.Get("/analytics", s.getAnalytics)
				r.Get("/tickets", s.listTickets)
				r.Get("/settings", s.getSettings)
				r.Get("/ticket-types", s.listTicketTypes)
				r.Get("/panels", s.listPanels)
				r.Get("/blocks", s.listBlocks)
				r.Get("/saved-replies", s.listSavedReplies)

				// Acting on tickets needs support access.
				r.Group(func(r chi.Router) {
					r.Use(requireLevel(store.LevelSupport))
					r.Get("/tickets/{ticketID}/assignees", s.listAssignees)
					r.Get("/members", s.searchMembers)
					r.Get("/tickets/{ticketID}/members", s.listTicketMembers)
					r.Post("/tickets/{ticketID}/members", s.addTicketMember)
					r.Delete("/tickets/{ticketID}/members/{userID}", s.removeTicketMember)
					r.Post("/tickets/{ticketID}/rename", s.renameTicket)
					r.Post("/tickets/{ticketID}/claim", s.claimTicket)
					r.Post("/tickets/{ticketID}/unclaim", s.unclaimTicket)
					r.Post("/tickets/close", s.closeTickets)
					r.Post("/tickets/{ticketID}/close", s.closeTicket)
					r.Post("/tickets/{ticketID}/reply", s.replyTicket)
					r.Post("/tickets/{ticketID}/notes", s.addTicketNote)
					r.Post("/tickets/{ticketID}/move", s.moveTicket)
					r.Post("/tickets/{ticketID}/reopen", s.reopenTicket)
					r.Post("/tickets/{ticketID}/hold", s.holdTicket)
					r.Post("/tickets/{ticketID}/resume", s.resumeTicket)
					r.Post("/blocks", s.createBlock)
					r.Delete("/blocks/{userID}", s.deleteBlock)
				})

				// Changing the setup needs admin access. Dashboard roles
				// themselves are owner only (checked in validateSettings).
				r.Group(func(r chi.Router) {
					r.Use(requireLevel(store.LevelAdmin))
					r.Patch("/settings", s.updateSettings)

					r.Post("/billing/checkout", s.createCheckoutSession)
					r.Post("/billing/portal", s.createPortalSession)

					r.Post("/ticket-types", s.createTicketType)
					r.Patch("/ticket-types/{typeID}", s.updateTicketType)
					r.Delete("/ticket-types/{typeID}", s.deleteTicketType)

					r.Post("/panels", s.createPanel)
					r.Patch("/panels/{panelID}", s.updatePanel)
					r.Delete("/panels/{panelID}", s.deletePanel)
					r.Post("/panels/{panelID}/publish", s.publishPanel)

					r.Post("/saved-replies", s.createSavedReply)
					r.Patch("/saved-replies/{replyID}", s.updateSavedReply)
					r.Delete("/saved-replies/{replyID}", s.deleteSavedReply)
				})

				// Wiping the server's data is for managers only (checked in
				// the handler, since it's stricter than any dashboard role).
				r.Delete("/data", s.deleteGuildData)
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
		"app_name":        s.cfg.AppName,
		"client_id":       s.cfg.DiscordClientID,
		"billing_enabled": s.cfg.BillingEnabled(),
	})
}

// invite redirects to Discord's bot authorization page, optionally
// preselecting a guild.
func (s *Server) invite(w http.ResponseWriter, r *http.Request) {
	values := discord.QueryValues{
		"client_id": s.cfg.DiscordClientID,
		"scope":     "bot applications.commands",
		// Discord wants the bitfield as a number; Permissions' String() lists names.
		"permissions": int64(config.BotPermissions),
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

// login starts a Discord login. ?next= is a dashboard path to return to
// afterwards, such as the transcript someone was trying to open.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	if !isLocalPath(next) {
		next = ""
	}
	http.Redirect(w, r, s.auth.LoginURL(r.Context(), next), http.StatusFound)
}

// isLocalPath reports whether p is a path on this site, so redirecting to it
// can't send anyone elsewhere ("//host" and "/\host" are other sites to a
// browser).
func isLocalPath(p string) bool {
	return strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && !strings.HasPrefix(p, "/api/") &&
		!strings.ContainsAny(p, "\\\r\n")
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("error") != "" {
		// The user cancelled the Discord consent screen.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	next := s.auth.Next(r.Context(), q.Get("state"))
	if _, err := s.auth.Complete(r.Context(), w, q.Get("code"), q.Get("state")); err != nil {
		s.log.Warn("login failed", slog.Any("err", err))
		http.Redirect(w, r, "/?error=login_failed", http.StatusFound)
		return
	}
	if !isLocalPath(next) {
		next = "/servers"
	}
	http.Redirect(w, r, next, http.StatusFound)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.Logout(w, r); err != nil {
		s.log.Error("logout failed", slog.Any("err", err))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		auth.User
		IsSuperadmin bool `json:"is_superadmin"`
	}{auth.FromContext(r.Context()).User, s.isSuperadmin(r)})
}

type guildResponse struct {
	ID         snowflake.ID `json:"id"`
	Name       string       `json:"name"`
	IconURL    *string      `json:"icon_url"`
	BotPresent bool         `json:"bot_present"`
	// CanManage is false for members who only have a dashboard role.
	CanManage bool `json:"can_manage"`
	// Level is what the user may do here: viewer, support, admin or owner.
	Level store.AccessLevel `json:"level"`
}

// listGuilds returns the servers the user can use the dashboard for,
// flagging which ones already have the bot. Servers without the bot are only
// listed for managers, who can add it.
func (s *Server) listGuilds(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	guilds, err := s.auth.Guilds(r.Context(), sess)
	if errors.Is(err, auth.ErrNoSession) {
		// The Discord login behind the session is over; the dashboard
		// sends the user back to log in.
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	} else if err != nil {
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
		level, err := accessLevel(g, dashboardRoles[g.ID], func() ([]snowflake.ID, error) {
			return s.memberRoles(r.Context(), g.ID, sess.User.ID)
		})
		if err != nil {
			// Leave the server out rather than failing the whole list.
			s.log.Warn("check dashboard role", slog.String("guild_id", g.ID.String()), slog.Any("err", err))
			continue
		}
		if level != "" {
			out = append(out, guildResponse{ID: g.ID, Name: g.Name, IconURL: g.IconURL(), BotPresent: active[g.ID],
				CanManage: level == store.LevelOwner, Level: level})
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
		if errors.Is(err, auth.ErrNoSession) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		} else if err != nil {
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

// levelOf is the user's access level in the request's guild.
func levelOf(r *http.Request) store.AccessLevel {
	return r.Context().Value(guildCtxKey{}).(guildAccess).level
}

// isManager reports whether the user has Manage Server (or more) in the
// request's guild, as opposed to only a dashboard role.
func isManager(r *http.Request) bool { return levelOf(r) == store.LevelOwner }

// requireLevel rejects requests from users below min with a 403. Routes
// without it only need dashboard access (viewer).
func requireLevel(min store.AccessLevel) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !levelOf(r).AtLeast(min) {
				writeError(w, http.StatusForbidden, levelMessage(min))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func levelMessage(min store.AccessLevel) string {
	if min == store.LevelAdmin {
		return "Only dashboard admins can change the setup. Ask a server manager for admin access."
	}
	return "Your dashboard access is read only. Ask a server manager for support access to act on tickets."
}

func (s *Server) getGuild(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	level := levelOf(r)
	writeJSON(w, http.StatusOK, guildResponse{ID: g.ID, Name: g.Name, IconURL: g.IconURL(), BotPresent: true,
		CanManage: level == store.LevelOwner, Level: level})
}

// clientIP records where a request really came from, for the per-IP rate
// limit. Behind trusted proxies it is the X-Forwarded-For entry those proxies
// added; otherwise the connection's address. Client-supplied headers such as
// X-Real-IP are never trusted (chi's RealIP was, and is deprecated for it).
func clientIP(trustedProxies int) func(http.Handler) http.Handler {
	if trustedProxies > 0 {
		return middleware.ClientIPFromXFFTrustedProxies(trustedProxies)
	}
	return middleware.ClientIPFromRemoteAddr
}

// securityHeaders stops the dashboard being framed by another site (its
// actions are one click), sniffed or leaking URLs. The SPA's own
// Content-Security-Policy is a <meta> tag written at build time (see
// web/vite.config.ts), since it needs the bootstrap script's hash;
// frame-ancestors only works as a header, so it's set here.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// spaHandler serves the built frontend, falling back to index.html so
// client-side routes work on refresh. Hashed assets are cached for good;
// everything else (index.html above all, which names those assets) must be
// revalidated on every load, or a deploy leaves browsers with a stale shell
// pointing at chunks that no longer exist.
func spaHandler(dir string) http.Handler {
	fileServer := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
