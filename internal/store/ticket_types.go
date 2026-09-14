package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/jackc/pgx/v5"
)

type TicketMode string

const (
	ModeChannel TicketMode = "channel"
	ModeThread  TicketMode = "thread"
)

type QuestionStyle string

const (
	QuestionShort     QuestionStyle = "short"
	QuestionParagraph QuestionStyle = "paragraph"
)

// Form limits. Discord allows 5 inputs per modal and 45 characters per
// label; answer lengths are kept short enough that five answers and the
// welcome message fit in one embed.
const (
	MaxQuestions       = 5
	MaxQuestionLabel   = 45
	MaxQuestionHint    = 100
	MaxShortAnswer     = 200
	MaxParagraphAnswer = 1000
)

// Suggested answer limits, sized so up to MaxAnswers fit comfortably in one
// embed alongside its title.
const (
	MaxAnswers     = 5
	MaxAnswerTitle = 100
	MaxAnswerBody  = 300
)

// Question is asked in a form when a member opens a ticket.
type Question struct {
	Label       string        `json:"label"`
	Placeholder string        `json:"placeholder"`
	Style       QuestionStyle `json:"style"`
	Required    bool          `json:"required"`
}

// MaxAnswer is the longest answer a member can give.
func (q Question) MaxAnswer() int {
	if q.Style == QuestionParagraph {
		return MaxParagraphAnswer
	}
	return MaxShortAnswer
}

// Answer is a short, pre-written answer to a common question, shown to a
// member before they open a ticket of this type.
type Answer struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// TicketType is a category of ticket (e.g. "Billing") and how it is handled.
type TicketType struct {
	ID             int64          `json:"id"`
	GuildID        snowflake.ID   `json:"guild_id"`
	Name           string         `json:"name"`
	Emoji          string         `json:"emoji"`
	Description    string         `json:"description"`
	Mode           TicketMode     `json:"mode"`
	ParentID       *snowflake.ID  `json:"parent_id"`
	SupportRoleIDs []snowflake.ID `json:"support_role_ids"`
	// NotifyOnOpen is who a new channel ticket pings: the support roles
	// (default), a specific role (NotifyRoleID), or nobody. Thread tickets
	// always ping the support roles, since that's what gives them access.
	NotifyOnOpen   NotifyMode    `json:"notify_on_open"`
	NotifyRoleID   *snowflake.ID `json:"notify_role_id"`
	// AutoAssign shares out new tickets of this type round robin, among
	// staff who've opted in with /ticket available. Off by default.
	AutoAssign bool `json:"auto_assign"`
	NameFormat     string        `json:"name_format"`
	WelcomeMessage string        `json:"welcome_message"`
	MaxOpenPerUser int           `json:"max_open_per_user"`
	Questions      []Question    `json:"questions"`
	// Answers are suggested answers shown to a member before this type's
	// form (or welcome message) is shown, so they can skip opening a
	// ticket if one answers their question.
	Answers []Answer `json:"answers"`
	// AutoCloseHours is how long a ticket can sit without activity before
	// it closes itself; nil means never.
	AutoCloseHours *int `json:"auto_close_hours"`
	// RequiredRoleIDs: members need one of these to open the type (any
	// member can when empty). BlockedRoleIDs: members with one of these
	// can't. CooldownMinutes: how long after their last ticket of this type
	// closes a member has to wait before opening another; 0 means no wait.
	RequiredRoleIDs []snowflake.ID `json:"required_role_ids"`
	BlockedRoleIDs  []snowflake.ID `json:"blocked_role_ids"`
	CooldownMinutes int            `json:"cooldown_minutes"`
	// AskRating is whether the opener is asked to rate a ticket when it
	// closes. RatingPrompt is the wording of that request; "" uses the
	// default.
	AskRating    bool   `json:"ask_rating"`
	RatingPrompt string `json:"rating_prompt"`
	// ButtonStyle is the colour of this type's button on its ticket panel.
	// ButtonLabel replaces the type's name on the button when set.
	ButtonStyle ButtonStyle `json:"button_style"`
	ButtonLabel string      `json:"button_label"`
	// ClaimLock is what claiming does to the rest of the support team in a
	// channel ticket; roles in ClaimLockExemptRoleIDs keep full access.
	ClaimLock              ClaimLock      `json:"claim_lock"`
	ClaimLockExemptRoleIDs []snowflake.ID `json:"claim_lock_exempt_role_ids"`
	// ReplyTargetMinutes is how quickly the team aims to answer a waiting
	// ticket; nil means no target. ReminderMinutes is how long a ticket may
	// wait on the team before staff are reminded (nil: never), repeating
	// every ReminderMinutes if ReminderRepeat. ReminderWhere says where the
	// reminder goes and ReminderPing who it mentions.
	ReplyTargetMinutes *int          `json:"reply_target_minutes"`
	ReminderMinutes    *int          `json:"reminder_minutes"`
	ReminderRepeat     bool          `json:"reminder_repeat"`
	ReminderWhere      ReminderWhere `json:"reminder_where"`
	ReminderPing       ReminderPing  `json:"reminder_ping"`
	// ClosedParentID is the category a closed channel ticket is kept in,
	// read only, for ClosedKeepDays before it's deleted; nil deletes the
	// channel straight away. ClosedMemberAccess is whether the opener can
	// still read it while it's kept.
	ClosedParentID     *snowflake.ID `json:"closed_parent_id"`
	ClosedMemberAccess ClosedAccess  `json:"closed_member_access"`
	ClosedKeepDays     int           `json:"closed_keep_days"`
	CreatedAt          time.Time     `json:"created_at"`
}

