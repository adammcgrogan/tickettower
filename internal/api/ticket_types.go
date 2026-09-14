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

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/panels"
	"github.com/adammcgrogan/tickettower/internal/store"
)

const maxSupportRoles = 10

// autoCloseOptions are the inactivity windows offered in the dashboard.
var autoCloseOptions = []int{12, 24, 48, 72, 168}

// cooldownOptions are the waits (in minutes) between a member's tickets of
// a type offered in the dashboard; 0 is no wait.
var cooldownOptions = []int{0, 5, 15, 30, 60, 360, 1440}

type ticketTypeInput struct {
	Name            string           `json:"name"`
	Emoji           string           `json:"emoji"`
	Description     string           `json:"description"`
	Mode            store.TicketMode `json:"mode"`
	ParentID        *snowflake.ID    `json:"parent_id"`
	SupportRoleIDs  []snowflake.ID   `json:"support_role_ids"`
	NameFormat      string           `json:"name_format"`
	WelcomeMessage  string           `json:"welcome_message"`
	MaxOpenPerUser  int              `json:"max_open_per_user"`
	Questions       []store.Question `json:"questions"`
	Answers         []store.Answer   `json:"answers"`
	AutoCloseHours  *int             `json:"auto_close_hours"`
	RequiredRoleIDs []snowflake.ID   `json:"required_role_ids"`
	BlockedRoleIDs  []snowflake.ID   `json:"blocked_role_ids"`
	CooldownMinutes int              `json:"cooldown_minutes"`
	// AskRating is a pointer so a client that doesn't send it (the setup
	// templates) keeps the default of asking.
	AskRating    *bool             `json:"ask_rating"`
	RatingPrompt string            `json:"rating_prompt"`
	ButtonStyle  store.ButtonStyle `json:"button_style"`
	ButtonLabel  string            `json:"button_label"`

	ClaimLock              store.ClaimLock     `json:"claim_lock"`
	ClaimLockExemptRoleIDs []snowflake.ID      `json:"claim_lock_exempt_role_ids"`
	ReplyTargetMinutes     *int                `json:"reply_target_minutes"`
	ReminderMinutes        *int                `json:"reminder_minutes"`
	ReminderRepeat         bool                `json:"reminder_repeat"`
	ReminderWhere          store.ReminderWhere `json:"reminder_where"`
	ReminderPing           store.ReminderPing  `json:"reminder_ping"`
	ClosedParentID         *snowflake.ID       `json:"closed_parent_id"`
	ClosedMemberAccess     store.ClosedAccess  `json:"closed_member_access"`
	ClosedKeepDays         int                 `json:"closed_keep_days"`
	NotifyOnOpen           store.NotifyMode    `json:"notify_on_open"`
	NotifyRoleID           *snowflake.ID       `json:"notify_role_id"`
}

// minuteOptions are the waits offered for reply targets and reminders.
var minuteOptions = []int{15, 30, 60, 120, 240, 480, 720, 1440, 2880}

func (in ticketTypeInput) apply(t *store.TicketType) {
	t.Name = in.Name
	t.Emoji = in.Emoji
	t.Description = in.Description
	t.Mode = in.Mode
	t.ParentID = in.ParentID
	t.SupportRoleIDs = in.SupportRoleIDs
	t.NameFormat = in.NameFormat
	t.WelcomeMessage = in.WelcomeMessage
	t.MaxOpenPerUser = in.MaxOpenPerUser
	t.Questions = in.Questions
	t.Answers = in.Answers
	t.AutoCloseHours = in.AutoCloseHours
	t.RequiredRoleIDs = in.RequiredRoleIDs
	t.BlockedRoleIDs = in.BlockedRoleIDs
	t.CooldownMinutes = in.CooldownMinutes
	t.AskRating = in.AskRating == nil || *in.AskRating
	t.RatingPrompt = in.RatingPrompt
	t.ButtonStyle = in.ButtonStyle
	t.ButtonLabel = in.ButtonLabel
	t.ClaimLock = in.ClaimLock
	t.ClaimLockExemptRoleIDs = in.ClaimLockExemptRoleIDs
	t.ReplyTargetMinutes = in.ReplyTargetMinutes
	t.ReminderMinutes = in.ReminderMinutes
	t.ReminderRepeat = in.ReminderRepeat
	t.ReminderWhere = in.ReminderWhere
	t.ReminderPing = in.ReminderPing
	t.ClosedParentID = in.ClosedParentID
	t.ClosedMemberAccess = in.ClosedMemberAccess
	t.ClosedKeepDays = in.ClosedKeepDays
	t.NotifyOnOpen = in.NotifyOnOpen
	t.NotifyRoleID = in.NotifyRoleID
}

