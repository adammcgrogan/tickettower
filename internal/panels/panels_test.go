package panels

import (
	"fmt"
	"testing"

	"github.com/disgoorg/disgo/discord"

	"github.com/adammcgrogan/ticketsbot/internal/store"
)

func TestParseEmoji(t *testing.T) {
	if ParseEmoji("") != nil {
		t.Error("empty string should give nil")
	}
	if e := ParseEmoji("🎫"); e.Name != "🎫" || e.ID != 0 {
		t.Errorf("unicode: %+v", e)
	}
	e := ParseEmoji("<a:party:123456789012345678>")
	if e.Name != "party" || e.ID != 123456789012345678 || !e.Animated {
		t.Errorf("custom: %+v", e)
	}
}

func TestIsValidEmoji(t *testing.T) {
	for _, s := range []string{"", "🎫", "👍🏽", "<:name:123456789012345678>"} {
		if !IsValidEmoji(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"ticket", ":ticket:", "<:bad>", "🎫 help"} {
		if IsValidEmoji(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func makeTypes(n int) ([]store.TicketType, []int64) {
	var types []store.TicketType
	var ids []int64
	for i := 1; i <= n; i++ {
		types = append(types, store.TicketType{ID: int64(i), Name: fmt.Sprintf("Type %d", i)})
		ids = append(ids, int64(i))
	}
	return types, ids
}

func TestButtonsWrapAtFivePerRow(t *testing.T) {
	types, ids := makeTypes(12)
	rows := components(store.Panel{Style: store.PanelButtons, TicketTypeIDs: ids}, types)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	last := rows[2].(discord.ActionRowComponent)
	if len(last.Components) != 2 {
		t.Errorf("last row has %d buttons, want 2", len(last.Components))
	}
}

func TestDropdownAndOrdering(t *testing.T) {
	types, _ := makeTypes(3)
	p := store.Panel{Style: store.PanelDropdown, TicketTypeIDs: []int64{3, 99, 1}}

	ordered := Ordered(p, types)
	if len(ordered) != 2 || ordered[0].ID != 3 || ordered[1].ID != 1 {
		t.Fatalf("ordered = %+v", ordered)
	}

	rows := components(p, types)
	row := rows[0].(discord.ActionRowComponent)
	menu := row.Components[0].(discord.StringSelectMenuComponent)
	if menu.CustomID != OpenSelectID || len(menu.Options) != 2 || menu.Options[0].Value != "3" {
		t.Errorf("menu = %+v", menu)
	}
}

func TestNoTypesMeansNoComponents(t *testing.T) {
	if c := components(store.Panel{Style: store.PanelButtons}, nil); c != nil {
		t.Errorf("components = %v, want nil", c)
	}
}
