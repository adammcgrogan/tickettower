package ticketbot

import (
	"slices"
	"testing"

	"github.com/disgoorg/disgo/discord"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestCheckMerge(t *testing.T) {
	open := store.Ticket{ID: 1, Number: 1, OpenerID: 42, Status: store.StatusOpen}
	target := store.Ticket{ID: 2, Number: 2, OpenerID: 42, Status: store.StatusOpen}

	if err := checkMerge(open, target); err != nil {
		t.Errorf("merge into another open ticket, same opener: %v", err)
	}
	for name, tc := range map[string]struct{ t, to store.Ticket }{
		"source closed": {store.Ticket{ID: 1, OpenerID: 42, Status: store.StatusClosed}, target},
		"target closed": {open, store.Ticket{ID: 2, OpenerID: 42, Status: store.StatusClosed}},
		"same ticket":   {open, open},
		"other opener":  {open, store.Ticket{ID: 2, OpenerID: 43, Status: store.StatusOpen}},
	} {
		if _, ok := UserMessage(checkMerge(tc.t, tc.to)); !ok {
			t.Errorf("%s: want a user error", name)
		}
	}
}

func TestMergeChoices(t *testing.T) {
	tickets := []store.Ticket{
		{ID: 1, Number: 10, TypeName: "General"},
		{ID: 2, Number: 11, TypeName: "Billing"},
	}
	names := func(cs []discord.AutocompleteChoice) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.(discord.AutocompleteChoiceString).Name)
		}
		return out
	}
	if got := names(mergeChoices(tickets, "")); !slices.Equal(got, []string{"#10 — General", "#11 — Billing"}) {
		t.Errorf("no filter = %v", got)
	}
	if got := names(mergeChoices(tickets, "bill")); !slices.Equal(got, []string{"#11 — Billing"}) {
		t.Errorf("filtered = %v", got)
	}
	if got := mergeChoices(tickets, "zzz"); got == nil || len(got) != 0 {
		t.Errorf("no match = %#v, want an empty list", got)
	}
}

func TestMergedArrivalMessage(t *testing.T) {
	from := store.Ticket{Number: 5, TypeName: "General", OpenerID: 42}
	msg := mergedArrivalMessage(from, 1)
	want := "🔀 <@1> merged ticket #5 (General) into this one. <@42> can continue here."
	if msg.Content != want {
		t.Errorf("content = %q, want %q", msg.Content, want)
	}
}

func TestTranscriptSuffix(t *testing.T) {
	if got := transcriptSuffix("http://localhost:8081", 1); got != "" {
		t.Errorf("non-https public URL should give no link, got %q", got)
	}
	if got := transcriptSuffix("https://tickettower.net", 42); got != " Transcript: https://tickettower.net/transcripts/42" {
		t.Errorf("got %q", got)
	}
}
