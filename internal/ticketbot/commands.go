package ticketbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/store"
)

const (
	claimButtonID        = "/ticket-btn/claim"
	closeButtonID        = "/ticket-btn/close"
	closeConfirmButtonID = "/ticket-btn/close-confirm"
	closeModalID         = "/ticket-modal/close"
	reopenButtonPrefix   = "/ticket-btn/reopen/" // + {ticketID}

	// Shown before a ticket type's form when it has suggested answers.
	answerOpenButtonPrefix  = "/ticket-btn/answer-open/"  // + {typeID}
	answerCloseButtonPrefix = "/ticket-btn/answer-close/" // + {typeID}
)

var guildOnly = []discord.InteractionContextType{discord.InteractionContextTypeGuild}

var reasonOption = discord.ApplicationCommandOptionString{
	Name:        "reason",
	Description: "Why the ticket is being closed",
}

var commands = []discord.ApplicationCommandCreate{
	discord.SlashCommandCreate{
		Name:        "ping",
		Description: "Check that the bot is online",
	},
	discord.SlashCommandCreate{
		Name:        "ticket",
		Description: "Open and manage tickets",
		Contexts:    guildOnly,
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        "open",
				Description: "Open a ticket, or open one for a member if you're staff",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:         "type",
						Description:  "The kind of ticket to open",
						Required:     true,
						Autocomplete: true,
					},
					discord.ApplicationCommandOptionUser{Name: "for", Description: "Staff only: open it for this member"},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "close",
				Description: "Close this ticket",
				Options:     []discord.ApplicationCommandOption{reasonOption},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "closerequest",
				Description: "Ask the member to confirm this ticket can be closed",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:        "reason",
						Description: "Why you think it's resolved",
						MaxLength:   ptr(500),
					},
					discord.ApplicationCommandOptionInt{
						Name:        "close_after",
						Description: "Close anyway if they don't answer (default: 1 day)",
						Choices:     closeAfterChoices,
					},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "claim",
				Description: "Claim this ticket, or unclaim it if it's yours",
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "add",
				Description: "Give someone access to this ticket",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionUser{Name: "user", Description: "Who to add", Required: true},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "remove",
				Description: "Remove someone's access to this ticket",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionUser{Name: "user", Description: "Who to remove", Required: true},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "move",
				Description: "Move this ticket to another ticket type",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:         "type",
						Description:  "The ticket type to move it to",
						Required:     true,
						Autocomplete: true,
					},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "rename",
				Description: "Rename this ticket",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{Name: "name", Description: "The new name", Required: true},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "note",
				Description: "Leave a private note on this ticket that only staff can see",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:        "text",
						Description: "The note",
						Required:    true,
						MaxLength:   ptr(store.MaxNoteLength),
					},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "hold",
				Description: "Put this ticket on hold while you wait on something else",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:        "reason",
						Description: "What you're waiting on",
						MaxLength:   ptr(MaxHoldReason),
					},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "resume",
				Description: "Take this ticket off hold",
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "block",
				Description: "Stop someone opening tickets in this server",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionUser{Name: "user", Description: "Who to block", Required: true},
					discord.ApplicationCommandOptionString{
						Name:        "reason",
						Description: "Shown to them if they try to open a ticket",
						MaxLength:   ptr(store.MaxBlockReason),
					},
					discord.ApplicationCommandOptionString{
						Name:        "duration",
						Description: "How long the block lasts. Leave out to block until unblocked.",
						Choices:     blockDurationChoices,
					},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "unblock",
				Description: "Let someone open tickets again",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionUser{Name: "user", Description: "Who to unblock", Required: true},
				},
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "close",
		Description: "Close this ticket",
		Contexts:    guildOnly,
		Options:     []discord.ApplicationCommandOption{reasonOption},
	},
	discord.SlashCommandCreate{
		Name:        "reply",
		Description: "Send one of your saved replies in this ticket",
		Contexts:    guildOnly,
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:         "name",
				Description:  "The saved reply to send",
				Required:     true,
				Autocomplete: true,
			},
		},
	},
}

