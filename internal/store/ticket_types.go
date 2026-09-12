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
	NameFormat     string         `json:"name_format"`
	WelcomeMessage string         `json:"welcome_message"`
	MaxOpenPerUser int            `json:"max_open_per_user"`
	Questions      []Question     `json:"questions"`
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
	// ButtonStyle is the colour of this type's button on ticket buttons.
	// ButtonLabel replaces the type's name on the button when set.
	ButtonStyle ButtonStyle `json:"button_style"`
	ButtonLabel string      `json:"button_label"`
	CreatedAt   time.Time   `json:"created_at"`
}

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
	name_format, welcome_message, max_open_per_user, questions, auto_close_hours,
	required_role_ids, blocked_role_ids, cooldown_minutes, ask_rating, rating_prompt, button_style, button_label,
	created_at`

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
	)
	err := row.Scan(&t.ID, &guildID, &t.Name, &t.Emoji, &t.Description, &mode, &parentID, &roles,
		&t.NameFormat, &t.WelcomeMessage, &t.MaxOpenPerUser, &t.Questions, &t.AutoCloseHours,
		&required, &blocked, &t.CooldownMinutes, &t.AskRating, &t.RatingPrompt, &style, &t.ButtonLabel, &t.CreatedAt)
	if err != nil {
		return t, notFound(err)
	}
	t.GuildID = snowflake.ID(guildID)
	t.Mode = TicketMode(mode)
	t.ButtonStyle = ButtonStyle(style)
	t.ParentID = idFromNullable(parentID)
	t.SupportRoleIDs = fromInt64s(roles)
	t.RequiredRoleIDs = fromInt64s(required)
	t.BlockedRoleIDs = fromInt64s(blocked)
	t.Questions = questionsOrEmpty(t.Questions)
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

// CreateTicketType inserts t and sets its ID and CreatedAt.
func (s *Store) CreateTicketType(ctx context.Context, t *TicketType) error {
	t.Questions = questionsOrEmpty(t.Questions)
	if t.ButtonStyle == "" {
		t.ButtonStyle = ButtonPrimary
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO ticket_types (guild_id, name, emoji, description, mode, parent_id, support_role_ids,
		                          name_format, welcome_message, max_open_per_user, questions, auto_close_hours,
		                          required_role_ids, blocked_role_ids, cooldown_minutes, ask_rating, rating_prompt,
		                          button_style, button_label)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id, created_at`,
		int64(t.GuildID), t.Name, t.Emoji, t.Description, string(t.Mode), nullableID(t.ParentID),
		toInt64s(t.SupportRoleIDs), t.NameFormat, t.WelcomeMessage, t.MaxOpenPerUser, t.Questions, t.AutoCloseHours,
		toInt64s(t.RequiredRoleIDs), toInt64s(t.BlockedRoleIDs), t.CooldownMinutes, t.AskRating, t.RatingPrompt,
		string(t.ButtonStyle), t.ButtonLabel,
	).Scan(&t.ID, &t.CreatedAt)
}

func (s *Store) UpdateTicketType(ctx context.Context, t TicketType) error {
	if t.ButtonStyle == "" {
		t.ButtonStyle = ButtonPrimary
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE ticket_types
		SET name = $3, emoji = $4, description = $5, mode = $6, parent_id = $7, support_role_ids = $8,
		    name_format = $9, welcome_message = $10, max_open_per_user = $11, questions = $12, auto_close_hours = $13,
		    required_role_ids = $14, blocked_role_ids = $15, cooldown_minutes = $16, ask_rating = $17,
		    rating_prompt = $18, button_style = $19, button_label = $20, updated_at = now()
		WHERE guild_id = $1 AND id = $2`,
		int64(t.GuildID), t.ID, t.Name, t.Emoji, t.Description, string(t.Mode), nullableID(t.ParentID),
		toInt64s(t.SupportRoleIDs), t.NameFormat, t.WelcomeMessage, t.MaxOpenPerUser, questionsOrEmpty(t.Questions),
		t.AutoCloseHours, toInt64s(t.RequiredRoleIDs), toInt64s(t.BlockedRoleIDs), t.CooldownMinutes, t.AskRating,
		t.RatingPrompt, string(t.ButtonStyle), t.ButtonLabel)
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
