package api

import (
	"net/url"
	"testing"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestTicketQuery(t *testing.T) {
	q := ticketQuery(url.Values{
		"status": {"closed"}, "type": {"7"}, "before": {"99"}, "limit": {"50"}, "q": {"  #0042 "},
	})
	if q.Status != store.StatusClosed || q.TypeID == nil || *q.TypeID != 7 || q.Before == nil || *q.Before != 99 ||
		q.Limit != 50 || q.Search != "42" {
		t.Errorf("parsed %+v", q)
	}

	// Anything unrecognised falls back to the defaults.
	q = ticketQuery(url.Values{"status": {"bogus"}, "type": {"x"}, "before": {"-"}, "limit": {"5000"}})
	if q.Status != "" || q.TypeID != nil || q.Before != nil || q.Limit != maxTicketPage || q.Search != "" {
		t.Errorf("defaults %+v", q)
	}

	// Only a leading # marks a ticket number; names keep their zeros.
	if q := ticketQuery(url.Values{"q": {"007bond"}}); q.Search != "007bond" {
		t.Errorf("search = %q, want 007bond", q.Search)
	}
}