// responder is satisfied by every interaction event that can reply.
type responder interface {
	CreateMessage(discord.MessageCreate, ...rest.RequestOpt) error
}

func ephemeral(content string) discord.MessageCreate {
	return discord.NewMessageCreate().WithContent(content).WithEphemeral(true)
}

// public posts a visible message without pinging anyone.
func public(content string) discord.MessageCreate {
	return discord.NewMessageCreate().WithContent(content).WithAllowedMentions(&discord.AllowedMentions{})
}

func timeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 20*time.Second)
}

func (b *Bot) handlePing(e *handler.CommandEvent) error {
	return e.CreateMessage(ephemeral("Pong! I'm online and ready to handle tickets."))
}

// --- Opening tickets ---

// formTimeout keeps the form lookup inside Discord's 3 second window for
// showing a modal.
const formTimeout = 2 * time.Second

func (b *Bot) handleOpenButton(e *handler.ComponentEvent) error {
	typeID, err := strconv.ParseInt(e.Vars["typeID"], 10, 64)
	if err != nil {
		return e.CreateMessage(ephemeral("This button is out of date."))
	}
	ctx, cancel := context.WithTimeout(e.Ctx, formTimeout)
	answers, err := b.answersFor(ctx, e.GuildID(), e.Member(), typeID)
	cancel()
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	if answers != nil {
		return e.CreateMessage(*answers)
	}

	modal, content, err := b.openFormOrContent(e, typeID)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	if modal != nil {
		return e.Modal(*modal)
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	_, err = e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent(content))
	return err
}

func (b *Bot) handleOpenSelect(e *handler.ComponentEvent) error {
	values := e.StringSelectMenuInteractionData().Values
	// Re-send the panel's components so the dropdown resets for the user.
	reset := discord.NewMessageUpdate().WithComponents(e.Message.Components...)
	if len(values) == 0 {
		return e.UpdateMessage(reset)
	}
	typeID, parseErr := strconv.ParseInt(values[0], 10, 64)

	var answers *discord.MessageCreate
	var answerErr error
	if parseErr == nil {
		ctx, cancel := context.WithTimeout(e.Ctx, formTimeout)
		answers, answerErr = b.answersFor(ctx, e.GuildID(), e.Member(), typeID)
		cancel()
	}

	var modal *discord.ModalCreate
	var formErr error
	if parseErr == nil && answers == nil && answerErr == nil {
		ctx, cancel := context.WithTimeout(e.Ctx, formTimeout)
		modal, formErr = b.formFor(ctx, e.GuildID(), e.Member(), typeID, formFromSelect)
		cancel()
	}
	if modal != nil && formErr == nil {
		// The dropdown is reset when the form is submitted.
		return e.Modal(*modal)
	}

	// Every other outcome resets the dropdown now and replies in a
	// follow-up, since answers (and the form after them, if any) are shown
	// in a fresh ephemeral message rather than the panel's.
	if err := e.UpdateMessage(reset); err != nil {
		return err
	}
	if answers != nil {
		_, err := e.CreateFollowupMessage(*answers)
		return err
	}
	var content string
	switch {
	case parseErr != nil:
		content = "These buttons are out of date."
	case answerErr != nil:
		content = b.describe(answerErr)
	case formErr != nil:
		content = b.describe(formErr)
	default:
		content = b.openForUser(e.Ctx, e.GuildID(), e.Member(), typeID, nil)
	}
	_, err := e.CreateFollowupMessage(ephemeral(content))
	return err
}

// openFormOrContent resolves a ticket button click into either a form to
// show or the message to leave for the member, without responding to the
// interaction itself.
func (b *Bot) openFormOrContent(e *handler.ComponentEvent, typeID int64) (*discord.ModalCreate, string, error) {
	ctx, cancel := context.WithTimeout(e.Ctx, formTimeout)
	modal, err := b.formFor(ctx, e.GuildID(), e.Member(), typeID, formFromButton)
	cancel()
	if err != nil {
		return nil, "", err
	}
	if modal != nil {
		return modal, "", nil
	}
	return nil, b.openForUser(e.Ctx, e.GuildID(), e.Member(), typeID, nil), nil
}

