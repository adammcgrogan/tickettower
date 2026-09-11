package ticketbot

import (
	"testing"
	"time"
)

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
