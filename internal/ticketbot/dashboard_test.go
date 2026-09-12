package ticketbot

import "testing"

func TestReplyMessage(t *testing.T) {
	msg := replyMessage("Ticket Tower", "Adam", "https://cdn.test/adam.png", "Acme", "https://cdn.test/acme.png", "Hi <@1>, @everyone")
	if len(msg.Embeds) != 1 {
		t.Fatalf("embeds = %d, want 1", len(msg.Embeds))
	}
	e := msg.Embeds[0]
	if e.Author == nil || e.Author.Name != "Adam" || e.Author.IconURL != "https://cdn.test/adam.png" {
		t.Errorf("author = %+v", e.Author)
	}
	if e.Description != "Hi <@1>, @everyone" {
		t.Errorf("description = %q", e.Description)
	}
	// Members see it's the server's staff replying, and from where.
	const footer = "Sent by Acme staff from the Ticket Tower Dashboard"
	if e.Footer == nil || e.Footer.Text != footer || e.Footer.IconURL != "https://cdn.test/acme.png" {
		t.Errorf("footer = %+v", e.Footer)
	}
	// A reply never pings anyone.
	if am := msg.AllowedMentions; am == nil || len(am.Parse) != 0 || len(am.Users) != 0 || len(am.Roles) != 0 {
		t.Errorf("allowed mentions = %+v, want none", msg.AllowedMentions)
	}

	// The transcript keeps who wrote it.
	got := toEmbeds(msg.Embeds)
	if got[0].Author == nil || got[0].Author.Name != "Adam" || got[0].Author.IconURL != "https://cdn.test/adam.png" {
		t.Errorf("captured author = %+v", got[0].Author)
	}

	// A server without an icon still gets the footer.
	if f := replyMessage("Ticket Tower", "Adam", "", "Acme", "", "Hi").Embeds[0].Footer; f == nil || f.Text != footer || f.IconURL != "" {
		t.Errorf("footer without icon = %+v", f)
	}
}