// validateQuestions normalises a ticket type's form questions.
func validateQuestions(qs []store.Question) ([]store.Question, error) {
	if len(qs) > store.MaxQuestions {
		return nil, invalid("questions", fmt.Sprintf("Add up to %d questions.", store.MaxQuestions))
	}
	out := make([]store.Question, len(qs))
	for i, q := range qs {
		q.Label = strings.TrimSpace(q.Label)
		if n := utf8.RuneCountInString(q.Label); n == 0 || n > store.MaxQuestionLabel {
			return nil, invalid("questions", fmt.Sprintf("Question %d needs a question of up to %d characters.", i+1, store.MaxQuestionLabel))
		}
		q.Placeholder = strings.TrimSpace(q.Placeholder)
		if utf8.RuneCountInString(q.Placeholder) > store.MaxQuestionHint {
			return nil, invalid("questions", fmt.Sprintf("Keep the placeholder for question %d to %d characters or fewer.", i+1, store.MaxQuestionHint))
		}
		if q.Style == "" {
			q.Style = store.QuestionShort
		}
		if q.Style != store.QuestionShort && q.Style != store.QuestionParagraph {
			return nil, invalid("questions", fmt.Sprintf("Choose a short answer or a paragraph for question %d.", i+1))
		}
		out[i] = q
	}
	return out, nil
}

// validateAnswers normalises a ticket type's suggested answers.
func validateAnswers(as []store.Answer) ([]store.Answer, error) {
	if len(as) > store.MaxAnswers {
		return nil, invalid("answers", fmt.Sprintf("Add up to %d answers.", store.MaxAnswers))
	}
	out := make([]store.Answer, len(as))
	for i, a := range as {
		a.Title = strings.TrimSpace(a.Title)
		if n := utf8.RuneCountInString(a.Title); n == 0 || n > store.MaxAnswerTitle {
			return nil, invalid("answers", fmt.Sprintf("Answer %d needs a title of up to %d characters.", i+1, store.MaxAnswerTitle))
		}
		a.Body = strings.TrimSpace(a.Body)
		if n := utf8.RuneCountInString(a.Body); n == 0 || n > store.MaxAnswerBody {
			return nil, invalid("answers", fmt.Sprintf("Answer %d needs a short answer of up to %d characters.", i+1, store.MaxAnswerBody))
		}
		out[i] = a
	}
	return out, nil
}

