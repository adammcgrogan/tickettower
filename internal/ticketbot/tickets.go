package ticketbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/discordx"
	"github.com/adammcgrogan/tickettower/internal/store"
)

const (
	colorAccent = 0xf2b544 // the dashboard's signal amber
	colorMuted  = 0x5d5d66

	// closeDelay gives people a moment to read the close message before a
	// ticket channel is deleted.
	closeDelay = 5 * time.Second

	defaultWelcome = "Thanks for reaching out, {user}! Tell us what you need help with and someone from the team will be with you shortly."
)

// ticketMemberPerms are granted to the opener, added users and support roles.
const ticketMemberPerms = discord.PermissionViewChannel |
	discord.PermissionSendMessages |
	discord.PermissionReadMessageHistory |
	discord.PermissionAttachFiles |
	discord.PermissionEmbedLinks

// userError is an error whose message is safe and useful to show the user.
type userError struct{ msg string }

func (e *userError) Error() string { return e.msg }

func userErr(format string, a ...any) error { return &userError{msg: fmt.Sprintf(format, a...)} }

// describe turns an error into a message for the user, logging anything
// unexpected.
func (b *Bot) describe(err error) string {
	var ue *userError
	if errors.As(err, &ue) {
		return ue.msg
	}
	if msg := discordx.Friendly(err); msg != "" {
		b.log.Warn("discord rejected request", slog.Any("err", err))
		return msg
	}
	b.log.Error("interaction failed", slog.Any("err", err))
	return "Something went wrong. Please try again in a moment."
}

