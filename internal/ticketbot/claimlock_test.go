package ticketbot

import (
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestLockedRolesAndNote(t *testing.T) {
	tt := &store.TicketType{SupportRoleIDs: []snowflake.ID{1, 2, 3}, ClaimLockExemptRoleIDs: []snowflake.ID{2}}
	channel := store.Ticket{Mode: store.ModeChannel}
	thread := store.Ticket{Mode: store.ModeThread}

	if got := lockedRoles(tt); got != nil {
		t.Errorf("lock off: locked roles = %v", got)
	}
	tt.ClaimLock = store.ClaimLockReadOnly
	if got := lockedRoles(tt); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Errorf("read only: locked roles = %v, want [1 3]", got)
	}
	if note := claimLockNote(channel, tt, 9); !strings.Contains(note, "read along") || !strings.Contains(note, "<@9>") {
		t.Errorf("read only note = %q", note)
	}
	if note := claimLockNote(thread, tt, 9); note != "" {
		t.Errorf("threads can't lock, got %q", note)
	}
	tt.ClaimLock = store.ClaimLockHidden
	if note := claimLockNote(channel, tt, 9); !strings.Contains(note, "only visible") {
		t.Errorf("hidden note = %q", note)
	}
	if lockedRoles(nil) != nil {
		t.Error("deleted type locks nothing")
	}
}

func TestReminderMessage(t *testing.T) {
	claimer := snowflake.ID(7)
	w := store.WaitingTicket{OpenerID: 42, SupportRoleIDs: []snowflake.ID{10, 11}, Ping: store.PingClaimer, Number: 3, TypeName: "Support"}

	// Unclaimed: the support roles are pinged and a Claim button offered.
	msg := reminderMessage(w, 2*time.Hour)
	if msg.Content != "<@&10> <@&11>" || len(msg.AllowedMentions.Roles) != 2 || len(msg.Components) != 1 {
		t.Errorf("unclaimed reminder = %+v", msg)
	}
	if !strings.Contains(msg.Embeds[0].Description, "2h 0m") {
		t.Errorf("description = %q", msg.Embeds[0].Description)
	}
	// Claimed: only the claimer, no Claim button.
	w.ClaimedBy = &claimer
	w.RemindersSent = 1
	msg = reminderMessage(w, time.Hour)
	if msg.Content != "<@7>" || len(msg.AllowedMentions.Users) != 1 || len(msg.Components) != 0 {
		t.Errorf("claimed reminder = %+v", msg)
	}
	if !strings.Contains(msg.Embeds[0].Description, "reminder 2") {
		t.Errorf("repeat count missing: %q", msg.Embeds[0].Description)
	}
	// Roles always, or nobody.
	w.Ping = store.PingRoles
	if msg = reminderMessage(w, time.Hour); msg.Content != "<@&10> <@&11>" {
		t.Errorf("roles ping = %q", msg.Content)
	}
	w.Ping = store.PingNobody
	if msg = reminderMessage(w, time.Hour); msg.Content != "" || len(msg.AllowedMentions.Roles) != 0 {
		t.Errorf("no ping = %+v", msg)
	}
}
