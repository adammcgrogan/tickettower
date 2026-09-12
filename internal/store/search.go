package store

import (
	"context"
	"strings"
	"unicode"
)

// Snippet markers wrap the matching words in MessageMatch.Snippet. They're
// private-use characters, so they can't clash with what someone typed.
const (
	MatchStart = ""
	MatchEnd   = ""
)

// MessageMatch is the message that made a ticket match a search.
type MessageMatch struct {
	AuthorName string `json:"author_name"`
	// Snippet is the part of the message around the matching words, with
	// each match wrapped in MatchStart and MatchEnd.
	Snippet string `json:"snippet"`
}

// searchQuery turns what someone typed into a tsquery string: every word
// must appear, and the last word may be the start of a longer one, so "refun"
// finds "refund". Returns "" if nothing searchable was typed.
func searchQuery(typed string) string {
	var terms []string
	for _, w := range strings.FieldsFunc(typed, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		terms = append(terms, w+":*")
	}
	return strings.Join(terms, " & ")
}

// messageMatches finds, for each ticket, the newest message matching query,
// with a snippet around the matching words.
func (s *Store) messageMatches(ctx context.Context, ticketIDs []int64, query string) (map[int64]MessageMatch, error) {
	out := map[int64]MessageMatch{}
	if len(ticketIDs) == 0 || query == "" {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (ticket_id) ticket_id, author_name,
		       ts_headline('simple', ticket_message_text(content, embeds), to_tsquery('simple', $2), $3)
		FROM ticket_messages
		WHERE ticket_id = ANY($1) AND deleted_at IS NULL AND search @@ to_tsquery('simple', $2)
		ORDER BY ticket_id, id DESC`,
		ticketIDs, query,
		"MaxFragments=1, MaxWords=16, MinWords=8, StartSel="+MatchStart+", StopSel="+MatchEnd+", FragmentDelimiter=…")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id int64
			m  MessageMatch
		)
		if err := rows.Scan(&id, &m.AuthorName, &m.Snippet); err != nil {
			return nil, err
		}
		m.Snippet = strings.Join(strings.Fields(m.Snippet), " ")
		out[id] = m
	}
	return out, rows.Err()
}
