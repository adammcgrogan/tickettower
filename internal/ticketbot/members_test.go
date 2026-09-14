package ticketbot

import (
	"slices"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestChannelMemberIDs(t *testing.T) {
	overwrites := discord.PermissionOverwrites{
		discord.RolePermissionOverwrite{RoleID: 100, Deny: discord.PermissionViewChannel},
		discord.MemberPermissionOverwrite{UserID: 1, Allow: ticketMemberPerms}, // the bot
		discord.MemberPermissionOverwrite{UserID: 2, Allow: ticketMemberPerms}, // the opener
		discord.RolePermissionOverwrite{RoleID: 10, Allow: ticketMemberPerms},  // a support role
		discord.MemberPermissionOverwrite{UserID: 3, Allow: ticketMemberPerms}, // added with /ticket add
		discord.MemberPermissionOverwrite{UserID: 4, Deny: discord.PermissionSendMessages},
	}
	if got := channelMemberIDs(overwrites, 1); !slices.Equal(got, []snowflake.ID{2, 3}) {
		t.Errorf("members = %v, want the opener and the added member", got)
	}
}

func TestSortMembers(t *testing.T) {
	ms := []TicketMember{{ID: 1, Name: "zed"}, {ID: 2, Name: "Amy"}, {ID: 3, Name: "Sam", Opener: true}, {ID: 4, Name: "bob"}}
	sortMembers(ms)
	var got []string
	for _, m := range ms {
		got = append(got, m.Name)
	}
	if want := []string{"Sam", "Amy", "bob", "zed"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestMemberMessages(t *testing.T) {
	added := addedMessage(1, 2)
	if added.Content != "<@1> added <@2> to this ticket." || !slices.Equal(added.AllowedMentions.Users, []snowflake.ID{2}) {
		t.Errorf("added: %q, mentions %+v", added.Content, added.AllowedMentions)
	}
	removed := removedMessage(1, 2)
	if removed.Content != "<@1> removed <@2> from this ticket." || len(removed.AllowedMentions.Users) != 0 {
		t.Errorf("removed: %q, mentions %+v", removed.Content, removed.AllowedMentions)
	}
}
