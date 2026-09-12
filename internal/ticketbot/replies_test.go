package ticketbot

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestReplyText(t *testing.T) {
	ticket := store.Ticket{OpenerID: 42, OpenerName: "Sam", Number: 7, TypeName: "Billing"}
	got := replyText("Hi {user} ({username}), #{number} is a {type} ticket in {server}. {staff}, {unknown}", ticket, "Acme", "Adam")
	want := "Hi <@42> (Sam), #0007 is a Billing ticket in Acme. Adam, {unknown}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Placeholders are filled in one pass, so a name can't inject another.
	ticket.OpenerName = "{server}"
	if got := replyText("{username}", ticket, "Acme", "Adam"); got != "{server}" {
		t.Errorf("injected = %q", got)
	}

	// A long reply is cut to fit an embed.
	long := replyText(strings.Repeat("{server}", 2000), ticket, strings.Repeat("a", 10), "Adam")
	if n := utf8.RuneCountInString(long); n != embedDescriptionLimit {
		t.Errorf("long reply is %d characters, want %d", n, embedDescriptionLimit)
	}
}

func TestFindSavedReply(t *testing.T) {
	if _, err := findSavedReply(nil, "refund"); err == nil || !strings.Contains(err.Error(), "no saved replies yet") {
		t.Errorf("no replies err = %v", err)
	}
	replies := []store.SavedReply{{ID: 3, Name: "Refund policy"}, {ID: 9, Name: "Appeals"}}
	for _, value := range []string{"9", "appeals", "  APPEALS "} {
		if r, err := findSavedReply(replies, value); err != nil || r.ID != 9 {
			t.Errorf("%q = %+v, %v", value, r, err)
		}
	}
	if _, err := findSavedReply(replies, "refund"); err == nil || !strings.Contains(err.Error(), `called "refund"`) {
		t.Errorf("unknown name err = %v", err)
	}
}

func TestReplyChoices(t *testing.T) {
	replies := []store.SavedReply{{ID: 3, Name: "Refund policy"}, {ID: 9, Name: "Appeals"}, {ID: 4, Name: "Refund status"}}
	got := replyChoices(replies, " REFUND")
	if len(got) != 2 {
		t.Fatalf("choices = %+v", got)
	}
	if c := got[1].(discord.AutocompleteChoiceString); c.Name != "Refund status" || c.Value != "4" {
		t.Errorf("second choice = %+v", c)
	}
	if got := replyChoices(replies, ""); len(got) != 3 {
		t.Errorf("empty search = %d choices, want all 3", len(got))
	}

	// Discord shows at most 25.
	many := make([]store.SavedReply, 30)
	for i := range many {
		many[i] = store.SavedReply{ID: int64(i + 1), Name: fmt.Sprintf("Reply %d", i+1)}
	}
	if got := replyChoices(many, ""); len(got) != 25 {
		t.Errorf("got %d choices, want 25", len(got))
	}
}
