package api

import (
	"strings"
	"testing"

	"github.com/adammcgrogan/tickettower/internal/store"
)

func TestValidateQuestions(t *testing.T) {
	got, err := validateQuestions([]store.Question{{Label: "  Order number  ", Placeholder: " #1234 "}})
	if err != nil {
		t.Fatal(err)
	}
	if q := got[0]; q.Label != "Order number" || q.Placeholder != "#1234" || q.Style != store.QuestionShort {
		t.Errorf("normalised = %+v", q)
	}
	if got, err := validateQuestions(nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("nil questions = %#v, %v", got, err)
	}

	bad := map[string][]store.Question{
		"too many":      make([]store.Question, store.MaxQuestions+1),
		"empty label":   {{Label: "  "}},
		"long label":    {{Label: strings.Repeat("a", store.MaxQuestionLabel+1)}},
		"long hint":     {{Label: "Hi", Placeholder: strings.Repeat("a", store.MaxQuestionHint+1)}},
		"unknown style": {{Label: "Hi", Style: "essay"}},
	}
	for name, qs := range bad {
		_, err := validateQuestions(qs)
		ve, ok := err.(*validationError)
		if !ok || ve.Field != "questions" {
			t.Errorf("%s: err = %v, want a questions validation error", name, err)
		}
	}
}
