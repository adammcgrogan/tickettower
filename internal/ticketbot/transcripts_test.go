package ticketbot

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestCapturedMessageTypes(t *testing.T) {
	keep := []discord.MessageType{discord.MessageTypeDefault, discord.MessageTypeReply,
		discord.MessageTypeSlashCommand, discord.MessageTypeContextMenuCommand}
	for _, mt := range keep {
		if !captured(mt) {
			t.Errorf("message type %d should be kept in transcripts", mt)
		}
	}
	skip := []discord.MessageType{discord.MessageTypeUserJoin, discord.MessageTypeChannelPinnedMessage,
		discord.MessageTypeThreadCreated, discord.MessageTypeThreadStarterMessage}
	for _, mt := range skip {
		if captured(mt) {
			t.Errorf("system message type %d should be skipped", mt)
		}
	}
}

func TestAuthorNameUsesServerNickname(t *testing.T) {
	global := "Adam"
	user := discord.User{Username: "adam", GlobalName: &global}
	nick := "Adam (Support)"
	blank := "  "
	tests := []struct {
		name   string
		member *discord.Member
		want   string
	}{
		{"nickname", &discord.Member{Nick: &nick}, "Adam (Support)"},
		{"no nickname", &discord.Member{}, "Adam"},
		{"blank nickname", &discord.Member{Nick: &blank}, "Adam"},
		{"no member (a DM or webhook)", nil, "Adam"},
	}
	for _, tt := range tests {
		if got := authorName(discord.Message{Author: user, Member: tt.member}); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestTicketCacheByGuild(t *testing.T) {
	c := newTicketCache()
	c.put(store.TicketRef{ID: 1, GuildID: 10, ChannelID: 100})
	c.put(store.TicketRef{ID: 2, GuildID: 10, ChannelID: 101})
	c.put(store.TicketRef{ID: 3, GuildID: 20, ChannelID: 200})
	if got := c.channelIDs(10); len(got) != 2 {
		t.Errorf("guild 10 channels = %v", got)
	}
	if got := c.channelIDs(0); len(got) != 3 {
		t.Errorf("all channels = %v", got)
	}
	c.removeGuild(10)
	if got := c.channelIDs(0); len(got) != 1 || got[0] != 200 {
		t.Errorf("after removing guild 10: %v", got)
	}
}

func TestAuthorIsStaff(t *testing.T) {
	support := []snowflake.ID{10, 11}
	cases := []struct {
		name    string
		roles   []snowflake.ID
		manages bool
		want    bool
	}{
		{"support role", []snowflake.ID{5, 11}, false, true},
		{"server manager without the role", []snowflake.ID{5}, true, true},
		{"someone added to the ticket", []snowflake.ID{5}, false, false},
		{"no roles", nil, false, false},
	}
	for _, c := range cases {
		if got := authorIsStaff(c.roles, support, c.manages); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
