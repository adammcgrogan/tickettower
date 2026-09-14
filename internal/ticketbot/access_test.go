package ticketbot

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestAccessErr(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	tt := store.TicketType{Name: "Partnership", MaxOpenPerUser: 1, RequiredRoleIDs: []snowflake.ID{10}, BlockedRoleIDs: []snowflake.ID{99}, CooldownMinutes: 30}
	recent := now.Add(-10 * time.Minute)
	old := now.Add(-2 * time.Hour)

	cases := []struct {
		name string
		st   openerState
		want string // substring of the message, or "" for allowed
	}{
		{"allowed", openerState{roles: []snowflake.ID{10}}, ""},
		{"blocked member", openerState{roles: []snowflake.ID{10}, block: &store.Block{Reason: "spam"}}, "Reason:** spam"},
		{"blocked without reason", openerState{roles: []snowflake.ID{10}, block: &store.Block{}}, "can't open tickets in this server."},
		{"blocked role wins over required", openerState{roles: []snowflake.ID{10, 99}}, "can't open Partnership tickets"},
		{"missing role", openerState{roles: []snowflake.ID{11}}, "need the <@&10> role"},
		{"at limit", openerState{roles: []snowflake.ID{10}, open: []snowflake.ID{5}}, "<#5>"},
		{"in cooldown", openerState{roles: []snowflake.ID{10}, lastClosed: &recent}, "<t:" + itoa(recent.Add(30*time.Minute).Unix()) + ":R>"},
		{"cooldown over", openerState{roles: []snowflake.ID{10}, lastClosed: &old}, ""},
	}
	for _, c := range cases {
		err := accessErr(tt, c.st, now)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: err = %v, want it to contain %q", c.name, err, c.want)
		}
	}

	// No restrictions: any member can open.
	plain := store.TicketType{Name: "Support", MaxOpenPerUser: 2}
	if err := accessErr(plain, openerState{}, now); err != nil {
		t.Errorf("unrestricted type: %v", err)
	}
}

func TestOnBehalfErr(t *testing.T) {
	now := time.Now()
	tt := store.TicketType{Name: "Billing", MaxOpenPerUser: 1, RequiredRoleIDs: []snowflake.ID{10}, BlockedRoleIDs: []snowflake.ID{99}, CooldownMinutes: 60}

	// The rules for members choosing for themselves don't hold staff back.
	st := openerState{roles: []snowflake.ID{99}, block: &store.Block{Reason: "spam"}, lastClosed: &now}
	if err := onBehalfErr(tt, st, 3); err != nil {
		t.Errorf("blocked member in a cooldown: %v", err)
	}
	// The member's open ticket limit does.
	err := onBehalfErr(tt, openerState{open: []snowflake.ID{5}}, 3)
	if err == nil || err.Error() != "<@3> already has an open Billing ticket: <#5>" {
		t.Errorf("at the limit: %v", err)
	}
}

func TestRoleList(t *testing.T) {
	if got := roleList([]snowflake.ID{1}); got != "the <@&1> role" {
		t.Errorf("one role = %q", got)
	}
	if got := roleList([]snowflake.ID{1, 2, 3}); got != "the <@&1>, <@&2> or <@&3> role" {
		t.Errorf("three roles = %q", got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
