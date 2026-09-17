package store

import (
	"context"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestAdminOverviewAndGuilds(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	seedGuild(t, s, 3001) // free, active, one ticket
	seedGuild(t, s, 3002) // premium, active, no tickets
	seedGuild(t, s, 3003) // free, left
	if err := s.MarkGuildLeft(ctx, 3003); err != nil {
		t.Fatal(err)
	}
	if err := s.SetGuildTier(ctx, 3002, "premium"); err != nil {
		t.Fatal(err)
	}

	tt := newType(t, s, 3001, "Support")
	tk := Ticket{GuildID: 3001, Number: 1, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: ModeChannel,
		ChannelID: 9001, OpenerID: 42, OpenerName: "adam"}
	if err := s.CreateTicket(ctx, &tk); err != nil {
		t.Fatal(err)
	}

	o, err := s.AdminOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if o.TotalGuilds != 3 || o.ActiveGuilds != 2 || o.PremiumGuilds != 1 || o.FreeGuilds != 1 {
		t.Errorf("overview = %+v", o)
	}
	if o.TicketsOpenedLast30 != 1 || o.UsingGuilds30 != 1 {
		t.Errorf("overview tickets = %+v", o)
	}

	guilds, err := s.AdminGuilds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(guilds) != 3 {
		t.Fatalf("guilds = %d, want 3", len(guilds))
	}
	byID := map[snowflake.ID]AdminGuild{}
	for _, g := range guilds {
		byID[g.ID] = g
	}
	if g := byID[3001]; g.Tier != "free" || g.TicketsTotal != 1 || g.TicketsLast30 != 1 || g.LastTicketAt == nil || g.TicketTypes != 1 || g.PanelPublished {
		t.Errorf("guild 3001 = %+v", g)
	}
	if g := byID[3002]; g.Tier != "premium" || g.TicketsTotal != 0 {
		t.Errorf("guild 3002 = %+v", g)
	}
	if g := byID[3003]; g.LeftAt == nil {
		t.Errorf("guild 3003 = %+v, want left_at set", g)
	}

	// Publishing a panel shows up as setup progress.
	panel := Panel{GuildID: 3001, Title: "Help", Style: PanelButtons, TicketTypeIDs: []int64{tt.ID}}
	if err := s.CreatePanel(ctx, &panel); err != nil {
		t.Fatal(err)
	}
	channel, message := snowflake.ID(1), snowflake.ID(2)
	if err := s.SetPanelMessage(ctx, 3001, panel.ID, &channel, &message); err != nil {
		t.Fatal(err)
	}
	guilds, _ = s.AdminGuilds(ctx)
	for _, g := range guilds {
		if g.ID == 3001 && !g.PanelPublished {
			t.Errorf("guild 3001 = %+v, want panel_published", g)
		}
	}

	joins, err := s.JoinsByDay(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(joins) != 7 || joins[6].Joined != 3 || joins[6].Left != 1 || joins[0].Joined != 0 {
		t.Errorf("joins by day = %+v, want 7 days ending with 3 joined, 1 left", joins)
	}

	// Downgrading clears the entitlements row rather than leaving a stale one.
	if err := s.SetGuildTier(ctx, 3002, "free"); err != nil {
		t.Fatal(err)
	}
	tier, err := s.GuildTier(ctx, 3002)
	if err != nil {
		t.Fatal(err)
	}
	if tier != "free" {
		t.Errorf("tier after downgrade = %q, want free", tier)
	}
}