// ClosedAccess is what the opener can do with a closed ticket's channel
// while it's kept.
type ClosedAccess string

const (
	ClosedRead   ClosedAccess = "read"   // read only
	ClosedHidden ClosedAccess = "hidden" // can't see it
)

func (a ClosedAccess) Valid() bool { return a == ClosedRead || a == ClosedHidden }

// ClosedKeepDayOptions are the keep times the dashboard offers.
var ClosedKeepDayOptions = []int{1, 3, 7, 14, 30}

// KeepsClosedChannels reports whether closed tickets of this type keep their
// channel for a while instead of deleting it.
func (t TicketType) KeepsClosedChannels() bool {
	return t.Mode == ModeChannel && t.ClosedParentID != nil
}

// ClaimLock is what claiming a channel ticket does to other support staff.
type ClaimLock string

const (
	ClaimLockOff      ClaimLock = "off"       // nothing changes
	ClaimLockReadOnly ClaimLock = "read_only" // others can read but not send
	ClaimLockHidden   ClaimLock = "hidden"    // others can't see the ticket
)

// ReminderWhere is where staff reminders are posted.
type ReminderWhere string

const (
	RemindInTicket ReminderWhere = "ticket"
	RemindInLog    ReminderWhere = "log"
	RemindInBoth   ReminderWhere = "both"
)

// NotifyMode is who a new ticket's welcome message pings, for channel
// tickets. Thread tickets always ping the support roles instead.
type NotifyMode string

const (
	NotifySupportRoles NotifyMode = "roles"
	NotifyCustomRole   NotifyMode = "custom"
	NotifyNobody       NotifyMode = "none"
)

func (m NotifyMode) Valid() bool {
	return m == NotifySupportRoles || m == NotifyCustomRole || m == NotifyNobody
}

// ReminderPing is who a reminder in the ticket mentions.
type ReminderPing string

const (
	PingClaimer ReminderPing = "claimer" // the claimer, or the support roles if unclaimed
	PingRoles   ReminderPing = "roles"   // always the support roles
	PingNobody  ReminderPing = "none"
)

// Valid reports whether the value is one the dashboard offers.
func (c ClaimLock) Valid() bool {
	return c == ClaimLockOff || c == ClaimLockReadOnly || c == ClaimLockHidden
}
func (w ReminderWhere) Valid() bool {
	return w == RemindInTicket || w == RemindInLog || w == RemindInBoth
}
func (p ReminderPing) Valid() bool { return p == PingClaimer || p == PingRoles || p == PingNobody }

// Label is what this type's button says.
func (t TicketType) Label() string {
	if t.ButtonLabel != "" {
		return t.ButtonLabel
	}
	return t.Name
}

// MaxRatingPrompt keeps a custom rating request short enough to read in a DM.
const MaxRatingPrompt = 300

// ButtonStyle is one of Discord's button colours.
type ButtonStyle string

const (
	ButtonPrimary   ButtonStyle = "primary"   // blurple
	ButtonSecondary ButtonStyle = "secondary" // grey
	ButtonSuccess   ButtonStyle = "success"   // green
	ButtonDanger    ButtonStyle = "danger"    // red
)

// ButtonStyles lists the styles in the order the dashboard offers them.
var ButtonStyles = []ButtonStyle{ButtonPrimary, ButtonSecondary, ButtonSuccess, ButtonDanger}

// MaxButtonLabel is Discord's limit on a button's label.
const MaxButtonLabel = 80

const ticketTypeColumns = `id, guild_id, name, emoji, description, mode, parent_id, support_role_ids,
	name_format, welcome_message, max_open_per_user, questions, answers, auto_close_hours,
	required_role_ids, blocked_role_ids, cooldown_minutes, ask_rating, rating_prompt, button_style, button_label,
	claim_lock, claim_lock_exempt_role_ids, reply_target_minutes, reminder_minutes, reminder_repeat, reminder_where,
	reminder_ping, closed_parent_id, closed_member_access, closed_keep_days, notify_on_open, notify_role_id,
	auto_assign, created_at`

