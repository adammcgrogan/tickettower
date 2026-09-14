package api

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

var retentionOptions = []int{7, 30, 90, 180, 365}

const maxDashboardRoles = 10

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetGuildSettings(r.Context(), guildFrom(r).ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// updateSettings applies a partial update: fields missing from the body keep
// their current values.
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	cur, err := s.store.GetGuildSettings(r.Context(), g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	// Decoding writes through pointers and reuses slices, so give in its own
	// copies; otherwise cur would change too and hide what was edited.
	in := cur
	in.TranscriptRetentionDays = clonePtr(cur.TranscriptRetentionDays)
	in.DashboardRoles = slices.Clone(cur.DashboardRoles)
	in.LogChannelID = clonePtr(cur.LogChannelID)
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.validateSettings(r.Context(), g.ID, isManager(r), cur, &in); err != nil {
		s.writeFailure(w, err)
		return
	}
	if err := s.store.UpdateGuildSettings(r.Context(), g.ID, in); err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) validateSettings(ctx context.Context, guildID snowflake.ID, manager bool, cur store.GuildSettings, in *store.GuildSettings) error {
	if d := in.TranscriptRetentionDays; d != nil && !slices.Contains(retentionOptions, *d) {
		return invalid("transcript_retention_days", "Choose one of the listed retention periods.")
	}

	if in.LogChannelID != nil && *in.LogChannelID == 0 {
		in.LogChannelID = nil
	}
	// Only check a newly chosen channel, so a log channel deleted in Discord
	// doesn't block saving other settings.
	if in.LogChannelID != nil && (cur.LogChannelID == nil || *cur.LogChannelID != *in.LogChannelID) {
		channels, err := s.guildChannels(ctx, guildID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(channels, func(c channelResponse) bool {
			return c.ID == *in.LogChannelID && (c.Kind == "text" || c.Kind == "announcement")
		}) {
			return invalid("log_channel_id", "Choose a text channel for the ticket log.")
		}
	}

	in.DashboardRoles = normaliseDashboardRoles(in.DashboardRoles)
	// Otherwise anyone with a dashboard role could hand out access.
	if !manager && !slices.Equal(in.DashboardRoles, normaliseDashboardRoles(cur.DashboardRoles)) {
		return invalid("dashboard_roles", "Only people with Manage Server can change who has dashboard access.")
	}
	if len(in.DashboardRoles) > maxDashboardRoles {
		return invalid("dashboard_roles", fmt.Sprintf("Choose up to %d roles.", maxDashboardRoles))
	}
	for _, r := range in.DashboardRoles {
		if !r.Level.Valid() {
			return invalid("dashboard_roles", "Choose viewer, support or admin for each role.")
		}
	}
	if len(in.DashboardRoles) > 0 {
		roles, err := s.guildRoles(ctx, guildID)
		if err != nil {
			return err
		}
		// Quietly drop roles that have since been deleted in Discord.
		in.DashboardRoles = slices.DeleteFunc(in.DashboardRoles, func(dr store.DashboardRole) bool {
			return !slices.ContainsFunc(roles, func(r roleResponse) bool { return r.ID == dr.RoleID })
		})
	}
	return nil
}

// normaliseDashboardRoles sorts roles by ID, keeping one entry per role (the
// highest level wins), and returns an empty (not nil) slice.
func normaliseDashboardRoles(in []store.DashboardRole) []store.DashboardRole {
	best := map[snowflake.ID]store.AccessLevel{}
	for _, r := range in {
		if r.RoleID == 0 {
			continue
		}
		if cur, ok := best[r.RoleID]; !ok || r.Level.AtLeast(cur) {
			best[r.RoleID] = r.Level
		}
	}
	out := make([]store.DashboardRole, 0, len(best))
	for id, level := range best {
		out = append(out, store.DashboardRole{RoleID: id, Level: level})
	}
	slices.SortFunc(out, func(a, b store.DashboardRole) int { return cmp.Compare(a.RoleID, b.RoleID) })
	return out
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// deleteGuildData wipes everything stored for the server: tickets and
// transcripts, ticket types, ticket buttons (their messages are removed from
// Discord too), saved replies, blocks and settings. Server managers only, and
// only once every ticket is closed, since the bot is still serving open ones.
func (s *Server) deleteGuildData(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	if !isManager(r) {
		writeError(w, http.StatusForbidden, "Only server managers can delete the server's data.")
		return
	}
	panels, err := s.store.ListPanels(r.Context(), g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	var open *store.ErrOpenTickets
	if err := s.store.DeleteGuildData(r.Context(), g.ID); errors.As(err, &open) {
		writeError(w, http.StatusConflict, openTicketsBeforeDeleteMessage(open.Count))
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	for _, p := range panels {
		if p.ChannelID == nil || p.MessageID == nil {
			continue
		}
		err := s.discord.DeleteMessage(*p.ChannelID, *p.MessageID, rest.WithCtx(r.Context()))
		if err != nil && !discordx.IsCode(err, discordx.CodeUnknownMessage, discordx.CodeUnknownChannel) {
			s.log.Warn("delete panel message", slog.Any("err", err))
		}
	}
	s.log.Info("deleted server data", slog.String("guild_id", g.ID.String()), slog.String("user_id", auth.FromContext(r.Context()).User.ID.String()))
	w.WriteHeader(http.StatusNoContent)
}

func openTicketsBeforeDeleteMessage(n int) string {
	if n == 1 {
		return "There is still 1 open ticket. Close it first, then delete the server's data."
	}
	return fmt.Sprintf("There are still %d open tickets. Close them first, then delete the server's data.", n)
}
