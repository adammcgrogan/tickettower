package ticketbot

import (
	"context"
	"fmt"
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
	claimButtonID = "/ticket-btn/claim"
	closeButtonID = "/ticket-btn/close"
	closeModalID  = "/ticket-modal/close"
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
		Description: "Manage the current ticket",
		Contexts:    guildOnly,
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        "close",
				Description: "Close this ticket",
				Options:     []discord.ApplicationCommandOption{reasonOption},
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
	modal, err := b.formFor(ctx, e.GuildID(), e.User(), typeID, formFromButton)
	cancel()
	if err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	if modal != nil {
		return e.Modal(*modal)
	}

	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	content := b.openForUser(e.Ctx, e.GuildID(), e.User(), typeID, nil)
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

	var modal *discord.ModalCreate
	var formErr error
	if parseErr == nil {
		ctx, cancel := context.WithTimeout(e.Ctx, formTimeout)
		modal, formErr = b.formFor(ctx, e.GuildID(), e.User(), typeID, formFromSelect)
		cancel()
	}
	if modal != nil && formErr == nil {
		// The dropdown is reset when the form is submitted.
		return e.Modal(*modal)
	}

	if err := e.UpdateMessage(reset); err != nil {
		return err
	}
	var content string
	switch {
	case parseErr != nil:
		content = "These buttons are out of date."
	case formErr != nil:
		content = b.describe(formErr)
	default:
		content = b.openForUser(e.Ctx, e.GuildID(), e.User(), typeID, nil)
	}
	_, err := e.CreateFollowupMessage(ephemeral(content))
	return err
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
	return reply(b.openForUser(e.Ctx, e.GuildID(), e.User(), typeID, form))
}

func (b *Bot) openForUser(ctx context.Context, guildID *snowflake.ID, user discord.User, typeID int64, form *formSubmission) string {
	if guildID == nil {
		return "Tickets can only be opened in a server."
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	channelID, err := b.openTicket(ctx, *guildID, user, typeID, form)
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

func (b *Bot) handleCloseCommand(e *handler.CommandEvent) error {
	reason, _ := e.SlashCommandInteractionData().OptString("reason")
	return b.close(e.Ctx, e, e.Channel().ID(), e.Member(), reason)
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
	return e.CreateMessage(discord.NewMessageCreate().
		WithContent(discord.UserMention(e.User().ID) + " added " + discord.UserMention(target.ID) + " to this ticket.").
		WithAllowedMentions(&discord.AllowedMentions{Users: []snowflake.ID{target.ID}}))
}

func (b *Bot) handleRemoveCommand(e *handler.CommandEvent) error {
	target := e.SlashCommandInteractionData().User("user")
	ctx, cancel := timeout(e.Ctx)
	defer cancel()
	if err := b.removeFromTicket(ctx, e.Channel().ID(), e.Member(), target); err != nil {
		return e.CreateMessage(ephemeral(b.describe(err)))
	}
	return e.CreateMessage(public(discord.UserMention(e.User().ID) + " removed " + discord.UserMention(target.ID) + " from this ticket."))
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
