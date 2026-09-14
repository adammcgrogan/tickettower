package ticketbot

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

const (
	formModalPrefix = "/ticket-form/" // + {source}/{typeID}/{version}

	// Where a form was opened from: panel dropdowns are reset on submit.
	formFromButton  = "b"
	formFromSelect  = "s"
	formFromCommand = "c" // /ticket open
)

// formSubmission is what a member typed into a ticket type's form.
type formSubmission struct {
	version string
	values  []string
}

type formAnswer struct {
	question, answer string
}

// formFor returns the form to show before opening a ticket of this type, or
// nil if it has no questions. Members who can't open one (blocked, missing a
// role, at their limit) get that error now, rather than after filling in the
// form.
func (b *Bot) formFor(ctx context.Context, guildID *snowflake.ID, m *discord.ResolvedMember, typeID int64, source string) (*discord.ModalCreate, error) {
	if guildID == nil || m == nil {
		return nil, nil
	}
	tt, err := b.store.GetTicketType(ctx, *guildID, typeID)
	if err != nil || len(tt.Questions) == 0 {
		return nil, nil // openTicket reports a missing type
	}
	st, err := b.loadOpener(ctx, *guildID, m.User.ID, m.RoleIDs, tt)
	if err != nil {
		return nil, err
	}
	if err := accessErr(tt, st, time.Now()); err != nil {
		return nil, err
	}
	modal := formModal(fmt.Sprintf("%s%s/%d/%s", formModalPrefix, source, tt.ID, formVersion(tt.Questions)), tt)
	return &modal, nil
}

func formModal(customID string, tt store.TicketType) discord.ModalCreate {
	m := discord.NewModalCreate(customID, truncate(tt.Name, 45))
	for i, q := range tt.Questions {
		style := discord.TextInputStyleShort
		if q.Style == store.QuestionParagraph {
			style = discord.TextInputStyleParagraph
		}
		input := discord.NewTextInput(fmt.Sprintf("q%d", i), style).
			WithRequired(q.Required).
			WithMaxLength(q.MaxAnswer())
		if q.Placeholder != "" {
			input = input.WithPlaceholder(q.Placeholder)
		}
		m = m.AddLabel(q.Label, input)
	}
	return m
}

// formVersion fingerprints a form's questions, so answers submitted after an
// admin edited them can't end up next to the wrong question.
func formVersion(qs []store.Question) string {
	h := fnv.New32a()
	for _, q := range qs {
		fmt.Fprintf(h, "%s\x00%s\x00%t\x00", q.Label, q.Style, q.Required)
	}
	return strconv.FormatUint(uint64(h.Sum32()), 36)
}

// formAnswers pairs a submission with the ticket type's questions.
func formAnswers(tt store.TicketType, form *formSubmission) ([]formAnswer, error) {
	if form == nil || len(tt.Questions) == 0 {
		return nil, nil
	}
	if form.version != formVersion(tt.Questions) {
		return nil, userErr("The questions for this ticket changed while you were answering. Please open it again.")
	}
	out := make([]formAnswer, len(tt.Questions))
	for i, q := range tt.Questions {
		out[i].question = q.Label
		if i < len(form.values) {
			out[i].answer = form.values[i]
		}
	}
	return out, nil
}

// truncate shortens s to at most n characters, marking the cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return string(r[:n-1]) + "…"
}
