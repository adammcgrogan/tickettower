package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

const (
	checkGuild   = snowflake.ID(1)
	checkBot     = snowflake.ID(2)
	checkBotRole = snowflake.ID(3)
)

// testChannel builds a channel the way disgo decodes one from Discord. kind
// is 0 for text, 4 for a category. denyEveryone is a permission bitfield
// denied to @everyone there.
func testChannel(t *testing.T, id snowflake.ID, kind int, name string, denyEveryone discord.Permissions) discord.GuildChannel {
	t.Helper()
	data := fmt.Sprintf(`{"id":"%d","type":%d,"guild_id":"%d","name":%q,"position":0,
		"permission_overwrites":[{"id":"%d","type":0,"allow":"0","deny":"%d"}]}`, id, kind, checkGuild, name, checkGuild, denyEveryone)
	var u discord.UnmarshalChannel
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		t.Fatal(err)
	}
	return u.Channel.(discord.GuildChannel)
}

// testChannelIn builds a text channel inside a category.
func testChannelIn(t *testing.T, id snowflake.ID, name string, category snowflake.ID) discord.GuildChannel {
	t.Helper()
	data := fmt.Sprintf(`{"id":"%d","type":0,"guild_id":"%d","name":%q,"position":0,"parent_id":"%d","permission_overwrites":[]}`,
		id, checkGuild, name, category)
	var u discord.UnmarshalChannel
	if err := json.Unmarshal([]byte(data), &u); err != nil {
		t.Fatal(err)
	}
	return u.Channel.(discord.GuildChannel)
}

// fillCategory adds n channels to a category.
func fillCategory(t *testing.T, s *setup, category snowflake.ID, n int) {
	t.Helper()
	for i := range n {
		s.channels = append(s.channels, testChannelIn(t, snowflake.ID(1000+i), fmt.Sprintf("ticket-%d", i), category))
	}
}

func healthySetup(t *testing.T) setup {
	category, support, logs := snowflake.ID(10), snowflake.ID(11), snowflake.ID(12)
	panelChannel, message := snowflake.ID(13), snowflake.ID(14)
	return setup{
		appName:  "Ticket Tower",
		guildID:  checkGuild,
		botID:    checkBot,
		botRoles: []snowflake.ID{checkBotRole},
		roles: []discord.Role{
			{ID: checkGuild, Permissions: discord.PermissionViewChannel},
			{ID: checkBotRole, Permissions: channelModePerms | threadModePerms | logChannelPerms},
		},
		channels: []discord.GuildChannel{
			testChannel(t, category, 4, "Tickets", 0),
			testChannel(t, support, 0, "support", 0),
			testChannel(t, logs, 0, "ticket-log", 0),
			testChannel(t, panelChannel, 0, "help", 0),
		},
		types: []store.TicketType{
			{ID: 1, Name: "General", Mode: store.ModeChannel, ParentID: &category},
			{ID: 2, Name: "Billing", Mode: store.ModeThread, ParentID: &support},
		},
		panels: []store.Panel{
			{ID: 7, Title: "Need a hand?", ChannelID: &panelChannel, MessageID: &message, TicketTypeIDs: []int64{1, 2}},
		},
		logChannel: &logs,
	}
}

func TestSetupProblemsHealthy(t *testing.T) {
	if got := setupProblems(healthySetup(t)); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

func TestSetupProblems(t *testing.T) {
	gone := snowflake.ID(99)
	tests := []struct {
		name   string
		change func(*setup)
		want   problem // Detail is matched as a substring
	}{
		{
			name: "bot can't create channels in the category",
			change: func(s *setup) {
				s.channels[0] = testChannel(t, 10, 4, "Tickets", discord.PermissionManageChannels|discord.PermissionManageRoles)
			},
			want: problem{Kind: "ticket_type", ID: 1, Title: "General tickets can't open",
				Detail: "missing Manage Channels and Manage Roles in the Tickets category"},
		},
		{
			name: "bot can't make private threads",
			change: func(s *setup) {
				s.roles[1].Permissions = s.roles[1].Permissions.Remove(discord.PermissionCreatePrivateThreads)
			},
			want: problem{Kind: "ticket_type", ID: 2, Detail: "missing Create Private Threads in #support"},
		},
		{
			name:   "category deleted",
			change: func(s *setup) { s.types[0].ParentID = &gone },
			want:   problem{Kind: "ticket_type", ID: 1, Detail: "category its channels open in was deleted"},
		},
		{
			name:   "published buttons' channel deleted",
			change: func(s *setup) { s.panels[0].ChannelID = &gone },
			want:   problem{Kind: "panel", ID: 7, Detail: "Publish them in another channel"},
		},
		{
			name:   "ticket type on no published buttons",
			change: func(s *setup) { s.panels[0].TicketTypeIDs = []int64{1} },
			want:   problem{Kind: "unlisted", ID: 2, Title: "Members can't choose Billing"},
		},
		{
			name:   "category nearly full",
			change: func(s *setup) { fillCategory(t, s, 10, 45) },
			want: problem{Kind: "ticket_type", ID: 1, Title: "General tickets will stop opening soon",
				Detail: "has 45 of the 50 channels"},
		},
		{
			name:   "category full",
			change: func(s *setup) { fillCategory(t, s, 10, 50) },
			want:   problem{Kind: "ticket_type", ID: 1, Title: "General tickets can't open", Detail: "The Tickets category is full"},
		},
		{
			name: "log channel can't be posted in",
			change: func(s *setup) {
				s.channels[2] = testChannel(t, 12, 0, "ticket-log", discord.PermissionSendMessages)
			},
			want: problem{Kind: "log_channel", Detail: "missing Send Messages in #ticket-log"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := healthySetup(t)
			tt.change(&s)
			got := setupProblems(s)
			if len(got) != 1 {
				t.Fatalf("got %d problems, want 1: %+v", len(got), got)
			}
			p := got[0]
			if p.Kind != tt.want.Kind || p.ID != tt.want.ID || (tt.want.Title != "" && p.Title != tt.want.Title) ||
				!strings.Contains(p.Detail, tt.want.Detail) {
				t.Errorf("got %+v, want %+v", p, tt.want)
			}
		})
	}
}

func TestSetupProblemsIgnoreUnpublished(t *testing.T) {
	s := healthySetup(t)
	// A draft doesn't make other ticket types look forgotten.
	s.panels[0].MessageID = nil
	if got := setupProblems(s); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

func TestSetupProblemsIgnoreRoomyCategory(t *testing.T) {
	s := healthySetup(t)
	fillCategory(t, &s, 10, 40)
	if got := setupProblems(s); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}
