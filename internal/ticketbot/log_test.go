package ticketbot

import (
	"slices"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestOpenedLog(t *testing.T) {
	tk := store.Ticket{TypeName: "Billing", Number: 3, OpenerID: 1, ChannelID: 9}
	fields := func(msg discord.MessageCreate) []string {
		var out []string
		for _, f := range msg.Embeds[0].Fields {
			out = append(out, f.Name+" "+f.Value)
		}
		return out
	}
	if got := fields(openedLog(tk, nil)); !slices.Equal(got, []string{"Opened by <@1>", "Ticket <#9>"}) {
		t.Errorf("opened by the member: %v", got)
	}
	staff := snowflake.ID(2)
	if got := fields(openedLog(tk, &staff)); !slices.Equal(got, []string{"Opened by <@2>", "For <@1>", "Ticket <#9>"}) {
		t.Errorf("opened by staff: %v", got)
	}
}

func TestHumanDuration(t *testing.T) {
	tests := map[time.Duration]string{
		20 * time.Second:              "under a minute",
		45 * time.Minute:              "45m",
		3*time.Hour + 12*time.Minute:  "3h 12m",
		52*time.Hour + 30*time.Minute: "2d 4h",
	}
	for d, want := range tests {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%v) = %q, want %q", d, got, want)
		}
	}
}