func (b *Bot) lockMember(guildID, userID snowflake.ID) func() {
	v, _ := b.openLocks.LoadOrStore(guildID.String()+":"+userID.String(), &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// openTicket creates a ticket channel or thread for user and returns its ID.
// form holds the member's answers if the type has questions.
func (b *Bot) openTicket(ctx context.Context, guildID snowflake.ID, user discord.User, typeID int64, form *formSubmission) (snowflake.ID, error) {
	defer b.lockMember(guildID, user.ID)()

	tt, err := b.store.GetTicketType(ctx, guildID, typeID)
	if errors.Is(err, store.ErrNotFound) {
		return 0, userErr("This ticket type no longer exists. Ask a server admin to update the ticket buttons.")
	} else if err != nil {
		return 0, err
	}

	open, err := b.store.OpenTicketChannels(ctx, guildID, tt.ID, user.ID)
	if err != nil {
		return 0, err
	}
	if err := limitErr(tt, open); err != nil {
		return 0, err
	}
	if tt.Mode == store.ModeThread && tt.ParentID == nil {
		return 0, userErr("This ticket type isn't set up yet. Ask a server admin to choose a channel for it.")
	}
	answers, err := formAnswers(tt, form)
	if err != nil {
		return 0, err
	}

	number, err := b.store.NextTicketNumber(ctx, guildID)
	if err != nil {
		return 0, err
	}
	name := channelName(tt.NameFormat, number, user)

	var channelID snowflake.ID
	switch tt.Mode {
	case store.ModeThread:
		invitable := false
		thread, err := b.client.Rest.CreateThread(*tt.ParentID, discord.GuildPrivateThreadCreate{
			Name:                name,
			AutoArchiveDuration: discord.AutoArchiveDuration1w,
			Invitable:           &invitable,
		}, rest.WithCtx(ctx))
		if err != nil {
			return 0, err
		}
		channelID = thread.ID()
		if err := b.client.Rest.AddThreadMember(channelID, user.ID, rest.WithCtx(ctx)); err != nil {
			b.cleanupChannel(channelID)
			return 0, err
		}
	default:
		create := discord.GuildTextChannelCreate{
			Name:                 name,
			Topic:                fmt.Sprintf("%s ticket #%d opened by %s", tt.Name, number, user.Username),
			PermissionOverwrites: channelOverwrites(guildID, b.client.ApplicationID, user.ID, tt.SupportRoleIDs),
		}
		if tt.ParentID != nil {
			create.ParentID = *tt.ParentID
		}
		ch, err := b.client.Rest.CreateGuildChannel(guildID, create, rest.WithCtx(ctx))
		if err != nil {
			return 0, err
		}
		channelID = ch.ID()
	}

	ticket := store.Ticket{
		GuildID:      guildID,
		Number:       number,
		TicketTypeID: &tt.ID,
		TypeName:     tt.Name,
		Mode:         tt.Mode,
		ChannelID:    channelID,
		OpenerID:     user.ID,
		OpenerName:   user.EffectiveName(),
		// Form answers mean the member has already explained, so it's the
		// team's turn and auto-close waits for their reply.
		WaitingOnStaff: len(answers) > 0,
	}
	if err := b.store.CreateTicket(ctx, &ticket); err != nil {
		b.cleanupChannel(channelID)
		return 0, err
	}
	// Track the channel before sending anything so the welcome message is
	// captured in the transcript.
	b.tickets.put(store.TicketRef{ID: ticket.ID, ChannelID: channelID, OpenerID: user.ID})

	if _, err := b.client.Rest.CreateMessage(channelID, welcomeMessage(ticket, tt, answers), rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to send welcome message", slog.Any("err", err))
	}
	go b.logEvent(guildID, openedLog(ticket))
	b.log.Info("ticket opened",
		slog.String("guild_id", guildID.String()), slog.Int("number", number), slog.String("mode", string(tt.Mode)))
	return channelID, nil
}

// limitErr reports whether a member already has as many open tickets of a
// type as they're allowed.
func limitErr(tt store.TicketType, open []snowflake.ID) error {
	if len(open) >= tt.MaxOpenPerUser {
		return userErr("You already have an open %s ticket: %s", tt.Name, discord.ChannelMention(open[len(open)-1]))
	}
	return nil
}

func channelOverwrites(guildID, botID, openerID snowflake.ID, supportRoles []snowflake.ID) []discord.PermissionOverwrite {
	overwrites := []discord.PermissionOverwrite{
		// The @everyone role shares the guild's ID.
		discord.RolePermissionOverwrite{RoleID: guildID, Deny: discord.PermissionViewChannel},
		discord.MemberPermissionOverwrite{UserID: botID, Allow: ticketMemberPerms | discord.PermissionManageChannels},
		discord.MemberPermissionOverwrite{UserID: openerID, Allow: ticketMemberPerms},
	}
	for _, id := range supportRoles {
		overwrites = append(overwrites, discord.RolePermissionOverwrite{RoleID: id, Allow: ticketMemberPerms})
	}
	return overwrites
}

// channelName fills in a ticket type's name format, e.g. "ticket-{number}".
func channelName(format string, number int, user discord.User) string {
	name := strings.NewReplacer(
		"{number}", fmt.Sprintf("%04d", number),
		"{username}", user.Username,
		"{user}", user.Username,
	).Replace(format)
	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("ticket-%04d", number)
	}
	if r := []rune(name); len(r) > 100 {
		name = string(r[:100])
	}
	return name
}

// embedLimit is Discord's cap on the total text in a message's embeds.
const embedLimit = 6000

func welcomeMessage(t store.Ticket, tt store.TicketType, answers []formAnswer) discord.MessageCreate {
	text := tt.WelcomeMessage
	if strings.TrimSpace(text) == "" {
		text = defaultWelcome
	}
	text = strings.ReplaceAll(text, "{user}", discord.UserMention(t.OpenerID))

	// Mentioning support roles also adds them to private threads.
	mentions := []string{discord.UserMention(t.OpenerID)}
	for _, id := range tt.SupportRoleIDs {
		mentions = append(mentions, discord.RoleMention(id))
	}

	title := fmt.Sprintf("%s · #%d", tt.Name, t.Number)
	const footer = "Staff can claim this ticket. Either side can close it when you're done."
	embed := discord.NewEmbed().
		WithTitle(title).
		WithColor(colorAccent).
		WithFooterText(footer)

	budget := embedLimit - utf8.RuneCountInString(title) - utf8.RuneCountInString(footer)
	for _, a := range answers {
		value := a.answer
		if strings.TrimSpace(value) == "" {
			value = "*No answer*"
		}
		embed = embed.AddField(a.question, value, false)
		budget -= utf8.RuneCountInString(a.question) + utf8.RuneCountInString(value)
	}
	// Long answers win over a long welcome message if both can't fit.
	embed = embed.WithDescription(truncate(text, budget))

	return discord.NewMessageCreate().
		WithContent(strings.Join(mentions, " ")).
		WithEmbeds(embed).
		AddActionRow(
			discord.NewSecondaryButton("Claim", claimButtonID).WithEmoji(discord.ComponentEmoji{Name: "🙋"}),
			discord.NewDangerButton("Close", closeButtonID).WithEmoji(discord.ComponentEmoji{Name: "🔒"}),
		).
		WithAllowedMentions(&discord.AllowedMentions{
			Users: []snowflake.ID{t.OpenerID},
			Roles: tt.SupportRoleIDs,
		})
}

// cleanupChannel deletes a half-created ticket channel after a failure.
func (b *Bot) cleanupChannel(channelID snowflake.ID) {
	if err := b.client.Rest.DeleteChannel(channelID); err != nil && !discordx.IsCode(err, discordx.CodeUnknownChannel) {
		b.log.Warn("failed to clean up ticket channel", slog.Any("err", err))
	}
}

// loadTicket returns the open ticket in a channel along with its type (nil
// if the type has since been deleted).
func (b *Bot) loadTicket(ctx context.Context, channelID snowflake.ID) (store.Ticket, *store.TicketType, error) {
	t, err := b.store.GetTicketByChannel(ctx, channelID)
	if errors.Is(err, store.ErrNotFound) {
		return t, nil, userErr("This only works inside a ticket.")
	} else if err != nil {
		return t, nil, err
	}
	if t.Status != store.StatusOpen {
		return t, nil, userErr("This ticket is already closed.")
	}
	var tt *store.TicketType
	if t.TicketTypeID != nil {
		if v, err := b.store.GetTicketType(ctx, t.GuildID, *t.TicketTypeID); err == nil {
			tt = &v
		}
	}
	return t, tt, nil
}

// isStaff reports whether a member can manage tickets of a type: server
// managers always can, otherwise they need one of the type's support roles.
func isStaff(m *discord.ResolvedMember, tt *store.TicketType) bool {
	if m == nil {
		return false
	}
	if m.Permissions.Has(discord.PermissionAdministrator) || m.Permissions.Has(discord.PermissionManageGuild) {
		return true
	}
	if tt == nil {
		return false
	}
	for _, id := range m.RoleIDs {
		if slices.Contains(tt.SupportRoleIDs, id) {
			return true
		}
	}
	return false
}

func canClose(t store.Ticket, tt *store.TicketType, m *discord.ResolvedMember) bool {
	return m != nil && (m.User.ID == t.OpenerID || isStaff(m, tt))
}

// toggleClaim claims the ticket for the member, or releases it if they
// already hold it. It returns the message to post in the ticket.
func (b *Bot) toggleClaim(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember) (string, error) {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return "", err
	}
	if !isStaff(m, tt) {
		return "", userErr("Only support staff can claim tickets.")
	}
	me := m.User.ID

	if t.ClaimedBy != nil && *t.ClaimedBy == me {
		if _, err := b.store.UnclaimTicket(ctx, t.ID, me); err != nil {
			return "", err
		}
		return discord.UserMention(me) + " unclaimed this ticket.", nil
	}
	if t.ClaimedBy != nil {
		return "", userErr("This ticket is already claimed by %s.", discord.UserMention(*t.ClaimedBy))
	}
	ok, err := b.store.ClaimTicket(ctx, t.ID, me, m.User.EffectiveName())
	if err != nil {
		return "", err
	}
	if !ok {
		return "", userErr("Someone else claimed this ticket just now.")
	}
	go b.logEvent(t.GuildID, claimedLog(t, me))
	return "🙋 " + discord.UserMention(me) + " has claimed this ticket and will help you from here.", nil
}

