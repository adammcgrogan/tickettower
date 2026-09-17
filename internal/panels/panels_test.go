package panels

import (
	"fmt"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
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

func TestCustomEmojiID(t *testing.T) {
	tests := map[string]snowflake.ID{
		"<:help:123456789012345678>":    123456789012345678,
		" <a:wave:123456789012345678> ": 123456789012345678,
		"🎫":                             0,
		"":                              0,
		"<:bad:12>":                     0,
	}
	for in, want := range tests {
		if got := CustomEmojiID(in); got != want {
			t.Errorf("CustomEmojiID(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestButtonStylesLabelsAndImages(t *testing.T) {
	p := store.Panel{Title: "Help", Color: 1, TicketTypeIDs: []int64{1, 2, 3}, ImageURL: "https://x/banner.png", ThumbnailURL: "https://x/logo.png"}
	types := []store.TicketType{
		{ID: 1, Name: "General support", ButtonLabel: "Get help", ButtonStyle: store.ButtonSuccess},
		{ID: 2, Name: "Report", ButtonStyle: store.ButtonDanger},
		{ID: 3, Name: "Other"}, // no style saved: primary
	}
	msg := Create("Powered by App", p, types)
	row := msg.Components[0].(discord.ActionRowComponent)
	got := []struct {
		label string
		style discord.ButtonStyle
	}{}
	for _, c := range row.Components {
		b := c.(discord.ButtonComponent)
		got = append(got, struct {
			label string
			style discord.ButtonStyle
		}{b.Label, b.Style})
	}
	want := []struct {
		label string
		style discord.ButtonStyle
	}{
		{"Get help", discord.ButtonStyleSuccess},
		{"Report", discord.ButtonStyleDanger},
		{"Other", discord.ButtonStylePrimary},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("button %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	e := msg.Embeds[0]
	if e.Image == nil || e.Image.URL != p.ImageURL || e.Thumbnail == nil || e.Thumbnail.URL != p.ThumbnailURL {
		t.Errorf("embed images = %+v / %+v", e.Image, e.Thumbnail)
	}
	if e.Footer == nil || e.Footer.Text != "Powered by App" {
		t.Errorf("footer = %+v, want branding", e.Footer)
	}
	if unbranded := Create("", p, types).Embeds[0]; unbranded.Footer != nil {
		t.Errorf("footer = %+v, want none for premium", unbranded.Footer)
	}

	p.Style, p.Placeholder = store.PanelDropdown, "What do you need?"
	menu := Create("Powered by App", p, types).Components[0].(discord.ActionRowComponent).Components[0].(discord.StringSelectMenuComponent)
	if menu.Placeholder != "What do you need?" || menu.Options[0].Label != "Get help" {
		t.Errorf("dropdown = %q, first option %q", menu.Placeholder, menu.Options[0].Label)
	}
	p.Placeholder = ""
	menu = Create("Powered by App", p, types).Components[0].(discord.ActionRowComponent).Components[0].(discord.StringSelectMenuComponent)
	if menu.Placeholder != DefaultPlaceholder {
		t.Errorf("default placeholder = %q", menu.Placeholder)
	}
}

func TestFooter(t *testing.T) {
	for url, want := range map[string]string{
		"https://tickettower.net":     "Powered by Ticket Tower · tickettower.net",
		"https://www.tickettower.net": "Powered by Ticket Tower · tickettower.net",
		"http://localhost:5173":       "Powered by Ticket Tower",
		"":                            "Powered by Ticket Tower",
	} {
		if got := Footer("Ticket Tower", url); got != want {
			t.Errorf("Footer(%q) = %q, want %q", url, got, want)
		}
	}
}