// handleAnswerOpen continues opening a ticket after a member decided its
// suggested answers didn't solve it, editing the ephemeral answers message
// in place.
func (b *Bot) handleAnswerOpen(e *handler.ComponentEvent) error {
	typeID, err := strconv.ParseInt(e.Vars["typeID"], 10, 64)
	if err != nil {
		return e.UpdateMessage(discord.NewMessageUpdate().WithContent("This button is out of date.").ClearComponents())
	}
	modal, content, err := b.openFormOrContent(e, typeID)
	if err != nil {
		return e.UpdateMessage(discord.NewMessageUpdate().WithContent(b.describe(err)).ClearComponents())
	}
	if modal != nil {
		return e.Modal(*modal)
	}
	if err := e.DeferUpdateMessage(); err != nil {
		return err
	}
	_, err = e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent(content).WithEmbeds().ClearComponents())
	return err
}

// handleAnswerClose records that a member's question was answered without
// opening a ticket.
func (b *Bot) handleAnswerClose(e *handler.ComponentEvent) error {
	typeID, err := strconv.ParseInt(e.Vars["typeID"], 10, 64)
	if err != nil {
		return e.UpdateMessage(discord.NewMessageUpdate().WithContent("This button is out of date.").ClearComponents())
	}
	if guildID := e.GuildID(); guildID != nil {
		ctx, cancel := timeout(e.Ctx)
		if err := b.store.RecordAnswerDeflection(ctx, *guildID, typeID); err != nil {
			b.log.Warn("record answer deflection", slog.Any("err", err))
		}
		cancel()
	}
	return e.UpdateMessage(discord.NewMessageUpdate().
		WithContent("Glad that helped! Open a ticket any time if you need more.").
		WithEmbeds().
		ClearComponents())
}

func (b *Bot) handleFormModal(e *handler.ModalEvent) error {
	var reply func(content string) error
	if e.Vars["source"] == formFromSelect && e.Message != nil {
		// Reset the panel's dropdown, as handleOpenSelect does.
		if err := e.UpdateMessage(discord.NewMessageUpdate().WithComponents(e.Message.Components...)); err != nil {
			return err
		}
		reply = func(content string) error {
			_, err := e.CreateFollowupMessage(ephemeral(content))
			return err
		}
	} else {
		if err := e.DeferCreateMessage(true); err != nil {
			return err
		}
		reply = func(content string) error {
			_, err := e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent(content))
			return err
		}
	}

	typeID, err := strconv.ParseInt(e.Vars["typeID"], 10, 64)
	if err != nil {
		return reply("This form is out of date.")
	}
	form := &formSubmission{version: e.Vars["version"]}
	for i := range store.MaxQuestions {
		form.values = append(form.values, strings.TrimSpace(e.Data.Text(fmt.Sprintf("q%d", i))))
	}
	return reply(b.openForUser(e.Ctx, e.GuildID(), e.Member(), typeID, form))
}

