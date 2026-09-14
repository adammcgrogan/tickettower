package api

import (
	"net/http"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/entitlements"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// isSuperadmin reports whether the session belongs to the configured
// SUPERADMIN_USER_ID. There's exactly one: the bot's owner.
func (s *Server) isSuperadmin(r *http.Request) bool {
	id := s.cfg.SuperadminUserID
	return id != 0 && auth.FromContext(r.Context()).User.ID == id
}

// requireSuperadmin protects the /admin API from everyone but the owner. A
// non-owner gets a 404 rather than a 403, so the panel's existence isn't
// advertised to other logged-in users.
func (s *Server) requireSuperadmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isSuperadmin(r) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// getAdminOverview returns install, plan and usage totals across every
// guild, for the superadmin panel's headline tiles.
func (s *Server) getAdminOverview(w http.ResponseWriter, r *http.Request) {
	o, err := s.store.AdminOverview(r.Context())
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

type adminGuildResponse struct {
	store.AdminGuild
	IconURL *string `json:"icon_url"`
}

// listAdminGuilds lists every guild the bot has ever joined, for the
// superadmin panel's install list.
func (s *Server) listAdminGuilds(w http.ResponseWriter, r *http.Request) {
	guilds, err := s.store.AdminGuilds(r.Context())
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	out := make([]adminGuildResponse, len(guilds))
	for i, g := range guilds {
		out[i] = adminGuildResponse{g, discord.Guild{ID: g.ID, Icon: g.Icon}.IconURL()}
	}
	writeJSON(w, http.StatusOK, out)
}

type setTierInput struct {
	Tier string `json:"tier"`
}

// setAdminGuildTier moves a guild between plans by hand. There's no billing
// wired up yet (see issue #77), so this is how a guild gets comped premium.
func (s *Server) setAdminGuildTier(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "guildID")
	if !ok {
		writeError(w, http.StatusNotFound, "guild not found")
		return
	}
	var in setTierInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Tier != entitlements.TierFree && in.Tier != entitlements.TierPremium {
		s.writeFailure(w, invalid("tier", "Tier must be \"free\" or \"premium\"."))
		return
	}
	if err := s.store.SetGuildTier(r.Context(), snowflake.ID(id), in.Tier); err != nil {
		s.writeFailure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