func scanTicketType(row pgx.Row) (TicketType, error) {
	var (
		t        TicketType
		guildID  int64
		mode     string
		style    string
		parentID *int64
		roles    []int64
		required []int64
		blocked  []int64
		exempt   []int64
		lock     string
		where    string
		ping     string
		closedID *int64
		access   string
		notify   string
		notifyID *int64
	)
	err := row.Scan(&t.ID, &guildID, &t.Name, &t.Emoji, &t.Description, &mode, &parentID, &roles,
		&t.NameFormat, &t.WelcomeMessage, &t.MaxOpenPerUser, &t.Questions, &t.Answers, &t.AutoCloseHours,
		&required, &blocked, &t.CooldownMinutes, &t.AskRating, &t.RatingPrompt, &style, &t.ButtonLabel,
		&lock, &exempt, &t.ReplyTargetMinutes, &t.ReminderMinutes, &t.ReminderRepeat, &where, &ping,
		&closedID, &access, &t.ClosedKeepDays, &notify, &notifyID, &t.AutoAssign, &t.CreatedAt)
	if err != nil {
		return t, notFound(err)
	}
	t.ClaimLock, t.ReminderWhere, t.ReminderPing = ClaimLock(lock), ReminderWhere(where), ReminderPing(ping)
	t.ClosedParentID, t.ClosedMemberAccess = idFromNullable(closedID), ClosedAccess(access)
	t.ClaimLockExemptRoleIDs = fromInt64s(exempt)
	t.GuildID = snowflake.ID(guildID)
	t.Mode = TicketMode(mode)
	t.ButtonStyle = ButtonStyle(style)
	t.ParentID = idFromNullable(parentID)
	t.SupportRoleIDs = fromInt64s(roles)
	t.RequiredRoleIDs = fromInt64s(required)
	t.BlockedRoleIDs = fromInt64s(blocked)
	t.Questions = questionsOrEmpty(t.Questions)
	t.Answers = answersOrEmpty(t.Answers)
	t.NotifyOnOpen = NotifyMode(notify)
	t.NotifyRoleID = idFromNullable(notifyID)
	return t, nil
}

// questionsOrEmpty avoids storing a JSON null, which the NOT NULL constraint
// wouldn't catch.
func questionsOrEmpty(qs []Question) []Question {
	if qs == nil {
		return []Question{}
	}
	return qs
}

// answersOrEmpty avoids storing a JSON null, which the NOT NULL constraint
// wouldn't catch.
func answersOrEmpty(as []Answer) []Answer {
	if as == nil {
		return []Answer{}
	}
	return as
}

