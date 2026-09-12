package ticketbot

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
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
