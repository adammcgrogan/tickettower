package ticketbot

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/adammcgrogan/tickettower/internal/store"
)

var testQuestions = []store.Question{
	{Label: "Order number", Style: store.QuestionShort, Required: true},
	{Label: "What happened?", Style: store.QuestionParagraph},
}

func TestFormAnswers(t *testing.T) {
	tt := store.TicketType{Questions: testQuestions}

	got, err := formAnswers(tt, &formSubmission{version: formVersion(testQuestions), values: []string{"#1234", "", "", "", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (formAnswer{"Order number", "#1234"}) || got[1] != (formAnswer{"What happened?", ""}) {
		t.Errorf("answers = %+v", got)
	}

	// Answers to an older version of the form are rejected.
	edited := append([]store.Question{}, testQuestions...)
	edited[0].Label = "Invoice number"
	_, err = formAnswers(store.TicketType{Questions: edited}, &formSubmission{version: formVersion(testQuestions)})
	var ue *userError
	if !errors.As(err, &ue) {
		t.Errorf("stale form: err = %v, want a user error", err)
	}

	// Types without questions, or opens without a form, have no answers.
	if got, err := formAnswers(store.TicketType{}, &formSubmission{}); got != nil || err != nil {
		t.Errorf("no questions = %v, %v", got, err)
	}
	if got, err := formAnswers(tt, nil); got != nil || err != nil {
		t.Errorf("no form = %v, %v", got, err)
	}
}

func TestWelcomeMessageFitsEmbedLimit(t *testing.T) {
	tt := store.TicketType{Name: strings.Repeat("n", 80), WelcomeMessage: strings.Repeat("w", 2000)}
	var answers []formAnswer
	for range store.MaxQuestions {
		answers = append(answers, formAnswer{strings.Repeat("q", store.MaxQuestionLabel), strings.Repeat("a", store.MaxParagraphAnswer)})
	}
	answers[1].answer = "  "

	msg := welcomeMessage(store.Ticket{Number: 9999}, tt, answers)
	e := msg.Embeds[0]
	total := utf8.RuneCountInString(e.Title) + utf8.RuneCountInString(e.Description) + utf8.RuneCountInString(e.Footer.Text)
	for _, f := range e.Fields {
		total += utf8.RuneCountInString(f.Name) + utf8.RuneCountInString(f.Value)
	}
	if total > embedLimit {
		t.Errorf("embed has %d characters, over Discord's %d", total, embedLimit)
	}
	if len(e.Fields) != store.MaxQuestions || e.Fields[1].Value != "*No answer*" {
		t.Errorf("fields = %+v", e.Fields)
	}
	if !strings.HasSuffix(e.Description, "…") {
		t.Error("welcome text should be trimmed to make room for the answers")
	}

	// Without answers the welcome message is left alone.
	if msg := welcomeMessage(store.Ticket{}, tt, nil); msg.Embeds[0].Description != tt.WelcomeMessage {
		t.Error("welcome message changed without answers")
	}
}
