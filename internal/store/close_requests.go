package store

import (
	"context"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// CloseRequest is a pending request from staff for the member to confirm
// their ticket can be closed.
type CloseRequest struct {
	TicketID int64
	By       snowflake.ID
	ByName   string
	Reason   string
	// ClosesAt is when the ticket closes by itself if the member doesn't
	// answer; nil means it stays open until they do.
	ClosesAt *time.Time
}

// RequestClose records a close request on an open ticket, replacing any
// pending one. It reports false if the ticket is closed.
func (s *Store) RequestClose(ctx context.Context, r CloseRequest) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets
		SET close_request_by = $2, close_request_by_name = $3, close_request_reason = $4, close_request_closes_at = $5
		WHERE id = $1 AND status = 'open'`, r.TicketID, int64(r.By), r.ByName, r.Reason, r.ClosesAt)
	return tag.RowsAffected() == 1, err
}

// GetCloseRequest returns a ticket's pending close request, or ErrNotFound.
func (s *Store) GetCloseRequest(ctx context.Context, ticketID int64) (CloseRequest, error) {
	r := CloseRequest{TicketID: ticketID}
	var by int64
	err := s.pool.QueryRow(ctx, `
		SELECT close_request_by, close_request_by_name, close_request_reason, close_request_closes_at
		FROM tickets WHERE id = $1 AND status = 'open' AND close_request_by IS NOT NULL`, ticketID).
		Scan(&by, &r.ByName, &r.Reason, &r.ClosesAt)
	r.By = snowflake.ID(by)
	return r, notFound(err)
}

// ClearCloseRequest withdraws a ticket's close request (the member kept it
// open). It reports whether there was one.
func (s *Store) ClearCloseRequest(ctx context.Context, ticketID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets SET close_request_by = NULL, close_request_by_name = '', close_request_reason = '',
		                   close_request_closes_at = NULL
		WHERE id = $1 AND close_request_by IS NOT NULL`, ticketID)
	return tag.RowsAffected() == 1, err
}

// CloseRequestsDue returns the pending close requests the member never
// answered, whose time is up.
func (s *Store) CloseRequestsDue(ctx context.Context, now time.Time) ([]CloseRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.close_request_by, t.close_request_by_name, t.close_request_reason, t.close_request_closes_at
		FROM tickets t
		WHERE t.status = 'open' AND t.close_request_closes_at <= $1 AND `+inActiveGuild+`
		ORDER BY t.close_request_closes_at LIMIT 100`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CloseRequest
	for rows.Next() {
		var r CloseRequest
		var by int64
		if err := rows.Scan(&r.TicketID, &by, &r.ByName, &r.Reason, &r.ClosesAt); err != nil {
			return nil, err
		}
		r.By = snowflake.ID(by)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CloseUnansweredRequest closes a ticket whose close request went
// unanswered, on behalf of the staff member who asked, rechecking that it's
// still due. It reports false if the member answered in the meantime.
func (s *Store) CloseUnansweredRequest(ctx context.Context, ticketID int64, now time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE tickets
		SET status = 'closed', closed_by = close_request_by, closed_by_name = close_request_by_name,
		    close_reason = close_request_reason, closed_at = $2
		WHERE id = $1 AND status = 'open' AND close_request_closes_at IS NOT NULL AND close_request_closes_at <= $2`,
		ticketID, now)
	return tag.RowsAffected() == 1, err
}
