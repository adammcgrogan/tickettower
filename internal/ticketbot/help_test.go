package ticketbot

import (
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
)

func TestHelpMessage(t *testing.T) {
	msg := helpMessage("Ticket Tower", "https://tickettower.net", "https://discord.gg/abc")
	if msg.Flags&discord.MessageFlagEphemeral == 0 {
		t.Error("help should only be visible to whoever asked")
	}
	e := msg.Embeds[0]
	if len(e.Fields) != 2 || !strings.Contains(e.Fields[1].Value, "`/ticket open`") {
		t.Errorf("fields = %+v", e.Fields)
	}
	if n := len(e.Fields[1].Value); n > 1024 {
		t.Errorf("commands field is %d characters, over Discord's 1024", n)
	}
	row := msg.Components[0].(discord.ActionRowComponent)
	var urls []string
	for _, c := range row.Components {
		urls = append(urls, c.(discord.ButtonComponent).URL)
	}
	if strings.Join(urls, " ") != "https://tickettower.net https://tickettower.net/help https://discord.gg/abc" {
		t.Errorf("buttons = %v", urls)
	}

	// Local development has an http dashboard: no link buttons Discord would reject.
	if dev := helpMessage("Ticket Tower", "http://localhost:5173", ""); len(dev.Components) != 0 {
		t.Errorf("dev buttons = %+v", dev.Components)
	}
}

func TestHelpIsRegistered(t *testing.T) {
	for _, c := range commands {
		if c.CommandName() == "help" {
			return
		}
	}
	t.Error("/help isn't in the registered commands")
}
