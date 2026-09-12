package ticketbot

import (
	"slices"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestCheckMove(t *testing.T) {
	general := int64(1)
	ticket := store.Ticket{TicketTypeID: &general, Mode: store.ModeChannel, Status: store.StatusOpen}
	billing := store.TicketType{ID: 2, Name: "Billing", Mode: store.ModeChannel}

	if err := checkMove(ticket, billing); err != nil {
		t.Errorf("move to billing: %v", err)
	}
	for name, tc := range map[string]struct {
		t  store.Ticket
		to store.TicketType
	}{
		"same type":    {ticket, store.TicketType{ID: general, Mode: store.ModeChannel}},
		"other mode":   {ticket, store.TicketType{ID: 3, Mode: store.ModeThread}},
		"closed":       {store.Ticket{TicketTypeID: &general, Mode: store.ModeChannel, Status: store.StatusClosed}, billing},
		"deleted type": {store.Ticket{Mode: store.ModeThread, Status: store.StatusOpen}, billing},
	} {
		if _, ok := UserMessage(checkMove(tc.t, tc.to)); !ok {
			t.Errorf("%s: want a user error", name)
		}
	}
	// A ticket whose type was deleted can still move.
	if err := checkMove(store.Ticket{Mode: store.ModeChannel, Status: store.StatusOpen}, billing); err != nil {
		t.Errorf("move from deleted type: %v", err)
	}
}

func TestRoleChanges(t *testing.T) {
	added, removed := roleChanges([]snowflake.ID{1, 2}, []snowflake.ID{2, 3})
	if !slices.Equal(added, []snowflake.ID{3}) || !slices.Equal(removed, []snowflake.ID{1}) {
		t.Errorf("added %v, removed %v", added, removed)
	}
	if added, removed := roleChanges(nil, []snowflake.ID{4}); !slices.Equal(added, []snowflake.ID{4}) || removed != nil {
		t.Errorf("from no roles: added %v, removed %v", added, removed)
	}
}

func TestMoveChoices(t *testing.T) {
	general := int64(1)
	types := []store.TicketType{
		{ID: 1, Name: "General", Mode: store.ModeChannel},
		{ID: 2, Name: "Billing", Mode: store.ModeChannel},
		{ID: 3, Name: "Bug reports", Mode: store.ModeThread},
	}
	ticket := store.Ticket{TicketTypeID: &general, Mode: store.ModeChannel, Status: store.StatusOpen}

	names := func(cs []discord.AutocompleteChoice) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.(discord.AutocompleteChoiceString).Name)
		}
		return out
	}
	// Only types the ticket can move to, matching what's typed.
	if got := names(moveChoices(types, ticket, true, "")); !slices.Equal(got, []string{"Billing"}) {
		t.Errorf("in ticket = %v", got)
	}
	if got := names(moveChoices(types, ticket, false, "b")); !slices.Equal(got, []string{"Billing", "Bug reports"}) {
		t.Errorf("outside a ticket = %v", got)
	}
	if got := moveChoices(types, ticket, true, "zzz"); got == nil || len(got) != 0 {
		t.Errorf("no match = %#v, want an empty list", got)
	}
}

func TestMovedMessage(t *testing.T) {
	msg := movedMessage("General", "Billing", 1, []snowflake.ID{10})
	if want := "🔀 <@1> moved this ticket from **General** to **Billing**. <@&10> can help from here."; msg.Content != want {
		t.Errorf("content = %q", msg.Content)
	}
	if !slices.Equal(msg.AllowedMentions.Roles, []snowflake.ID{10}) || len(msg.AllowedMentions.Users) != 0 {
		t.Errorf("allowed mentions = %+v", msg.AllowedMentions)
	}
	if msg := movedMessage("General", "Billing", 1, nil); len(msg.AllowedMentions.Roles) != 0 {
		t.Errorf("no new roles should ping nobody: %+v", msg.AllowedMentions)
	}
}