// validateTicketType normalises the input and checks it against the guild's
// channels and roles.
func (s *Server) validateTicketType(ctx context.Context, guildID snowflake.ID, in *ticketTypeInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > 80 {
		return invalid("name", "Give the ticket type a name of up to 80 characters.")
	}
	in.Emoji = strings.TrimSpace(in.Emoji)
	if !panels.IsValidEmoji(in.Emoji) {
		return invalid("emoji", "Use a single emoji, or a custom emoji like <:name:id>.")
	}
	// Discord only lets the bot put a custom emoji on a button if the emoji
	// is from a server it's in, so check it's one of this server's rather
	// than failing at publish time.
	if id := panels.CustomEmojiID(in.Emoji); id != 0 {
		emojis, err := s.guildEmojis(ctx, guildID)
		if err != nil {
			return err
		}
		if !hasEmoji(emojis, id) {
			return invalid("emoji", "That custom emoji isn't from this server, so the bot can't put it on a button. Use one of this server's emoji, or a standard one.")
		}
	}
	in.Description = strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(in.Description) > 100 {
		return invalid("description", "Keep the description to 100 characters or fewer.")
	}
	if in.Mode != store.ModeChannel && in.Mode != store.ModeThread {
		return invalid("mode", "Choose whether tickets open as channels or threads.")
	}
	in.NameFormat = strings.TrimSpace(in.NameFormat)
	if in.NameFormat == "" {
		in.NameFormat = "ticket-{number}"
	}
	if utf8.RuneCountInString(in.NameFormat) > 90 {
		return invalid("name_format", "Keep the name format to 90 characters or fewer.")
	}
	in.WelcomeMessage = strings.TrimSpace(in.WelcomeMessage)
	if utf8.RuneCountInString(in.WelcomeMessage) > 2000 {
		return invalid("welcome_message", "Keep the welcome message to 2,000 characters or fewer.")
	}
	if in.MaxOpenPerUser == 0 {
		in.MaxOpenPerUser = 1
	}
	if in.MaxOpenPerUser < 1 || in.MaxOpenPerUser > 10 {
		return invalid("max_open_per_user", "Set a limit between 1 and 10.")
	}
	questions, err := validateQuestions(in.Questions)
	if err != nil {
		return err
	}
	in.Questions = questions
	answers, err := validateAnswers(in.Answers)
	if err != nil {
		return err
	}
	in.Answers = answers
	if h := in.AutoCloseHours; h != nil && !slices.Contains(autoCloseOptions, *h) {
		return invalid("auto_close_hours", "Choose one of the listed auto-close times.")
	}
	if !slices.Contains(cooldownOptions, in.CooldownMinutes) {
		return invalid("cooldown_minutes", "Choose one of the listed waits.")
	}
	if in.ButtonStyle == "" {
		in.ButtonStyle = store.ButtonPrimary
	}
	if !slices.Contains(store.ButtonStyles, in.ButtonStyle) {
		return invalid("button_style", "Choose one of the listed button colours.")
	}
	in.ButtonLabel = strings.TrimSpace(in.ButtonLabel)
	if utf8.RuneCountInString(in.ButtonLabel) > store.MaxButtonLabel {
		return invalid("button_label", fmt.Sprintf("Keep the button label to %d characters or fewer.", store.MaxButtonLabel))
	}
	if in.ClaimLock == "" {
		in.ClaimLock = store.ClaimLockOff
	}
	if !in.ClaimLock.Valid() {
		return invalid("claim_lock", "Choose what claiming does for the rest of the team.")
	}
	if in.ClaimLock != store.ClaimLockOff && in.Mode == store.ModeThread {
		return invalid("claim_lock", "Private threads can't limit a role's access, so claim locks only work for channel tickets.")
	}
	if m := in.ReplyTargetMinutes; m != nil && !slices.Contains(minuteOptions, *m) {
		return invalid("reply_target_minutes", "Choose one of the listed reply targets.")
	}
	if m := in.ReminderMinutes; m != nil && !slices.Contains(minuteOptions, *m) {
		return invalid("reminder_minutes", "Choose one of the listed reminder times.")
	}
	if in.ReminderWhere == "" {
		in.ReminderWhere = store.RemindInTicket
	}
	if !in.ReminderWhere.Valid() {
		return invalid("reminder_where", "Choose where reminders are posted.")
	}
	if in.ReminderPing == "" {
		in.ReminderPing = store.PingClaimer
	}
	if !in.ReminderPing.Valid() {
		return invalid("reminder_ping", "Choose who reminders mention.")
	}
	if in.NotifyOnOpen == "" {
		in.NotifyOnOpen = store.NotifySupportRoles
	}
	if !in.NotifyOnOpen.Valid() {
		return invalid("notify_on_open", "Choose who new tickets ping.")
	}
	// Thread tickets always ping the support roles, since that's what adds
	// them to the thread, so a custom setting only applies to channel tickets.
	if in.Mode == store.ModeThread {
		in.NotifyOnOpen = store.NotifySupportRoles
		in.NotifyRoleID = nil
	}
	if in.NotifyOnOpen != store.NotifyCustomRole {
		in.NotifyRoleID = nil
	} else if in.NotifyRoleID == nil {
		return invalid("notify_role_id", "Choose the role to ping.")
	}
	in.RatingPrompt = strings.TrimSpace(in.RatingPrompt)
	if utf8.RuneCountInString(in.RatingPrompt) > store.MaxRatingPrompt {
		return invalid("rating_prompt", fmt.Sprintf("Keep the rating request to %d characters or fewer.", store.MaxRatingPrompt))
	}

	if in.ParentID != nil && *in.ParentID == 0 {
		in.ParentID = nil
	}
	channels, err := s.guildChannels(ctx, guildID)
	if err != nil {
		return err
	}
	var parent *channelResponse
	if in.ParentID != nil {
		for i := range channels {
			if channels[i].ID == *in.ParentID {
				parent = &channels[i]
			}
		}
	}
	switch in.Mode {
	case store.ModeThread:
		if parent == nil || parent.Kind != "text" {
			return invalid("parent_id", "Choose the text channel that ticket threads will be created in.")
		}
	case store.ModeChannel:
		if in.ParentID != nil && (parent == nil || parent.Kind != "category") {
			return invalid("parent_id", "Choose a category for ticket channels, or leave it empty.")
		}
	}

	// Closed channels can be kept in a category (channel tickets only).
	if in.ClosedParentID != nil && *in.ClosedParentID == 0 {
		in.ClosedParentID = nil
	}
	if in.Mode == store.ModeThread {
		in.ClosedParentID = nil
	}
	if in.ClosedParentID != nil {
		var closed *channelResponse
		for i := range channels {
			if channels[i].ID == *in.ClosedParentID {
				closed = &channels[i]
			}
		}
		if closed == nil || closed.Kind != "category" {
			return invalid("closed_parent_id", "Choose the category closed tickets are kept in.")
		}
		if in.ParentID != nil && *in.ParentID == *in.ClosedParentID {
			return invalid("closed_parent_id", "Keep closed tickets in a different category from open ones, so the open category doesn't fill up.")
		}
	}
	if in.ClosedMemberAccess == "" {
		in.ClosedMemberAccess = store.ClosedRead
	}
	if !in.ClosedMemberAccess.Valid() {
		return invalid("closed_member_access", "Choose whether the member can still read a closed ticket.")
	}
	if in.ClosedKeepDays == 0 {
		in.ClosedKeepDays = 7
	}
	if !slices.Contains(store.ClosedKeepDayOptions, in.ClosedKeepDays) {
		return invalid("closed_keep_days", "Choose one of the listed times to keep closed tickets.")
	}

	in.SupportRoleIDs = normaliseIDs(in.SupportRoleIDs)
	in.RequiredRoleIDs = normaliseIDs(in.RequiredRoleIDs)
	in.BlockedRoleIDs = normaliseIDs(in.BlockedRoleIDs)
	// Only support roles can be exempt from a claim lock.
	in.ClaimLockExemptRoleIDs = slices.DeleteFunc(normaliseIDs(in.ClaimLockExemptRoleIDs), func(id snowflake.ID) bool {
		return !slices.Contains(in.SupportRoleIDs, id)
	})
	var notifyRoleIDs []snowflake.ID
	if in.NotifyRoleID != nil {
		notifyRoleIDs = []snowflake.ID{*in.NotifyRoleID}
	}
	roleFields := []struct {
		field string
		ids   []snowflake.ID
		limit string
	}{
		{"support_role_ids", in.SupportRoleIDs, fmt.Sprintf("Choose up to %d support roles.", maxSupportRoles)},
		{"required_role_ids", in.RequiredRoleIDs, fmt.Sprintf("Choose up to %d required roles.", maxSupportRoles)},
		{"blocked_role_ids", in.BlockedRoleIDs, fmt.Sprintf("Choose up to %d blocked roles.", maxSupportRoles)},
		{"notify_role_id", notifyRoleIDs, "Choose the role to ping."},
	}
	var roles []roleResponse
	for _, f := range roleFields {
		if len(f.ids) > maxSupportRoles {
			return invalid(f.field, f.limit)
		}
		if len(f.ids) == 0 {
			continue
		}
		if roles == nil {
			if roles, err = s.guildRoles(ctx, guildID); err != nil {
				return err
			}
		}
		for _, id := range f.ids {
			if !slices.ContainsFunc(roles, func(r roleResponse) bool { return r.ID == id }) {
				return invalid(f.field, "One of the selected roles no longer exists.")
			}
		}
	}
	// A role that's both required and blocked could never open a ticket.
	for _, id := range in.RequiredRoleIDs {
		if slices.Contains(in.BlockedRoleIDs, id) {
			return invalid("blocked_role_ids", "A role can't be both required and blocked.")
		}
	}
	return nil
}

