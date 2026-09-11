package store

import (
	"context"
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
	CreatedAt      time.Time      `json:"created_at"`
}

const ticketTypeColumns = `id, guild_id, name, emoji, description, mode, parent_id, support_role_ids,
	name_format, welcome_message, max_open_per_user, questions, created_at`

func scanTicketType(row pgx.Row) (TicketType, error) {
	var (
		t        TicketType
		guildID  int64
		mode     string
		parentID *int64
		roles    []int64
	)
	err := row.Scan(&t.ID, &guildID, &t.Name, &t.Emoji, &t.Description, &mode, &parentID, &roles,
		&t.NameFormat, &t.WelcomeMessage, &t.MaxOpenPerUser, &t.Questions, &t.CreatedAt)
	if err != nil {
		return t, notFound(err)
	}
	t.GuildID = snowflake.ID(guildID)
	t.Mode = TicketMode(mode)
	t.ParentID = idFromNullable(parentID)
	t.SupportRoleIDs = fromInt64s(roles)
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
	return s.pool.QueryRow(ctx, `
		INSERT INTO ticket_types (guild_id, name, emoji, description, mode, parent_id, support_role_ids,
		                          name_format, welcome_message, max_open_per_user, questions)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at`,
		int64(t.GuildID), t.Name, t.Emoji, t.Description, string(t.Mode), nullableID(t.ParentID),
		toInt64s(t.SupportRoleIDs), t.NameFormat, t.WelcomeMessage, t.MaxOpenPerUser, t.Questions,
	).Scan(&t.ID, &t.CreatedAt)
}

func (s *Store) UpdateTicketType(ctx context.Context, t TicketType) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ticket_types
		SET name = $3, emoji = $4, description = $5, mode = $6, parent_id = $7, support_role_ids = $8,
		    name_format = $9, welcome_message = $10, max_open_per_user = $11, questions = $12, updated_at = now()
		WHERE guild_id = $1 AND id = $2`,
		int64(t.GuildID), t.ID, t.Name, t.Emoji, t.Description, string(t.Mode), nullableID(t.ParentID),
		toInt64s(t.SupportRoleIDs), t.NameFormat, t.WelcomeMessage, t.MaxOpenPerUser, questionsOrEmpty(t.Questions))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTicketType(ctx context.Context, guildID snowflake.ID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM ticket_types WHERE guild_id = $1 AND id = $2`, int64(guildID), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