func (b *Bot) openForUser(ctx context.Context, guildID *snowflake.ID, m *discord.ResolvedMember, typeID int64, form *formSubmission) string {
	if guildID == nil || m == nil {
		return "Tickets can only be opened in a server."
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	channelID, err := b.openTicket(ctx, *guildID, m.User, m.RoleIDs, typeID, form, nil)
	if err != nil {
		return b.describe(err)
	}
	return "Your ticket is ready: " + discord.ChannelMention(channelID)
}

// --- Claiming ---

func (b *Bot) handleClaimButton(e *handler.ComponentEvent) error {
	return b.claim(e.Ctx, e, e.Channel().ID(), e.Member())
}

func (b *Bot) handleClaimCommand(e *handler.CommandEvent) error {
	return b.claim(e.Ctx, e, e.Channel().ID(), e.Member())
}

func (b *Bot) claim(ctx context.Context, r responder, channelID snowflake.ID, m *discord.ResolvedMember) error {
	ctx, cancel := timeout(ctx)
	defer cancel()
	msg, err := b.toggleClaim(ctx, channelID, m)
	if err != nil {
		return r.CreateMessage(ephemeral(b.describe(err)))
	}
	return r.CreateMessage(public(msg))
}

// --- Closing ---

func (b *Bot) handleCloseButton(e *handler.ComponentEvent) error {
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	t, tt, err := b.loadTicket(ctx, e.Channel().ID())
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	if !canClose(t, tt, e.Member()) {
		return e.CreateMessage(ephemeral("Only the person who opened this ticket or support staff can close it."))
	}
	return e.Modal(discord.NewModalCreate(closeModalID, "Close ticket").
		AddLabel("Reason (optional)", discord.NewParagraphTextInput("reason").
			WithRequired(false).
			WithMaxLength(500).
			WithPlaceholder("e.g. Resolved, refund issued")))
}

func (b *Bot) handleCloseModal(e *handler.ModalEvent) error {
	return b.close(e.Ctx, e, e.Channel().ID(), e.Member(), e.Data.Text("reason"))
}

// handleCloseCommand closes the ticket. A member closing their own ticket
// without a reason is asked to confirm first, since channel tickets are
// deleted seconds later and a mistyped command shouldn't end the conversation.
func (b *Bot) handleCloseCommand(e *handler.CommandEvent) error {
	reason, _ := e.SlashCommandInteractionData().OptString("reason")
	if strings.TrimSpace(reason) == "" {
		ctx, cancel := timeout(e.Ctx)
		defer cancel()
		t, tt, err := b.loadTicket(ctx, e.Channel().ID())
		if err != nil {
			return e.CreateMessage(ephemeral(b.describe(err)))
		}
		if m := e.Member(); m != nil && m.User.ID == t.OpenerID && !isStaff(m, tt) {
			return e.CreateMessage(discord.NewMessageCreate().
				WithContent("Close this ticket? " + closeWarning(t)).
				WithEphemeral(true).
				AddActionRow(
					discord.NewDangerButton("Close ticket", closeConfirmButtonID).WithEmoji(discord.ComponentEmoji{Name: "🔒"}),
				))
		}
	}
	return b.close(e.Ctx, e, e.Channel().ID(), e.Member(), reason)
}

// closeWarning says what closing means for this kind of ticket.
func closeWarning(t store.Ticket) string {
	if t.Mode == store.ModeThread {
		return "You can reopen it later if you need to."
	}
	return "This channel will be deleted, though the transcript is kept."
}

// handleCloseConfirm answers the confirmation shown to a member who typed
// /close on their own ticket.
func (b *Bot) handleCloseConfirm(e *handler.ComponentEvent) error {
	// The confirmation is ephemeral, so the close message goes in the
	// channel separately and the confirmation is tidied away.
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	t, msg, err := b.closeTicket(ctx, e.Channel().ID(), e.Member(), "")
	if err != nil {
		return e.UpdateMessage(discord.NewMessageUpdate().WithContent(b.describe(err)).ClearComponents())
	}
	if _, err := b.rest.CreateMessage(t.ChannelID, msg, rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to post close message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	go b.finishClose(t)
	return e.UpdateMessage(discord.NewMessageUpdate().WithContent("Ticket closed.").ClearComponents())
}

// --- Reopening ---

// handleReopenButton reopens a closed thread ticket from the close message
// in the thread, or from the opener's DM.
func (b *Bot) handleReopenButton(e *handler.ComponentEvent) error {
	id, err := strconv.ParseInt(e.Vars["ticketID"], 10, 64)
	if err != nil {
		return e.CreateMessage(ephemeral("This button is out of date."))
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	t, err := b.store.GetTicket(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		err = userErr("This ticket no longer exists.")
	}
	if err == nil {
		// In the thread, the opener or staff may reopen. From the DM there's
		// no member, and only the opener ever got that button.
		member, userID := e.Member(), e.User().ID
		t, err = b.reopenTicket(ctx, t, userID, func(tt *store.TicketType) bool {
			if member == nil {
				return userID == t.OpenerID
			}
			return canClose(t, tt, member)
		})
	}
	if err != nil {
		_, err = e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent(b.describe(err)))
		return err
	}
	if _, err := b.rest.CreateMessage(t.ChannelID, reopenedMessage(e.User().ID), rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to post reopen message", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
	_, err = e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent("Ticket reopened: " + discord.ChannelMention(t.ChannelID)))
	return err
}

func (b *Bot) close(ctx context.Context, r responder, channelID snowflake.ID, m *discord.ResolvedMember, reason string) error {
	ctx, cancel := timeout(ctx)
	defer cancel()
	t, msg, err := b.closeTicket(ctx, channelID, m, strings.TrimSpace(reason))
	if err != nil {
		return r.CreateMessage(ephemeral(b.describe(err)))
	}
	respondErr := r.CreateMessage(msg)
	go b.finishClose(t)
	return respondErr
}

// --- Managing members ---

func (b *Bot) handleAddCommand(e *handler.CommandEvent) error {
	target := e.SlashCommandInteractionData().User("user")
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	if err := b.addToTicket(ctx, e.Channel().ID(), e.Member(), target); err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.CreateMessage(addedMessage(e.User().ID, target.ID))
}

func (b *Bot) handleRemoveCommand(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	target := data.User("user")
	var targetMember *discord.ResolvedMember
	if rm, ok := data.OptMember("user"); ok {
		targetMember = &rm
	}
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	if err := b.removeFromTicket(ctx, e.Channel().ID(), e.Member(), target, targetMember); err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.CreateMessage(removedMessage(e.User().ID, target.ID))
}

func (b *Bot) handleRenameCommand(e *handler.CommandEvent) error {
	name := e.SlashCommandInteractionData().String("name")
	// Renames can be slow when Discord's rate limit applies, so defer first.
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(e.Ctx, 15*time.Second)
	defer cancel()
	content := "Ticket renamed."
	if err := b.renameTicket(ctx, e.Channel().ID(), e.Member(), name); err != nil {
		content = b.describe(err)
	}
	_, err := e.UpdateInteractionResponse(discord.NewMessageUpdate().WithContent(content))
	return err
}

// --- Blocking members ---

// blockDurations maps the /ticket block duration choice to how long the
// block lasts. Matches blockDurationOptions in the API and web.
var blockDurations = map[string]time.Duration{
	"1h":  time.Hour,
	"1d":  24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

var blockDurationChoices = []discord.ApplicationCommandOptionChoiceString{
	{Name: "1 hour", Value: "1h"},
	{Name: "1 day", Value: "1d"},
	{Name: "7 days", Value: "7d"},
	{Name: "30 days", Value: "30d"},
}

func (b *Bot) handleBlockCommand(e *handler.CommandEvent) error {
	data := e.SlashCommandInteractionData()
	target := data.User("user")
	var targetMember *discord.ResolvedMember
	if rm, ok := data.OptMember("user"); ok {
		targetMember = &rm
	}
	reason, _ := data.OptString("reason")
	duration, _ := data.OptString("duration")
	if e.GuildID() == nil {
		return e.CreateMessage(ephemeral("Members can only be blocked in a server."))
	}
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	msg, err := b.blockMember(ctx, *e.GuildID(), e.Member(), target, targetMember, reason, blockDurations[duration])
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.CreateMessage(ephemeral(msg))
}

func (b *Bot) handleUnblockCommand(e *handler.CommandEvent) error {
	target := e.SlashCommandInteractionData().User("user")
	if e.GuildID() == nil {
		return e.CreateMessage(ephemeral("Members can only be unblocked in a server."))
	}
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	msg, err := b.unblockMember(ctx, *e.GuildID(), e.Member(), target)
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.CreateMessage(ephemeral(msg))
}

func ptr[T any](v T) *T { return &v }