func (s *Server) listTicketTypes(w http.ResponseWriter, r *http.Request) {
	types, err := s.store.ListTicketTypes(r.Context(), guildFrom(r).ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, types)
}

func (s *Server) createTicketType(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	var in ticketTypeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.validateTicketType(r.Context(), g.ID, &in); err != nil {
		s.writeFailure(w, err)
		return
	}
	count, err := s.store.CountTicketTypes(r.Context(), g.ID)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	if max := s.limits(r.Context(), g.ID).MaxTicketTypes; count >= max {
		s.writeFailure(w, invalid("", fmt.Sprintf("You can have up to %d ticket types.", max)))
		return
	}

	t := store.TicketType{GuildID: g.ID}
	in.apply(&t)
	if err := s.store.CreateTicketType(r.Context(), &t); err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) updateTicketType(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	id, ok := pathID(r, "typeID")
	if !ok {
		writeError(w, http.StatusNotFound, "ticket type not found")
		return
	}
	t, err := s.store.GetTicketType(r.Context(), g.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ticket type not found")
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}

	var in ticketTypeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.validateTicketType(r.Context(), g.ID, &in); err != nil {
		s.writeFailure(w, err)
		return
	}
	in.apply(&t)
	if err := s.store.UpdateTicketType(r.Context(), t); err != nil {
		s.writeFailure(w, err)
		return
	}

	affected, err := s.store.PublishedPanelsWithType(r.Context(), g.ID, id)
	if err != nil {
		s.log.Error("load affected panels", slog.Any("err", err))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket_type": t,
		"warning":     s.refreshPublished(r.Context(), g.ID, affected),
	})
}