func (s *Store) ListTicketTypes(ctx context.Context, guildID snowflake.ID) ([]TicketType, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+ticketTypeColumns+` FROM ticket_types WHERE guild_id = $1 ORDER BY id`, int64(guildID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TicketType{}
	for rows.Next() {
		t, err := scanTicketType(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTicketType(ctx context.Context, guildID snowflake.ID, id int64) (TicketType, error) {
	return scanTicketType(s.pool.QueryRow(ctx,
		`SELECT `+ticketTypeColumns+` FROM ticket_types WHERE guild_id = $1 AND id = $2`, int64(guildID), id))
}

func (s *Store) CountTicketTypes(ctx context.Context, guildID snowflake.ID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM ticket_types WHERE guild_id = $1`, int64(guildID)).Scan(&n)
	return n, err
}

// defaults fills in the zero values of enum-like fields, so callers that
// don't set them (tests, templates) get the dashboard's defaults.
func (t *TicketType) defaults() {
	t.Questions = questionsOrEmpty(t.Questions)
	t.Answers = answersOrEmpty(t.Answers)
	if t.ButtonStyle == "" {
		t.ButtonStyle = ButtonPrimary
	}
	if t.ClaimLock == "" {
		t.ClaimLock = ClaimLockOff
	}
	if t.ReminderWhere == "" {
		t.ReminderWhere = RemindInTicket
	}
	if t.ReminderPing == "" {
		t.ReminderPing = PingClaimer
	}
	if t.ClosedMemberAccess == "" {
		t.ClosedMemberAccess = ClosedRead
	}
	if t.ClosedKeepDays == 0 {
		t.ClosedKeepDays = 7
	}
	if t.NotifyOnOpen == "" {
		t.NotifyOnOpen = NotifySupportRoles
	}
}

// CreateTicketType inserts t and sets its ID and CreatedAt.
func (s *Store) CreateTicketType(ctx context.Context, t *TicketType) error {
	t.defaults()
	return s.pool.QueryRow(ctx, `
		INSERT INTO ticket_types (guild_id, name, emoji, description, mode, parent_id, support_role_ids,
		                          name_format, welcome_message, max_open_per_user, questions, answers, auto_close_hours,
		                          required_role_ids, blocked_role_ids, cooldown_minutes, ask_rating, rating_prompt,
		                          button_style, button_label, claim_lock, claim_lock_exempt_role_ids,
		                          reply_target_minutes, reminder_minutes, reminder_repeat, reminder_where, reminder_ping,
		                          closed_parent_id, closed_member_access, closed_keep_days, notify_on_open, notify_role_id,
		                          auto_assign)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19,
		        $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33)
		RETURNING id, created_at`,
		int64(t.GuildID), t.Name, t.Emoji, t.Description, string(t.Mode), nullableID(t.ParentID),
		toInt64s(t.SupportRoleIDs), t.NameFormat, t.WelcomeMessage, t.MaxOpenPerUser, t.Questions, t.Answers, t.AutoCloseHours,
		toInt64s(t.RequiredRoleIDs), toInt64s(t.BlockedRoleIDs), t.CooldownMinutes, t.AskRating, t.RatingPrompt,
		string(t.ButtonStyle), t.ButtonLabel, string(t.ClaimLock), toInt64s(t.ClaimLockExemptRoleIDs),
		t.ReplyTargetMinutes, t.ReminderMinutes, t.ReminderRepeat, string(t.ReminderWhere), string(t.ReminderPing),
		nullableID(t.ClosedParentID), string(t.ClosedMemberAccess), t.ClosedKeepDays,
		string(t.NotifyOnOpen), nullableID(t.NotifyRoleID), t.AutoAssign,
	).Scan(&t.ID, &t.CreatedAt)
}

func (s *Store) UpdateTicketType(ctx context.Context, t TicketType) error {
	t.defaults()
	tag, err := s.pool.Exec(ctx, `
		UPDATE ticket_types
		SET name = $3, emoji = $4, description = $5, mode = $6, parent_id = $7, support_role_ids = $8,
		    name_format = $9, welcome_message = $10, max_open_per_user = $11, questions = $12, answers = $13,
		    auto_close_hours = $14, required_role_ids = $15, blocked_role_ids = $16, cooldown_minutes = $17,
		    ask_rating = $18, rating_prompt = $19, button_style = $20, button_label = $21, claim_lock = $22,
		    claim_lock_exempt_role_ids = $23, reply_target_minutes = $24, reminder_minutes = $25,
		    reminder_repeat = $26, reminder_where = $27, reminder_ping = $28, closed_parent_id = $29,
		    closed_member_access = $30, closed_keep_days = $31, notify_on_open = $32, notify_role_id = $33,
		    auto_assign = $34, updated_at = now()
		WHERE guild_id = $1 AND id = $2`,
		int64(t.GuildID), t.ID, t.Name, t.Emoji, t.Description, string(t.Mode), nullableID(t.ParentID),
		toInt64s(t.SupportRoleIDs), t.NameFormat, t.WelcomeMessage, t.MaxOpenPerUser, t.Questions, t.Answers,
		t.AutoCloseHours, toInt64s(t.RequiredRoleIDs), toInt64s(t.BlockedRoleIDs), t.CooldownMinutes, t.AskRating,
		t.RatingPrompt, string(t.ButtonStyle), t.ButtonLabel, string(t.ClaimLock), toInt64s(t.ClaimLockExemptRoleIDs),
		t.ReplyTargetMinutes, t.ReminderMinutes, t.ReminderRepeat, string(t.ReminderWhere), string(t.ReminderPing),
		nullableID(t.ClosedParentID), string(t.ClosedMemberAccess), t.ClosedKeepDays,
		string(t.NotifyOnOpen), nullableID(t.NotifyRoleID), t.AutoAssign)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrOpenTickets is returned by DeleteTicketType when the type still has open
// tickets. Deleting it would leave them without support roles or auto-close.
type ErrOpenTickets struct{ Count int }

func (e *ErrOpenTickets) Error() string {
	return fmt.Sprintf("ticket type has %d open tickets", e.Count)
}

// DeleteTicketType deletes a type that has no open tickets. It returns
// *ErrOpenTickets (with the count) if some are still open, so the check and
// the delete can't race with a ticket opening in between.
func (s *Store) DeleteTicketType(ctx context.Context, guildID snowflake.ID, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM ticket_types
		WHERE guild_id = $1 AND id = $2
		  AND NOT EXISTS (SELECT 1 FROM tickets WHERE ticket_type_id = $2 AND status = 'open')`, int64(guildID), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var open int
	err = s.pool.QueryRow(ctx, `
		SELECT count(t.id) FROM ticket_types tt
		LEFT JOIN tickets t ON t.ticket_type_id = tt.id AND t.status = 'open'
		WHERE tt.guild_id = $1 AND tt.id = $2
		GROUP BY tt.id`, int64(guildID), id).Scan(&open)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return &ErrOpenTickets{Count: open}
}
