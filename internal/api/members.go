package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/go-chi/chi/v5"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
	"github.com/adammcgrogan/tickettower/internal/ticketbot"
)

// maxMemberResults caps the member search, which is a type-ahead.
const maxMemberResults = 10

// searchMembers finds the server's members by name, or by a pasted user ID,
// to add them to a ticket. Bots are left out.
func (s *Server) searchMembers(w http.ResponseWriter, r *http.Request) {
	guildID := guildFrom(r).ID
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	out := []memberResponse{}
	if q == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	var members []discord.Member
	if id, ok := userIDQuery(q); ok {
		m, err := s.discord.GetMember(guildID, id, rest.WithCtx(r.Context()))
		if err == nil {
			members = append(members, *m)
		} else if !discordx.IsCode(err, discordx.CodeUnknownMember, discordx.CodeUnknownUser) {
			s.writeFailure(w, err)
			return
		}
	} else {
		var err error
		// Discord matches name prefixes; ask for a few spare in case of bots.
		if members, err = s.discord.SearchMembers(guildID, q, 2*maxMemberResults, rest.WithCtx(r.Context())); err != nil {
			s.writeFailure(w, err)
			return
		}
	}
	for _, m := range members {
		if m.User.Bot {
			continue
		}
		out = append(out, memberResponse{ID: m.User.ID, Name: m.EffectiveName(), AvatarURL: m.EffectiveAvatarURL()})
		if len(out) == maxMemberResults {
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// userIDQuery reports whether a search is a pasted user ID rather than a
// name. Discord's IDs are 17 to 20 digits.
func userIDQuery(q string) (snowflake.ID, bool) {
	if len(q) < 17 || len(q) > 20 {
		return 0, false
	}
	id, err := snowflake.Parse(q)
	return id, err == nil
}

// listTicketMembers lists who has their own access to an open ticket, so
// people can be removed from it.
func (s *Server) listTicketMembers(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	if t.Status != store.StatusOpen {
		s.writeFailure(w, invalid("", "This ticket is closed."))
		return
	}
	members, err := s.dashboard.Members(r.Context(), t)
	if s.actionFailed(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, members)
}

type memberInput struct {
	UserID snowflake.ID `json:"user_id"`
}

// addTicketMember gives someone access to a ticket from the dashboard, as
// /ticket add does, and returns them.
func (s *Server) addTicketMember(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	var in memberInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.UserID == 0 {
		s.writeFailure(w, invalid("user_id", "Choose who to add."))
		return
	}
	member, err := s.discord.GetMember(t.GuildID, in.UserID, rest.WithCtx(r.Context()))
	if discordx.IsCode(err, discordx.CodeUnknownMember, discordx.CodeUnknownUser) {
		s.writeFailure(w, invalid("user_id", "Nobody in this server has that user ID."))
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	user := auth.FromContext(r.Context()).User
	if s.actionFailed(w, s.dashboard.AddMember(r.Context(), t, user.ID, member.User), "user_id") {
		return
	}
	writeJSON(w, http.StatusCreated, ticketbot.TicketMember{
		ID: member.User.ID, Name: member.EffectiveName(), AvatarURL: member.EffectiveAvatarURL(),
		Opener: member.User.ID == t.OpenerID,
	})
}

// removeTicketMember takes someone's access to a ticket away from the
// dashboard, as /ticket remove does.
func (s *Server) removeTicketMember(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	userID, err := snowflake.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	// Someone who has left the server can still be taken off the ticket.
	target := discord.User{ID: userID}
	var targetMember *discord.ResolvedMember
	member, err := s.discord.GetMember(t.GuildID, userID, rest.WithCtx(r.Context()))
	switch {
	case err == nil:
		target = member.User
		if targetMember, err = s.resolvedMember(r.Context(), t.GuildID, *member); err != nil {
			s.writeFailure(w, err)
			return
		}
	case !discordx.IsCode(err, discordx.CodeUnknownMember, discordx.CodeUnknownUser):
		s.writeFailure(w, err)
		return
	}
	user := auth.FromContext(r.Context()).User
	if s.actionFailed(w, s.dashboard.RemoveMember(r.Context(), t, user.ID, target, targetMember), "") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolvedMember adds a member's server-wide permissions, which decide
// whether taking them off a ticket would change what they can see.
func (s *Server) resolvedMember(ctx context.Context, guildID snowflake.ID, m discord.Member) (*discord.ResolvedMember, error) {
	roles, err := s.rawRoles(ctx, guildID)
	if err != nil {
		return nil, err
	}
	var owner snowflake.ID
	if g, err := s.store.GetGuild(ctx, guildID); err == nil {
		owner = g.OwnerID
	}
	return &discord.ResolvedMember{Member: m, Permissions: memberPermissions(guildID, owner, m, roles)}, nil
}

// memberPermissions is a member's server-wide permissions. The owner has
// them all, whatever their roles.
func memberPermissions(guildID, ownerID snowflake.ID, m discord.Member, roles []discord.Role) discord.Permissions {
	if m.User.ID == ownerID {
		return discord.PermissionsAll
	}
	return discordx.Permissions(guildID, m.User.ID, m.RoleIDs, roles, nil)
}

type renameInput struct {
	Name string `json:"name"`
}

// renameTicket renames a ticket's channel or thread from the dashboard, as
// /ticket rename does.
func (s *Server) renameTicket(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	var in renameInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > 100 {
		s.writeFailure(w, invalid("name", "Give it a name of up to 100 characters."))
		return
	}
	if s.actionFailed(w, s.dashboard.Rename(r.Context(), t, in.Name), "name") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// channelDeleted explains the 409 for an action on a ticket whose channel
// was deleted in Discord.
const channelDeleted = "This ticket's channel was deleted in Discord, so the ticket has been closed."

// actionFailed writes the error from a dashboard action on a ticket, if there
// is one, and reports whether it did: a deleted channel is a 409 (the ticket
// has been closed), and the bot's reasons are 422s tied to field.
func (s *Server) actionFailed(w http.ResponseWriter, err error, field string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ticketbot.ErrChannelDeleted) {
		writeError(w, http.StatusConflict, channelDeleted)
	} else if msg, ok := ticketbot.UserMessage(err); ok {
		s.writeFailure(w, invalid(field, msg))
	} else {
		s.writeFailure(w, err)
	}
	return true
}