func (s *Server) deleteTicketType(w http.ResponseWriter, r *http.Request) {
	g := guildFrom(r)
	id, ok := pathID(r, "typeID")
	if !ok {
		writeError(w, http.StatusNotFound, "ticket type not found")
		return
	}
	affected, err := s.store.PublishedPanelsWithType(r.Context(), g.ID, id)
	if err != nil {
		s.writeFailure(w, err)
		return
	}
	var open *store.ErrOpenTickets
	if err := s.store.DeleteTicketType(r.Context(), g.ID, id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ticket type not found")
		return
	} else if errors.As(err, &open) {
		writeError(w, http.StatusConflict, openTicketsMessage(open.Count))
		return
	} else if err != nil {
		s.writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"warning": s.refreshPublished(r.Context(), g.ID, affected),
	})
}

// refreshPublished re-renders published panels in Discord after their ticket
// types change. It returns a warning for the user if any could not be
// updated.
func (s *Server) refreshPublished(ctx context.Context, guildID snowflake.ID, affected []store.Panel) string {
	if len(affected) == 0 {
		return ""
	}
	const warning = "Saved, but some published ticket buttons couldn't be updated in Discord. Try publishing them again."
	types, err := s.store.ListTicketTypes(ctx, guildID)
	if err != nil {
		s.log.Error("load ticket types", slog.Any("err", err))
		return warning
	}
	failed := false
	for _, p := range affected {
		fresh, err := s.store.GetPanel(ctx, guildID, p.ID)
		if err == nil {
			err = s.syncPanelMessage(ctx, fresh, types)
		}
		if err != nil {
			s.log.Warn("refresh panel", slog.Int64("panel_id", p.ID), slog.Any("err", err))
			failed = true
		}
	}
	if failed {
		return warning
	}
	return ""
}

// syncPanelMessage edits a published panel's message in place. If the
// message has been deleted in Discord, the panel is marked unpublished.
func (s *Server) syncPanelMessage(ctx context.Context, p store.Panel, types []store.TicketType) error {
	if p.ChannelID == nil || p.MessageID == nil {
		return nil
	}
	branding := s.limits(ctx, p.GuildID).Branding
	_, err := s.discord.UpdateMessage(*p.ChannelID, *p.MessageID, panels.Update(s.cfg.AppName, branding, p, types), rest.WithCtx(ctx))
	if discordx.IsCode(err, discordx.CodeUnknownMessage, discordx.CodeUnknownChannel) {
		return s.store.SetPanelMessage(ctx, p.GuildID, p.ID, nil, nil)
	}
	return err
}

// openTicketsMessage explains why a ticket type with open tickets can't be
// deleted.
func openTicketsMessage(n int) string {
	if n == 1 {
		return "This ticket type still has 1 open ticket. Close it first, or move it to another type, then delete the type."
	}
	return fmt.Sprintf("This ticket type still has %d open tickets. Close them first, or move them to another type, then delete the type.", n)
}

// hasEmoji reports whether a custom emoji is one of the guild's.
func hasEmoji(emojis []discord.Emoji, id snowflake.ID) bool {
	return slices.ContainsFunc(emojis, func(e discord.Emoji) bool { return e.ID == id })
}
