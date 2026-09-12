package api

import (
	"strings"
	"testing"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestValidateSavedReply(t *testing.T) {
	in := savedReplyInput{Name: "  Refund policy ", Content: "\n Hi {user}! \n"}
	if err := validateSavedReply(&in); err != nil {
		t.Fatal(err)
	}
	if in.Name != "Refund policy" || in.Content != "Hi {user}!" {
		t.Errorf("normalised = %+v", in)
	}

	// The limits count characters, not bytes.
	ok := savedReplyInput{
		Name:    strings.Repeat("é", store.MaxSavedReplyName),
		Content: strings.Repeat("é", store.MaxSavedReplyContent),
	}
	if err := validateSavedReply(&ok); err != nil {
		t.Errorf("at the limits: %v", err)
	}

	bad := map[string]struct {
		in    savedReplyInput
		field string
	}{
		"no name":      {savedReplyInput{Name: "  ", Content: "Hi"}, "name"},
		"long name":    {savedReplyInput{Name: strings.Repeat("a", store.MaxSavedReplyName+1), Content: "Hi"}, "name"},
		"no message":   {savedReplyInput{Name: "Hi", Content: " \n "}, "content"},
		"long message": {savedReplyInput{Name: "Hi", Content: strings.Repeat("a", store.MaxSavedReplyContent+1)}, "content"},
	}
	for name, tc := range bad {
		err := validateSavedReply(&tc.in)
		ve, ok := err.(*validationError)
		if !ok || ve.Field != tc.field {
			t.Errorf("%s: err = %v, want a %s validation error", name, err, tc.field)
		}
	}
}
