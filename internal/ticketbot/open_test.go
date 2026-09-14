package ticketbot

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestOpenable(t *testing.T) {
	parent := snowflake.ID(50)
	types := []store.TicketType{
		{ID: 1, Name: "General", Mode: store.ModeChannel, SupportRoleIDs: []snowflake.ID{7}},
		{ID: 2, Name: "Partnership", Mode: store.ModeChannel, RequiredRoleIDs: []snowflake.ID{10}},
		{ID: 3, Name: "Appeals", Mode: store.ModeChannel, BlockedRoleIDs: []snowflake.ID{99}},
		{ID: 4, Name: "Bug reports", Mode: store.ModeThread, ParentID: &parent, SupportRoleIDs: []snowflake.ID{7}},
		{ID: 5, Name: "Unfinished", Mode: store.ModeThread},
	}
	member := func(perms discord.Permissions, roles ...snowflake.ID) *discord.ResolvedMember {
		return &discord.ResolvedMember{Member: discord.Member{RoleIDs: roles}, Permissions: perms}
	}
	all := map[int64]bool{1: true, 2: true, 3: true, 4: true, 5: true}

	for _, c := range []struct {
		name       string
		m          *discord.ResolvedMember
		forSomeone bool
		visible    map[int64]bool
		want       []string
	}{
		{"member", member(0), false, all, []string{"General", "Appeals", "Bug reports"}},
		{"required role", member(0, 10), false, all, []string{"General", "Partnership", "Appeals", "Bug reports"}},
		{"blocked role", member(0, 10, 99), false, all, []string{"General", "Partnership", "Bug reports"}},
		{"only buttons they can see", member(0), false, map[int64]bool{1: true, 2: true}, []string{"General"}},
		{"no buttons they can see", member(0), false, map[int64]bool{}, []string{}},
		{"manager", member(discord.PermissionManageGuild), false, nil, []string{"General", "Appeals", "Bug reports"}},
		{"staff for someone", member(0, 7, 99), true, nil, []string{"General", "Bug reports"}},
		{"manager for someone", member(discord.PermissionManageGuild), true, nil, []string{"General", "Partnership", "Appeals", "Bug reports"}},
	} {
		got := []string{}
		for _, tt := range openable(types, c.m, c.forSomeone, c.visible) {
			got = append(got, tt.Name)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestVisibleTypes(t *testing.T) {
	public, staffOnly, msg := snowflake.ID(1), snowflake.ID(2), snowflake.ID(9)
	panels := []store.Panel{
		{ChannelID: &public, MessageID: &msg, TicketTypeIDs: []int64{1, 2}},
		{ChannelID: &staffOnly, MessageID: &msg, TicketTypeIDs: []int64{3}},
		{ChannelID: &public, MessageID: &msg, TicketTypeIDs: []int64{4}},
		{TicketTypeIDs: []int64{5}}, // never published
	}
	checks := 0
	got := visibleTypes(panels, func(id snowflake.ID) bool {
		checks++
		return id == public
	})
	if want := map[int64]bool{1: true, 2: true, 4: true}; !maps.Equal(got, want) {
		t.Errorf("visible = %v, want %v", got, want)
	}
	if checks != 2 {
		t.Errorf("checked channels %d times, want once each", checks)
	}
}

func TestTypeChoices(t *testing.T) {
	var types []store.TicketType
	for i := 1; i <= 30; i++ {
		types = append(types, store.TicketType{ID: int64(i), Name: fmt.Sprintf("Type %d", i)})
	}
	if got := typeChoices(types, ""); len(got) != 25 {
		t.Errorf("%d choices, want Discord's limit of 25", len(got))
	}
	var names, values []string
	for _, c := range typeChoices(types, " TYPE 3") {
		names = append(names, c.(discord.AutocompleteChoiceString).Name)
		values = append(values, c.(discord.AutocompleteChoiceString).Value)
	}
	if !slices.Equal(names, []string{"Type 3", "Type 30"}) || !slices.Equal(values, []string{"3", "30"}) {
		t.Errorf("matching %q = %v %v", " TYPE 3", names, values)
	}
	if got := typeChoices(types, "zzz"); got == nil || len(got) != 0 {
		t.Errorf("no match = %#v, want an empty list", got)
	}
}

func TestTypeByValue(t *testing.T) {
	types := []store.TicketType{{ID: 1, Name: "General"}, {ID: 12, Name: "Billing"}}
	for value, want := range map[string]int64{"12": 12, "billing": 12, " General ": 1} {
		if tt, ok := typeByValue(types, value); !ok || tt.ID != want {
			t.Errorf("typeByValue(%q) = %d, %v; want %d", value, tt.ID, ok, want)
		}
	}
	if _, ok := typeByValue(types, "Refunds"); ok {
		t.Error("an unknown name matched")
	}
}
