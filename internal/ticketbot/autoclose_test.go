package ticketbot

import "testing"

func TestAutoCloseLabel(t *testing.T) {
	tests := map[int]string{12: "12 hours", 24: "1 day", 48: "2 days", 72: "3 days", 168: "1 week", 1: "1 hour"}
	for hours, want := range tests {
		if got := autoCloseLabel(hours); got != want {
			t.Errorf("autoCloseLabel(%d) = %q, want %q", hours, got, want)
		}
	}
}
