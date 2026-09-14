package ticketbot

import (
	"strings"
	"testing"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestNoteText(t *testing.T) {
	if got, err := noteText("  Refunded $20, don't refund again.\n"); err != nil || got != "Refunded $20, don't refund again." {
		t.Errorf("note = %q, %v", got, err)
	}
	if got, err := noteText(strings.Repeat("é", store.MaxNoteLength)); err != nil || len([]rune(got)) != store.MaxNoteLength {
		t.Errorf("longest note: %v", err)
	}
	for name, text := range map[string]string{"empty": " \n ", "too long": strings.Repeat("x", store.MaxNoteLength+1)} {
		_, err := noteText(text)
		if _, ok := UserMessage(err); !ok {
			t.Errorf("%s: err = %v, want a message for the user", name, err)
		}
	}
}