// closeTicket marks the ticket closed and returns the closing message. The
// channel is archived or deleted afterwards by finishClose.
func (b *Bot) closeTicket(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, reason string) (store.Ticket, discord.MessageCreate, error) {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return t, discord.MessageCreate{}, err
	}
	if !canClose(t, tt, m) {
		return t, discord.MessageCreate{}, userErr("Only the person who opened this ticket or support staff can close it.")
	}

	closer := m.User.ID
	ok, err := b.store.CloseTicket(ctx, t.ID, closer, m.User.EffectiveName(), reason)
	if err != nil {
		return t, discord.MessageCreate{}, err
	}
	if !ok {
		return t, discord.MessageCreate{}, userErr("This ticket is already closed.")
	}
	t.Status, t.ClosedBy, t.CloseReason = store.StatusClosed, &closer, reason
	return t, closedMessage(t), nil
}

// closedMessage is posted in a ticket as it closes. Tickets without a closer
// were closed automatically.
func closedMessage(t store.Ticket) discord.MessageCreate {
	desc := "Closed automatically"
	if t.ClosedBy != nil {
		desc = "Closed by " + discord.UserMention(*t.ClosedBy)
	}
	if t.CloseReason != "" {
		desc += "\n**Reason:** " + t.CloseReason
	}
	if t.Mode == store.ModeChannel {
		desc += "\n\nThis channel will be deleted in a few seconds."
	}
	return discord.NewMessageCreate().
		WithEmbeds(discord.NewEmbed().WithTitle("Ticket closed").WithDescription(desc).WithColor(colorMuted)).
		WithAllowedMentions(&discord.AllowedMentions{})
}

