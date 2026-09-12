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
