package ticketbot

import (
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
)

// helpCommands is what /help lists, in the order people need them.
var helpCommands = []struct {
	usages []string
	what   string
}{
	{[]string{"/ticket open"}, "Open a ticket, or open one for a member if you're staff"},
	{[]string{"/ticket claim"}, "Claim this ticket, or unclaim it if it's yours"},
	{[]string{"/close"}, "Close this ticket"},
	{[]string{"/ticket closerequest"}, "Ask the member to confirm the ticket can be closed"},
	{[]string{"/reply"}, "Send one of your saved replies"},
	{[]string{"/ticket add", "/ticket remove"}, "Give or remove someone's access to this ticket"},
	{[]string{"/ticket move", "/ticket rename"}, "Move this ticket to another type, or rename it"},
	{[]string{"/ticket merge"}, "Merge this ticket into another open ticket from the same member"},
	{[]string{"/ticket note"}, "Leave a private note only staff can see"},
	{[]string{"/ticket hold", "/ticket resume"}, "Put this ticket on hold, or take it off hold"},
	{[]string{"/ticket block", "/ticket unblock"}, "Stop or allow someone opening tickets"},
	{[]string{"/ticket available"}, "Opt in or out of auto-assigned tickets"},
	{[]string{"/ping"}, "Check that the bot is online"},
}

func (b *Bot) handleHelp(e *handler.CommandEvent) error {
	return e.CreateMessage(helpMessage(b.cfg.AppName, b.cfg.PublicURL, b.cfg.SupportURL))
}

// helpMessage explains how to get started and lists the commands. Setup
// happens in the dashboard, so it leads there. Discord rejects link buttons
// to non-https URLs, so those are only added for https links.
func helpMessage(appName, publicURL, supportURL string) discord.MessageCreate {
	var lines []string
	for _, c := range helpCommands {
		lines = append(lines, "`"+strings.Join(c.usages, "` `")+"` "+c.what)
	}
	start := "Log in to the dashboard with Discord, pick your server, and follow the quick setup: create a ticket type, then post a ticket panel members can click."
	embed := discord.NewEmbed().
		WithTitle(appName+" help").
		WithColor(colorAccent).
		WithDescription("Members open private tickets from a ticket panel, and your team handles them here or in the dashboard.").
		AddField("Getting started", start, false).
		AddField("Commands", strings.Join(lines, "\n"), false)

	msg := discord.NewMessageCreate().WithEmbeds(embed).WithEphemeral(true)
	var buttons []discord.InteractiveComponent
	for _, l := range []struct{ label, url string }{
		{"Open the dashboard", publicURL},
		{"Help guides", publicURL + "/help"},
		{"Support server", supportURL},
	} {
		if strings.HasPrefix(l.url, "https://") {
			buttons = append(buttons, discord.NewLinkButton(l.label, l.url))
		}
	}
	if len(buttons) > 0 {
		msg = msg.AddActionRow(buttons...)
	}
	return msg
}
