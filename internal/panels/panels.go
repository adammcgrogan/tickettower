// Package panels renders ticket panel messages. The API uses it to publish
// panels; the bot handles the interactions they produce.
package panels

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

// Custom IDs for panel components. Button IDs are followed by the ticket
// type ID.
const (
	OpenButtonPrefix = "/panel/open/"
	OpenSelectID     = "/panel/select"
)

// MaxTypesPerPanel is Discord's limit on buttons per message (5 rows of 5)
// and options per select menu.
const MaxTypesPerPanel = 25

var customEmoji = regexp.MustCompile(`^<(a?):([A-Za-z0-9_]{2,32}):(\d{15,21})>$`)

// CustomEmojiID returns the ID of a custom emoji (<:name:id>), or 0 for a
// unicode emoji or an empty string.
func CustomEmojiID(s string) snowflake.ID {
	if m := customEmoji.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		id, _ := snowflake.Parse(m[3])
		return id
	}
	return 0
}

// ParseEmoji converts a unicode emoji or a custom emoji (<:name:id>) into a
// component emoji. It returns nil for an empty string.
func ParseEmoji(s string) *discord.ComponentEmoji {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if m := customEmoji.FindStringSubmatch(s); m != nil {
		id, _ := snowflake.Parse(m[3])
		return &discord.ComponentEmoji{ID: id, Name: m[2], Animated: m[1] == "a"}
	}
	return &discord.ComponentEmoji{Name: s}
}

// IsValidEmoji reports whether s looks like a single emoji or a custom emoji.
func IsValidEmoji(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || customEmoji.MatchString(s) {
		return true
	}
	// Unicode emoji can be several code points (skin tones, ZWJ sequences),
	// but are never long or plain ASCII.
	n := len([]rune(s))
	return n <= 10 && !strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 <>:")
}

// Create renders the panel as a new message. branding adds a "Powered by"
// footer, which free-tier guilds get and premium ones don't.
func Create(appName string, branding bool, p store.Panel, types []store.TicketType) discord.MessageCreate {
	return discord.NewMessageCreate().
		WithEmbeds(embed(appName, branding, p)).
		WithComponents(components(p, types)...)
}

// Update renders the panel as an edit to its existing message.
func Update(appName string, branding bool, p store.Panel, types []store.TicketType) discord.MessageUpdate {
	return discord.NewMessageUpdate().
		WithEmbeds(embed(appName, branding, p)).
		WithComponents(components(p, types)...)
}

// Ordered returns the panel's ticket types in display order, skipping any
// that no longer exist.
func Ordered(p store.Panel, types []store.TicketType) []store.TicketType {
	byID := make(map[int64]store.TicketType, len(types))
	for _, t := range types {
		byID[t.ID] = t
	}
	out := make([]store.TicketType, 0, len(p.TicketTypeIDs))
	for _, id := range p.TicketTypeIDs {
		if t, ok := byID[id]; ok {
			out = append(out, t)
		}
	}
	return out
}

// DefaultPlaceholder is the dropdown's prompt when a panel doesn't set one.
const DefaultPlaceholder = "Choose a topic…"

func embed(appName string, branding bool, p store.Panel) discord.Embed {
	e := discord.NewEmbed().WithTitle(p.Title).WithColor(p.Color)
	if branding {
		e = e.WithFooterText("Powered by " + appName)
	}
	if p.Description != "" {
		e = e.WithDescription(p.Description)
	}
	if p.ImageURL != "" {
		e = e.WithImage(p.ImageURL)
	}
	if p.ThumbnailURL != "" {
		e = e.WithThumbnail(p.ThumbnailURL)
	}
	return e
}

// button renders a ticket type's button in its chosen colour.
func button(t store.TicketType) discord.ButtonComponent {
	id := OpenButtonPrefix + strconv.FormatInt(t.ID, 10)
	label := truncate(t.Label(), 80)
	var b discord.ButtonComponent
	switch t.ButtonStyle {
	case store.ButtonSecondary:
		b = discord.NewSecondaryButton(label, id)
	case store.ButtonSuccess:
		b = discord.NewSuccessButton(label, id)
	case store.ButtonDanger:
		b = discord.NewDangerButton(label, id)
	default:
		b = discord.NewPrimaryButton(label, id)
	}
	if e := ParseEmoji(t.Emoji); e != nil {
		b = b.WithEmoji(*e)
	}
	return b
}

func components(p store.Panel, all []store.TicketType) []discord.LayoutComponent {
	types := Ordered(p, all)
	if len(types) > MaxTypesPerPanel {
		types = types[:MaxTypesPerPanel]
	}
	if len(types) == 0 {
		return nil
	}

	if p.Style == store.PanelDropdown {
		options := make([]discord.StringSelectMenuOption, 0, len(types))
		for _, t := range types {
			o := discord.NewStringSelectMenuOption(truncate(t.Label(), 100), strconv.FormatInt(t.ID, 10))
			if t.Description != "" {
				o = o.WithDescription(truncate(t.Description, 100))
			}
			if e := ParseEmoji(t.Emoji); e != nil {
				o = o.WithEmoji(*e)
			}
			options = append(options, o)
		}
		placeholder := p.Placeholder
		if placeholder == "" {
			placeholder = DefaultPlaceholder
		}
		return []discord.LayoutComponent{
			discord.NewActionRow(discord.NewStringSelectMenu(OpenSelectID, truncate(placeholder, 150), options...)),
		}
	}

	var rows []discord.LayoutComponent
	var row []discord.InteractiveComponent
	for i, t := range types {
		row = append(row, button(t))
		if len(row) == 5 || i == len(types)-1 {
			rows = append(rows, discord.NewActionRow(row...))
			row = nil
		}
	}
	return rows
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