// finishClose notifies the opener and removes the ticket from view: channels
// are deleted, threads are archived and locked.
func (b *Bot) finishClose(t store.Ticket) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The close message has been posted, so the transcript is complete.
	b.tickets.remove(t.ChannelID)
	b.logEvent(t.GuildID, b.closedLog(t))
	b.notifyOpener(ctx, t)

	var err error
	switch t.Mode {
	case store.ModeThread:
		archived, locked := true, true
		_, err = b.client.Rest.UpdateChannel(t.ChannelID, discord.GuildThreadUpdate{Archived: &archived, Locked: &locked}, rest.WithCtx(ctx))
	default:
		time.Sleep(closeDelay)
		err = b.client.Rest.DeleteChannel(t.ChannelID, rest.WithCtx(ctx))
	}
	if err != nil && !discordx.IsCode(err, discordx.CodeUnknownChannel) {
		b.log.Warn("failed to clean up closed ticket", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
}

func (b *Bot) notifyOpener(ctx context.Context, t store.Ticket) {
	dm, err := b.client.Rest.CreateDMChannel(t.OpenerID, rest.WithCtx(ctx))
	if err != nil {
		return
	}
	guildName := "the server"
	if g, ok := b.client.Caches.Guild(t.GuildID); ok {
		guildName = g.Name
	}
	desc := fmt.Sprintf("Your **%s** ticket (#%d) in **%s** has been closed.", t.TypeName, t.Number, guildName)
	if t.CloseReason != "" {
		desc += "\n**Reason:** " + t.CloseReason
	}
	desc += "\n\nHow did we do? Rate your experience below."
	embed := discord.NewEmbed().WithTitle("Ticket closed").WithDescription(desc).WithColor(colorMuted)
	msg := discord.NewMessageCreate().WithEmbeds(embed).WithComponents(b.feedbackComponents(t.ID)...)
	_, err = b.client.Rest.CreateMessage(dm.ID(), msg, rest.WithCtx(ctx))
	if err != nil && !discordx.IsCode(err, discordx.CodeCannotDMUser) {
		b.log.Warn("failed to DM ticket opener", slog.Any("err", err))
	}
}

// addToTicket gives a user access to the ticket.
func (b *Bot) addToTicket(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, target discord.User) error {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return err
	}
	if !isStaff(m, tt) {
		return userErr("Only support staff can add people to tickets.")
	}
	if t.Mode == store.ModeThread {
		return b.client.Rest.AddThreadMember(t.ChannelID, target.ID, rest.WithCtx(ctx))
	}
	allow := ticketMemberPerms
	return b.client.Rest.UpdatePermissionOverwrite(t.ChannelID, target.ID,
		discord.MemberPermissionOverwriteUpdate{Allow: &allow}, rest.WithCtx(ctx))
}

// removeFromTicket revokes a user's access to the ticket.
func (b *Bot) removeFromTicket(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, target discord.User) error {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return err
	}
	if !isStaff(m, tt) {
		return userErr("Only support staff can remove people from tickets.")
	}
	if target.ID == t.OpenerID {
		return userErr("You can't remove the person who opened the ticket.")
	}
	if t.Mode == store.ModeThread {
		return b.client.Rest.RemoveThreadMember(t.ChannelID, target.ID, rest.WithCtx(ctx))
	}
	return b.client.Rest.DeletePermissionOverwrite(t.ChannelID, target.ID, rest.WithCtx(ctx))
}

// renameTicket renames the ticket's channel or thread.
func (b *Bot) renameTicket(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember, name string) error {
	t, tt, err := b.loadTicket(ctx, channelID)
	if err != nil {
		return err
	}
	if !isStaff(m, tt) {
		return userErr("Only support staff can rename tickets.")
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return userErr("Names must be between 1 and 100 characters.")
	}

	var update discord.ChannelUpdate = discord.GuildTextChannelUpdate{Name: &name}
	if t.Mode == store.ModeThread {
		update = discord.GuildThreadUpdate{Name: &name}
	}
	_, err = b.client.Rest.UpdateChannel(t.ChannelID, update, rest.WithCtx(ctx))
	if errors.Is(err, context.DeadlineExceeded) {
		// disgo waits out rate limits; renames are limited to 2 per 10 minutes.
		return userErr("Discord only allows renaming a channel twice every 10 minutes. Try again later.")
	}
	return err
}
