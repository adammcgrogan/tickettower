package ticketbot

import (
	"testing"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestWeekOverWeek(t *testing.T) {
	cases := []struct {
		cur, prev int
		want      string
	}{
		{0, 0, ""},
		{5, 3, " (+2)"},
		{3, 5, " (-2)"},
		{4, 4, " (same as last week)"},
	}
	for _, c := range cases {
		if got := weekOverWeek(c.cur, c.prev); got != c.want {
			t.Errorf("weekOverWeek(%d, %d) = %q, want %q", c.cur, c.prev, got, c.want)
		}
	}
}

func TestMedianLabel(t *testing.T) {
	if got := medianLabel(nil); got != "—" {
		t.Errorf("nil = %q, want a dash", got)
	}
	sec := 125.0
	if got := medianLabel(&sec); got != "2m" {
		t.Errorf("medianLabel(125s) = %q, want %q", got, "2m")
	}
}

func TestSatisfactionLabel(t *testing.T) {
	if got := satisfactionLabel(nil, 0); got != "No ratings yet" {
		t.Errorf("no ratings = %q", got)
	}
	avg := 4.5
	if got := satisfactionLabel(&avg, 1); got != "4.5/5 (1 rating)" {
		t.Errorf("one rating = %q", got)
	}
	if got := satisfactionLabel(&avg, 3); got != "4.5/5 (3 ratings)" {
		t.Errorf("several ratings = %q", got)
	}
}

func TestBusiestDay(t *testing.T) {
	series := []store.SeriesPoint{
		{Date: "2026-01-05", Opened: 1, Closed: 0}, // Monday
		{Date: "2026-01-06", Opened: 3, Closed: 2}, // Tuesday, busiest
		{Date: "2026-01-07", Opened: 0, Closed: 1},
	}
	if got := busiestDay(series); got != "Tuesday" {
		t.Errorf("busiestDay = %q, want Tuesday", got)
	}
	if got := busiestDay(nil); got != "" {
		t.Errorf("busiestDay(nil) = %q, want empty", got)
	}
}
