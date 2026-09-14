package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// blockDurations maps a duration option to how long the block lasts. Matches
// blockDurations in the bot and blockDurationOptions in the web app; "" means
// the block never expires.
var blockDurations = map[string]time.Duration{
	"":    0,
	"1h":  time.Hour,
	"1d":  24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

type blockInput struct {
	UserID   snowflake.ID `json:"user_id"`
	Reason   string       `json:"reason"`
	Duration string       `json:"duration"`
}

func (s *Server) listBlocks(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListBlocks(r.Context(), guildFrom(r).ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// createBlock blocks a member by user ID. The member has to be in the server,
// which also gives us their current name for the list.
func (s *Server) createBlock(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	var in blockInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > store.MaxBlockReason {
		s.writeFailure(w, invalid("reason", fmt.Sprintf("Keep the reason to %d characters or fewer.", store.MaxBlockReason)))
		return
	}
	if in.UserID == 0 {
		s.writeFailure(w, invalid("user_id", "Paste the member's user ID. In Discord, right-click them and choose Copy User ID."))
		return
	}
	duration, ok := blockDurations[in.Duration]
	if !ok {
		s.writeFailure(w, invalid("duration", "Choose one of the listed durations."))
		return
	}
	user := auth.FromContext(r.Context()).User
	if in.UserID == user.ID {
		s.writeFailure(w, invalid("user_id", "You can't block yourself."))
		return
	}
	member, err := s.discord.GetMember(g.ID, in.UserID, rest.WithCtx(r.Context()))
	if discordx.IsCode(err, discordx.CodeUnknownMember, discordx.CodeUnknownUser) {
		s.writeFailure(w, invalid("user_id", "Nobody in this server has that user ID."))
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	if member.User.Bot {
		s.writeFailure(w, invalid("user_id", "Bots can't open tickets anyway."))
		return
	}
	if s.managesGuild(r, g.ID, *member) {
		s.writeFailure(w, invalid("user_id", fmt.Sprintf("%s manages this server, so they can't be blocked.", member.EffectiveName())))
		return
	}

	block := store.Block{
		GuildID: g.ID, UserID: in.UserID, UserName: member.EffectiveName(), Reason: in.Reason,
		BlockedBy: user.ID, BlockedByName: user.DisplayName,
	}
	if duration > 0 {
		until := time.Now().Add(duration)
		block.ExpiresAt = &until
	}
	if err := s.store.BlockMember(r.Context(), &block); err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, block)
}

func (s *Server) deleteBlock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "userID")
	if !ok {
		writeError(w, http.StatusNotFound, "block not found")
		return
	}
	err := s.store.UnblockMember(r.Context(), guildFrom(r).ID, snowflake.ID(id))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "block not found")
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// managesGuild reports whether a member owns the server or has a role with
// Administrator or Manage Server, from the guild's role list.
func (s *Server) managesGuild(r *http.Request, guildID snowflake.ID, m discord.Member) bool {
	if g, err := s.store.GetGuild(r.Context(), guildID); err == nil && g.OwnerID == m.User.ID {
		return true
	}
	roles, err := s.rawRoles(r.Context(), guildID)
	if err != nil {
		return false
	}
	for _, role := range roles {
		if role.ID != guildID && !containsID(m.RoleIDs, role.ID) {
			continue
		}
		if role.Permissions.Has(discord.PermissionAdministrator) || role.Permissions.Has(discord.PermissionManageGuild) {
			return true
		}
	}
	return false
}

func containsID(ids []snowflake.ID, id snowflake.ID) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
