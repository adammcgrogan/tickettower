package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestSearchQuery(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"  ":            "",
		"refund":        "refund:*",
		"order #48213":  "order:* & 48213:*",
		"can't  login!": "can:* & t:* & login:*",
		"naïve café":    "naïve:* & café:*",
	}
	for in, want := range cases {
		if got := searchQuery(in); got != want {
			t.Errorf("searchQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListTicketsSearchesMessages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	seedGuild(t, s, testGuild)
	seedGuild(t, s, 3001)
	tt := newType(t, s, testGuild, "Support")
	other := newType(t, s, 3001, "Support")

	var channel snowflake.ID = 9000
	add := func(guildID snowflake.ID, typ TicketType, number int) Ticket {
		t.Helper()
		channel++
		tk := Ticket{GuildID: guildID, Number: number, TicketTypeID: &typ.ID, TypeName: typ.Name, Mode: ModeChannel,
			ChannelID: channel, OpenerID: 42, OpenerName: "member"}
		if err := s.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	var msgID snowflake.ID = 500
	say := func(tk Ticket, author, content string, embeds ...Embed) snowflake.ID {
		t.Helper()
		msgID++
		err := s.InsertTicketMessage(ctx, TicketMessage{ID: msgID, TicketID: tk.ID, AuthorID: 1, AuthorName: author,
			Content: content, Embeds: embeds, CreatedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		return msgID
	}

	first := add(testGuild, tt, 1)
	say(first, "alice", "Hi, my order number is 48213 and it never arrived.")
	say(first, "bob", "Looking into it now.")
	second := add(testGuild, tt, 2)
	say(second, "carol", "Where do I find the Refund Policy?")
	say(second, "Ticket Tower", "", Embed{Author: &EmbedAuthor{Name: "dave"}, Description: "Refunds take 3 to 5 days via the portal."})
	third := add(testGuild, tt, 3)
	say(third, "Ticket Tower", "", Embed{Fields: []EmbedField{{Name: "Order number", Value: "48213"}}})
	fourth := add(testGuild, tt, 4)
	deleted := say(fourth, "erin", "secret 48213 mention")
	if err := s.MarkTicketMessageDeleted(ctx, deleted); err != nil {
		t.Fatal(err)
	}
	foreign := add(3001, other, 1)
	say(foreign, "zed", "order 48213 in another server")

	list := func(search string) map[int]*MessageMatch {
		t.Helper()
		got, err := s.ListTickets(ctx, testGuild, TicketQuery{Search: search, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		out := map[int]*MessageMatch{}
		for _, tk := range got {
			out[tk.Number] = tk.Match
		}
		return out
	}

	got := list("48213")
	if len(got) != 2 || got[1] == nil || got[3] == nil {
		t.Fatalf("search by order number = %v, want tickets 1 and 3 with matches", got)
	}
	if m := got[1]; m.AuthorName != "alice" || !strings.Contains(m.Snippet, MatchStart+"48213"+MatchEnd) {
		t.Errorf("ticket 1 match = %+v", m)
	}
	if m := got[3]; !strings.Contains(m.Snippet, MatchStart+"48213"+MatchEnd) {
		t.Errorf("form answer match = %+v", m)
	}

	// Prefixes and case don't matter, and dashboard replies (embeds) count.
	got = list("refun")
	if len(got) != 1 || got[2] == nil || got[2].Snippet == "" {
		t.Fatalf("prefix search = %v", got)
	}
	// Every word has to appear.
	if got = list("refund arrived"); len(got) != 0 {
		t.Errorf("all words required, got %v", got)
	}
	// Names still match, with no message match attached.
	if got = list("member"); len(got) != 4 || got[1] != nil {
		t.Errorf("opener name search = %v", got)
	}
	// Punctuation-only input is treated as no message search.
	if got = list("#"); len(got) != 0 {
		t.Errorf("punctuation search = %v", got)
	}
}
