package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

type claimInput struct {
	// UserID assigns the ticket to another staff member; empty claims it
	// for the dashboard user.
	UserID snowflake.ID `json:"user_id"`
}

// claimTicket claims a ticket for the dashboard user, or hands it to another
// staff member, as the Claim button in Discord does. The claimer must be
// support staff for the ticket's type or manage the server, since a claim
// lock gives them the ticket channel.
func (s *Server) claimTicket(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	var in claimInput
	if !decodeJSON(w, r, &in) {
		return
	}
	user := auth.FromContext(r.Context()).User
	target := in.UserID
	if target == 0 {
		target = user.ID
	}
	member, err := s.discord.GetMember(t.GuildID, target, rest.WithCtx(r.Context()))
	if discordx.IsCode(err, discordx.CodeUnknownMember, discordx.CodeUnknownUser) {
		s.writeFailure(w, invalid("user_id", "Nobody in this server has that user ID."))
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	if !s.canHandle(r, t, *member) {
		if target == user.ID {
			s.writeFailure(w, invalid("", "Only the ticket type's support staff can claim its tickets, and you aren't in one of its support roles."))
		} else {
			s.writeFailure(w, invalid("user_id", member.EffectiveName()+" isn't in one of the ticket type's support roles."))
		}
		return
	}
	if target == user.ID {
		t, err = s.dashboard.Claim(r.Context(), t, user.ID, member.EffectiveName())
	} else {
		t, err = s.dashboard.Assign(r.Context(), t, user.ID, target, member.EffectiveName())
	}
	s.writeTicketAction(w, t, err)
}

// unclaimTicket releases a ticket's claim from the dashboard, whoever holds
// it, so a lead can free up a ticket a colleague has gone quiet on.
func (s *Server) unclaimTicket(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	user := auth.FromContext(r.Context()).User
	t, err := s.dashboard.Unclaim(r.Context(), t, user.ID)
	s.writeTicketAction(w, t, err)
}

type assigneeResponse struct {
	ID        snowflake.ID `json:"id"`
	Name      string       `json:"name"`
	AvatarURL string       `json:"avatar_url"`
}

// maxAssignees caps the assignee search, which is a type-ahead.
const maxAssignees = 10

// listAssignees searches the server's members by name for those who can
// take the ticket: the type's support staff and server managers.
func (s *Server) listAssignees(w http.ResponseWriter, r *http.Request) {
	t, ok := s.guildTicket(w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []assigneeResponse{})
		return
	}
	// Discord matches name prefixes; ask for more than shown since staff
	// are a subset.
	members, err := s.discord.SearchMembers(t.GuildID, q, 100, rest.WithCtx(r.Context()))
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	out := []assigneeResponse{}
	for _, m := range members {
		if m.User.Bot || !s.canHandle(r, t, m) {
			continue
		}
		out = append(out, assigneeResponse{ID: m.User.ID, Name: m.EffectiveName(), AvatarURL: m.EffectiveAvatarURL()})
		if len(out) == maxAssignees {
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// canHandle reports whether a member may hold a ticket: they manage the
// server or have one of its type's support roles (as isStaff in the bot).
func (s *Server) canHandle(r *http.Request, t store.Ticket, m discord.Member) bool {
	if s.managesGuild(r, t.GuildID, m) {
		return true
	}
	if t.TicketTypeID == nil {
		return false
	}
	tt, err := s.store.GetTicketType(r.Context(), t.GuildID, *t.TicketTypeID)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(m.RoleIDs, func(id snowflake.ID) bool { return slices.Contains(tt.SupportRoleIDs, id) })
}
