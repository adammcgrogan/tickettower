package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/disgoorg/snowflake/v2"

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
	in.DashboardRoleIDs = slices.Clone(cur.DashboardRoleIDs)
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

	in.DashboardRoleIDs = normaliseIDs(in.DashboardRoleIDs)
	// Otherwise anyone with a dashboard role could hand out access.
	if !manager && !slices.Equal(in.DashboardRoleIDs, normaliseIDs(cur.DashboardRoleIDs)) {
		return invalid("dashboard_role_ids", "Only people with Manage Server can change who has dashboard access.")
	}
	if len(in.DashboardRoleIDs) > maxDashboardRoles {
		return invalid("dashboard_role_ids", fmt.Sprintf("Choose up to %d roles.", maxDashboardRoles))
	}
	if len(in.DashboardRoleIDs) > 0 {
		roles, err := s.guildRoles(ctx, guildID)
		if err != nil {
			return err
		}
		// Quietly drop roles that have since been deleted in Discord.
		in.DashboardRoleIDs = slices.DeleteFunc(in.DashboardRoleIDs, func(id snowflake.ID) bool {
			return !slices.ContainsFunc(roles, func(r roleResponse) bool { return r.ID == id })
		})
	}
	return nil
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
