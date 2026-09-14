package ticketbot

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestChannelName(t *testing.T) {
	user := discord.User{Username: "adam"}
	answers := []formAnswer{{"Order number", "#12 345"}, {"What happened?", ""}}
	tests := []struct{ format, want string }{
		{"ticket-{number}", "ticket-0042"},
		{"{username}-{number}", "adam-0042"},
		{"{user}-{number}", "adam-0042"},
		{"{type}-{number}", "billing-refunds-0042"},
		{"order-{answer1}", "order-12-345"},
		{"{answer2}-{number}", "0042"},     // an empty answer leaves no stray hyphen
		{"{answer5}", "ticket-0042"},       // no such question
		{"{nope}-{number}", "{nope}-0042"}, // unknown placeholders are left alone
	}
	for _, tc := range tests {
		if got := channelName(tc.format, 42, user, "Billing & Refunds 💳", answers); got != tc.want {
			t.Errorf("channelName(%q) = %q, want %q", tc.format, got, tc.want)
		}
	}

	// A member's answer can't smuggle in another placeholder.
	if got := channelName("{answer1}", 42, user, "Help", []formAnswer{{"Q", "{number}"}}); got != "number" {
		t.Errorf("injected placeholder = %q", got)
	}
}

func TestWelcomeText(t *testing.T) {
	ticket := store.Ticket{Number: 42, OpenerID: 1, OpenerName: "Adam"}
	tt := store.TicketType{
		Name:           "Billing",
		SupportRoleIDs: []snowflake.ID{10, 20},
		WelcomeMessage: "Hi {user} ({username}), this is {type} ticket #{number} in {server}. {support} will help. Order: {answer1}{answer3}",
	}
	answers := []formAnswer{{"Order number", " #1234 "}, {"Details", "{user}"}}

	got := welcomeText(ticket, tt, answers, "Tower HQ")
	want := "Hi <@1> (Adam), this is Billing ticket #0042 in Tower HQ. <@&10> <@&20> will help. Order: #1234"
	if got != want {
		t.Errorf("welcomeText =\n%q\nwant\n%q", got, want)
	}

	tt.SupportRoleIDs, tt.WelcomeMessage = nil, "{support} will reply. {answer2}"
	if got := welcomeText(ticket, tt, answers, ""); got != "the team will reply. {user}" {
		t.Errorf("without roles = %q", got)
	}
}

func TestKeepsAccess(t *testing.T) {
	tt := &store.TicketType{SupportRoleIDs: []snowflake.ID{10, 20}}
	member := func(perms discord.Permissions, roles ...snowflake.ID) *discord.ResolvedMember {
		return &discord.ResolvedMember{Member: discord.Member{RoleIDs: roles}, Permissions: perms}
	}
	if err := keepsAccess(1, member(discord.PermissionViewChannel, 30), tt); err != nil {
		t.Errorf("plain member: %v", err)
	}
	if err := keepsAccess(1, nil, tt); err != nil {
		t.Errorf("member who left: %v", err)
	}
	if err := keepsAccess(1, member(discord.PermissionViewChannel, 30), nil); err != nil {
		t.Errorf("deleted type: %v", err)
	}
	err := keepsAccess(1, member(discord.PermissionAdministrator), tt)
	if err == nil || !strings.Contains(err.Error(), "administrator") {
		t.Errorf("admin: %v", err)
	}
	err = keepsAccess(1, member(discord.PermissionViewChannel, 30, 20), tt)
	if err == nil || !strings.Contains(err.Error(), discord.RoleMention(20)) {
		t.Errorf("support role: %v", err)
	}
}

func TestClosedDM(t *testing.T) {
	claimer := "Sam"
	tk := store.Ticket{Number: 7, TypeName: "Billing", CloseReason: "Refunded", ClaimedByName: &claimer, OpenerName: "jo"}

	desc, ask := closedDM(tk, nil, "Acme")
	if !ask || !strings.Contains(desc, "**Reason:** Refunded") || !strings.HasSuffix(desc, defaultRatingPrompt) {
		t.Errorf("deleted type: ask=%v desc=%q", ask, desc)
	}
	desc, ask = closedDM(tk, &store.TicketType{AskRating: true}, "Acme")
	if !ask || !strings.HasSuffix(desc, defaultRatingPrompt) {
		t.Errorf("default prompt: ask=%v desc=%q", ask, desc)
	}
	desc, ask = closedDM(tk, &store.TicketType{AskRating: false, RatingPrompt: "ignored"}, "Acme")
	if ask || strings.Contains(desc, "ignored") || !strings.HasSuffix(desc, "**Reason:** Refunded") {
		t.Errorf("ratings off: ask=%v desc=%q", ask, desc)
	}
	desc, ask = closedDM(tk, &store.TicketType{AskRating: true, RatingPrompt: "How did {staff} do with #{number} in {server}?"}, "Acme")
	if !ask || !strings.HasSuffix(desc, "How did Sam do with #0007 in Acme?") {
		t.Errorf("custom prompt: ask=%v desc=%q", ask, desc)
	}
	tk.ClaimedByName = nil
	desc, _ = closedDM(tk, &store.TicketType{AskRating: true, RatingPrompt: "Rate {staff}"}, "Acme")
	if !strings.HasSuffix(desc, "Rate the team") {
		t.Errorf("unclaimed staff placeholder: %q", desc)
	}
}

// A welcome message with long answers substituted in stays within Discord's
// limits: 4,096 characters of description, 1,024 per field and 6,000 in all.
func TestWelcomeMessageFitsEmbedLimits(t *testing.T) {
	long := strings.Repeat("x", store.MaxParagraphAnswer)
	ticket := store.Ticket{Number: 7, OpenerID: 1, OpenerName: "Adam"}
	tt := store.TicketType{Name: "Support", WelcomeMessage: strings.Repeat("w", 2000) + " {answer1} {answer2} {answer3}"}
	answers := []formAnswer{{"One", long}, {"Two", long}, {"Three", long + "extra"}}

	msg := welcomeMessage(ticket, tt, answers, "HQ")
	embed := msg.Embeds[0]
	total := utf8.RuneCountInString(embed.Title) + utf8.RuneCountInString(embed.Description) + utf8.RuneCountInString(embed.Footer.Text)
	if n := utf8.RuneCountInString(embed.Description); n > embedDescriptionLimit {
		t.Errorf("description is %d characters", n)
	}
	for _, f := range embed.Fields {
		if n := utf8.RuneCountInString(f.Value); n > embedFieldValueLimit {
			t.Errorf("field %q is %d characters", f.Name, n)
		}
		total += utf8.RuneCountInString(f.Name) + utf8.RuneCountInString(f.Value)
	}
	if total > embedLimit {
		t.Errorf("embed totals %d characters", total)
	}
	if len(msg.Components) != 1 {
		t.Errorf("components = %d, want the Claim/Close row", len(msg.Components))
	}

	fb := welcomeFallback(ticket, tt)
	if len(fb.Components) != 1 || fb.Content == "" {
		t.Errorf("fallback has no buttons or mentions: %+v", fb)
	}
}
