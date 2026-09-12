package ticketbot

import (
	"strings"
	"testing"

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
