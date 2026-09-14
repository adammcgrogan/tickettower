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
	"unicode"
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
	// cleanupGrace is how long after closing a ticket the sweep waits before
	// retrying its channel cleanup, so it never races a close in progress.
	cleanupGrace = 30 * time.Second

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
// roles are the member's roles, for types that restrict who can open them.
// form holds the member's answers if the type has questions.
func (b *Bot) openTicket(ctx context.Context, guildID snowflake.ID, user discord.User, roles []snowflake.ID, typeID int64, form *formSubmission) (snowflake.ID, error) {
	defer b.lockMember(guildID, user.ID)()

	tt, err := b.store.GetTicketType(ctx, guildID, typeID)
	if errors.Is(err, store.ErrNotFound) {
		return 0, userErr("This ticket type no longer exists. Ask a server admin to update the ticket buttons.")
	} else if err != nil {
		return 0, err
	}

	st, err := b.loadOpener(ctx, guildID, user.ID, roles, tt)
	if err != nil {
		return 0, err
	}
	if err := accessErr(tt, st, time.Now()); err != nil {
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
	name := channelName(tt.NameFormat, number, user, tt.Name, answers)

	var channelID snowflake.ID
	switch tt.Mode {
	case store.ModeThread:
		invitable := false
		thread, err := b.rest.CreateThread(*tt.ParentID, discord.GuildPrivateThreadCreate{
			Name:                name,
			AutoArchiveDuration: discord.AutoArchiveDuration1w,
			Invitable:           &invitable,
		}, rest.WithCtx(ctx))
		if err != nil {
			return 0, err
		}
		channelID = thread.ID()
		if err := b.rest.AddThreadMember(channelID, user.ID, rest.WithCtx(ctx)); err != nil {
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
		ch, err := b.rest.CreateGuildChannel(guildID, create, rest.WithCtx(ctx))
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
	b.tickets.put(store.TicketRef{ID: ticket.ID, GuildID: guildID, ChannelID: channelID, OpenerID: user.ID})

	welcome := welcomeMessage(ticket, tt, answers, b.guildName(ctx, guildID))
	if _, err := b.rest.CreateMessage(channelID, welcome, rest.WithCtx(ctx)); err != nil {
		b.log.Warn("failed to send welcome message", slog.Int64("ticket_id", ticket.ID), slog.Any("err", err))
		// Whatever Discord disliked about it, the ticket still needs its
		// pings and buttons.
		if _, err := b.rest.CreateMessage(channelID, welcomeFallback(ticket, tt), rest.WithCtx(ctx)); err != nil {
			b.log.Error("failed to send fallback welcome message", slog.Int64("ticket_id", ticket.ID), slog.Any("err", err))
		}
	}
	go b.logEvent(guildID, openedLog(ticket))
	b.log.Info("ticket opened",
		slog.String("guild_id", guildID.String()), slog.Int("number", number), slog.String("mode", string(tt.Mode)))
	return channelID, nil
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

// answerPlaceholders returns "{answer1}" to "{answer5}" paired with the
// member's answers (empty when there's no such question), each passed
// through clean.
func answerPlaceholders(answers []formAnswer, clean func(string) string) []string {
	pairs := make([]string, 0, 2*store.MaxQuestions)
	for i := range store.MaxQuestions {
		var v string
		if i < len(answers) {
			v = clean(answers[i].answer)
		}
		pairs = append(pairs, fmt.Sprintf("{answer%d}", i+1), v)
	}
	return pairs
}

// slug lowercases s and joins its words with hyphens, e.g. "Billing & refunds"
// becomes "billing-refunds".
func slug(s string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(s) {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			gap = true
			continue
		}
		if gap && b.Len() > 0 {
			b.WriteByte('-')
		}
		gap = false
		b.WriteRune(r)
	}
	return b.String()
}

// channelName fills in a ticket type's name format, e.g. "ticket-{number}".
// Placeholders are replaced in one pass, so a member's answer can't inject
// another placeholder.
func channelName(format string, number int, user discord.User, typeName string, answers []formAnswer) string {
	pairs := []string{
		"{number}", fmt.Sprintf("%04d", number),
		"{username}", user.Username,
		"{user}", user.Username,
		"{type}", slug(typeName),
	}
	pairs = append(pairs, answerPlaceholders(answers, slug)...)
	name := strings.NewReplacer(pairs...).Replace(format)
	// An empty placeholder can leave a stray hyphen, e.g. "{answer1}-{number}".
	name = strings.Trim(name, " -")
	if name == "" {
		name = fmt.Sprintf("ticket-%04d", number)
	}
	if r := []rune(name); len(r) > 100 {
		name = string(r[:100])
	}
	return name
}

// Discord's caps on an embed: the total text across a message's embeds, one
// description, and one field's value.
const (
	embedLimit           = 6000
	embedFieldValueLimit = 1024
)

// welcomeText fills in the placeholders in a ticket type's welcome message.
// Mentions inside an embed are shown but don't ping anyone.
func welcomeText(t store.Ticket, tt store.TicketType, answers []formAnswer, server string) string {
	text := tt.WelcomeMessage
	if strings.TrimSpace(text) == "" {
		text = defaultWelcome
	}
	support := "the team"
	if len(tt.SupportRoleIDs) > 0 {
		roles := make([]string, len(tt.SupportRoleIDs))
		for i, id := range tt.SupportRoleIDs {
			roles[i] = discord.RoleMention(id)
		}
		support = strings.Join(roles, " ")
	}
	pairs := []string{
		"{user}", discord.UserMention(t.OpenerID),
		"{username}", t.OpenerName,
		"{number}", fmt.Sprintf("%04d", t.Number),
		"{type}", tt.Name,
		"{server}", server,
		"{support}", support,
	}
	pairs = append(pairs, answerPlaceholders(answers, strings.TrimSpace)...)
	return strings.NewReplacer(pairs...).Replace(text)
}

func welcomeMessage(t store.Ticket, tt store.TicketType, answers []formAnswer, server string) discord.MessageCreate {
	text := welcomeText(t, tt, answers, server)

	title := fmt.Sprintf("%s · #%d", tt.Name, t.Number)
	const footer = "Staff can claim this ticket. Either side can close it when you're done."
	embed := discord.NewEmbed().
		WithTitle(title).
		WithColor(colorAccent).
		WithFooterText(footer)

	budget := embedLimit - utf8.RuneCountInString(title) - utf8.RuneCountInString(footer)
	for _, a := range answers {
		value := truncate(a.answer, embedFieldValueLimit)
		if strings.TrimSpace(value) == "" {
			value = "*No answer*"
		}
		embed = embed.AddField(a.question, value, false)
		budget -= utf8.RuneCountInString(a.question) + utf8.RuneCountInString(value)
	}
	// Long answers win over a long welcome message if both can't fit, and
	// the description has a cap of its own, which placeholders such as
	// {answer1} can push the text past.
	embed = embed.WithDescription(truncate(text, min(budget, embedDescriptionLimit)))

	return ticketMentions(discord.NewMessageCreate().WithEmbeds(embed).AddActionRow(ticketControls()...), t, tt)
}

// ticketControls are the Claim and Close buttons under a ticket's welcome.
func ticketControls() []discord.InteractiveComponent {
	return []discord.InteractiveComponent{
		discord.NewSecondaryButton("Claim", claimButtonID).WithEmoji(discord.ComponentEmoji{Name: "🙋"}),
		discord.NewDangerButton("Close", closeButtonID).WithEmoji(discord.ComponentEmoji{Name: "🔒"}),
	}
}

// ticketMentions pings the opener and the support roles in a message's
// content, which also adds the roles to a private thread.
func ticketMentions(msg discord.MessageCreate, t store.Ticket, tt store.TicketType) discord.MessageCreate {
	mentions := []string{discord.UserMention(t.OpenerID)}
	for _, id := range tt.SupportRoleIDs {
		mentions = append(mentions, discord.RoleMention(id))
	}
	return msg.WithContent(strings.Join(mentions, " ")).
		WithAllowedMentions(&discord.AllowedMentions{Users: []snowflake.ID{t.OpenerID}, Roles: tt.SupportRoleIDs})
}

// welcomeFallback is posted if Discord rejects the welcome message, so the
// ticket still pings everyone and has its buttons.
func welcomeFallback(t store.Ticket, tt store.TicketType) discord.MessageCreate {
	embed := discord.NewEmbed().
		WithTitle(fmt.Sprintf("%s · #%d", tt.Name, t.Number)).
		WithDescription("Thanks for reaching out! Someone from the team will be with you shortly.").
		WithColor(colorAccent)
	return ticketMentions(discord.NewMessageCreate().WithEmbeds(embed).AddActionRow(ticketControls()...), t, tt)
}

// cleanupChannel deletes a half-created ticket channel after a failure.
func (b *Bot) cleanupChannel(channelID snowflake.ID) {
	if err := b.rest.DeleteChannel(channelID); err != nil && !discordx.IsCode(err, discordx.CodeUnknownChannel) {
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
		b.releaseClaimLock(ctx, t, tt, me)
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
	b.applyClaimLock(ctx, t, tt, me)
	go b.logEvent(t.GuildID, claimedLog(t, me))
	return "🙋 " + discord.UserMention(me) + " has claimed this ticket and will help you from here." + claimLockNote(t, tt, me), nil
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
// were closed automatically. Thread tickets get a Reopen button, since the
// thread is only archived.
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
	msg := discord.NewMessageCreate().
		WithEmbeds(discord.NewEmbed().WithTitle("Ticket closed").WithDescription(desc).WithColor(colorMuted)).
		WithAllowedMentions(&discord.AllowedMentions{})
	if t.Mode == store.ModeThread {
		msg = msg.AddActionRow(reopenButton(t.ID))
	}
	return msg
}

// reopenButton reopens a closed thread ticket. It's on the close message and
// in the opener's DM.
func reopenButton(ticketID int64) discord.ButtonComponent {
	return discord.NewSecondaryButton("Reopen", fmt.Sprintf("%s%d", reopenButtonPrefix, ticketID)).
		WithEmoji(discord.ComponentEmoji{Name: "🔓"})
}

// reopenTicket reopens a closed thread ticket: the thread is unarchived and
// unlocked, and the ticket tracked again. permitted decides, given the
// ticket's type (nil if deleted), whether the person may; dashboard users
// have already been checked. It returns the reopened ticket.
func (b *Bot) reopenTicket(ctx context.Context, t store.Ticket, byID snowflake.ID, permitted func(*store.TicketType) bool) (store.Ticket, error) {
	if t.Mode != store.ModeThread {
		return t, userErr("This ticket's channel was deleted when it closed, so it can't be reopened. Open a new ticket instead.")
	}
	if t.Status == store.StatusOpen {
		return t, userErr("This ticket is already open.")
	}
	var tt *store.TicketType
	if t.TicketTypeID != nil {
		if v, err := b.store.GetTicketType(ctx, t.GuildID, *t.TicketTypeID); err == nil {
			tt = &v
		}
	}
	if permitted != nil && !permitted(tt) {
		return t, userErr("Only the person who opened this ticket or support staff can reopen it.")
	}
	archived, locked := false, false
	_, err := b.rest.UpdateChannel(t.ChannelID, discord.GuildThreadUpdate{Archived: &archived, Locked: &locked}, rest.WithCtx(ctx))
	if discordx.IsCode(err, discordx.CodeUnknownChannel) {
		return t, userErr("This ticket's thread was deleted, so it can't be reopened. Open a new ticket instead.")
	} else if err != nil {
		return t, err
	}
	ok, err := b.store.ReopenTicket(ctx, t.ID, time.Now())
	if err != nil {
		return t, err
	}
	if !ok {
		return t, userErr("This ticket is already open.")
	}
	t, err = b.store.GetTicket(ctx, t.ID)
	if err != nil {
		return t, err
	}
	b.tickets.put(store.TicketRef{ID: t.ID, GuildID: t.GuildID, ChannelID: t.ChannelID, OpenerID: t.OpenerID, HasFirstResponse: t.FirstResponseAt != nil})
	go b.logEvent(t.GuildID, reopenedLog(t, byID))
	b.log.Info("ticket reopened", slog.String("guild_id", t.GuildID.String()), slog.Int("number", t.Number))
	return t, nil
}

// reopenedMessage is posted in the thread when a ticket reopens.
func reopenedMessage(byID snowflake.ID) discord.MessageCreate {
	return discord.NewMessageCreate().
		WithEmbeds(discord.NewEmbed().
			WithTitle("Ticket reopened").
			WithDescription("Reopened by "+discord.UserMention(byID)+". Pick up where you left off.").
			WithColor(colorAccent)).
		AddActionRow(
			discord.NewSecondaryButton("Claim", claimButtonID).WithEmoji(discord.ComponentEmoji{Name: "🙋"}),
			discord.NewDangerButton("Close", closeButtonID).WithEmoji(discord.ComponentEmoji{Name: "🔒"}),
		).
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

	if t.Mode != store.ModeThread {
		time.Sleep(closeDelay)
	}
	b.cleanUpChannel(ctx, t)
}

// cleanUpChannel deletes a closed ticket's channel, or archives and locks
// its thread, and records that it's done. A failure is only logged: the
// cleanup sweep retries it, so a restart or a revoked permission never
// leaves a channel behind for good.
func (b *Bot) cleanUpChannel(ctx context.Context, t store.Ticket) {
	var err error
	switch t.Mode {
	case store.ModeThread:
		archived, locked := true, true
		_, err = b.rest.UpdateChannel(t.ChannelID, discord.GuildThreadUpdate{Archived: &archived, Locked: &locked}, rest.WithCtx(ctx))
	default:
		err = b.rest.DeleteChannel(t.ChannelID, rest.WithCtx(ctx))
	}
	if err != nil && !discordx.IsCode(err, discordx.CodeUnknownChannel) {
		b.log.Warn("failed to clean up closed ticket, will retry", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
		return
	}
	if err := b.store.MarkChannelCleaned(ctx, t.ID); err != nil {
		b.log.Error("failed to mark ticket channel cleaned", slog.Int64("ticket_id", t.ID), slog.Any("err", err))
	}
}

// cleanUpClosedTickets retries the channel cleanup of tickets that closed a
// while ago but whose channel is still around, for instance because the bot
// restarted during the close delay or lacked Manage Channels at the time.
func (b *Bot) cleanUpClosedTickets(ctx context.Context, now time.Time) {
	due, err := b.store.TicketsToCleanUp(ctx, now.Add(-cleanupGrace))
	if err != nil {
		if ctx.Err() == nil {
			b.log.Error("failed to find closed tickets to clean up", slog.Any("err", err))
		}
		return
	}
	for _, t := range due {
		b.cleanUpChannel(ctx, t)
	}
	if len(due) > 0 {
		b.log.Info("retried cleanup of closed ticket channels", slog.Int("count", len(due)))
	}
}

func (b *Bot) notifyOpener(ctx context.Context, t store.Ticket) {
	dm, err := b.rest.CreateDMChannel(t.OpenerID, rest.WithCtx(ctx))
	if err != nil {
		return
	}
	var tt *store.TicketType
	if t.TicketTypeID != nil {
		if v, err := b.store.GetTicketType(ctx, t.GuildID, *t.TicketTypeID); err == nil {
			tt = &v
		}
	}
	desc, ask := closedDM(t, tt, b.guildName(ctx, t.GuildID))
	embed := discord.NewEmbed().WithTitle("Ticket closed").WithDescription(desc).WithColor(colorMuted)
	msg := discord.NewMessageCreate().WithEmbeds(embed)
	if ask {
		msg = msg.WithComponents(b.feedbackComponents(t.ID)...)
	} else {
		msg = msg.WithComponents(b.transcriptRow(t.ID)...)
	}
	if t.Mode == store.ModeThread {
		msg = msg.AddActionRow(reopenButton(t.ID))
	}
	_, err = b.rest.CreateMessage(dm.ID(), msg, rest.WithCtx(ctx))
	if err != nil && !discordx.IsCode(err, discordx.CodeCannotDMUser) {
		b.log.Warn("failed to DM ticket opener", slog.Any("err", err))
	}
}

// defaultRatingPrompt asks for a rating when a ticket type has no wording
// of its own.
const defaultRatingPrompt = "How did we do? Rate your experience below."

// closedDM is the text of the DM an opener gets when their ticket closes,
// and whether it asks for a rating. tt is nil when the type was deleted, in
// which case the rating is asked for as usual.
func closedDM(t store.Ticket, tt *store.TicketType, server string) (string, bool) {
	desc := fmt.Sprintf("Your **%s** ticket (#%d) in **%s** has been closed.", t.TypeName, t.Number, server)
	if t.CloseReason != "" {
		desc += "\n**Reason:** " + t.CloseReason
	}
	if tt != nil && !tt.AskRating {
		return desc, false
	}
	prompt := defaultRatingPrompt
	if tt != nil && strings.TrimSpace(tt.RatingPrompt) != "" {
		staff := "the team"
		if t.ClaimedByName != nil && *t.ClaimedByName != "" {
			staff = *t.ClaimedByName
		}
		prompt = strings.NewReplacer(
			"{staff}", staff,
			"{server}", server,
			"{type}", t.TypeName,
			"{number}", fmt.Sprintf("%04d", t.Number),
			"{username}", t.OpenerName,
		).Replace(tt.RatingPrompt)
	}
	return desc + "\n\n" + prompt, true
}

// guildName prefers the gateway cache, which the dashboard doesn't have.
func (b *Bot) guildName(ctx context.Context, id snowflake.ID) string {
	if b.client != nil {
		if g, ok := b.client.Caches.Guild(id); ok {
			return g.Name
		}
	}
	if g, err := b.store.GetGuild(ctx, id); err == nil {
		return g.Name
	}
	return "the server"
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
		return b.rest.AddThreadMember(t.ChannelID, target.ID, rest.WithCtx(ctx))
	}
	allow := ticketMemberPerms
	return b.rest.UpdatePermissionOverwrite(t.ChannelID, target.ID,
		discord.MemberPermissionOverwriteUpdate{Allow: &allow}, rest.WithCtx(ctx))
}

// removeFromTicket revokes a user's access to the ticket. It only reports
// success when access actually changed: someone who can see the ticket
// through a support role or Administrator keeps it, and someone who was
// never added has nothing to remove. targetMember is nil if the person has
// left the server.
func (b *Bot) removeFromTicket(ctx context.Context, channelID snowflake.ID, m *discord.ResolvedMember,
	target discord.User, targetMember *discord.ResolvedMember) error {
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
		if _, err := b.rest.GetThreadMember(t.ChannelID, target.ID, false, rest.WithCtx(ctx)); discordx.IsCode(err, discordx.CodeUnknownMember) {
			return userErr("%s isn't in this ticket.", discord.UserMention(target.ID))
		}
		return b.rest.RemoveThreadMember(t.ChannelID, target.ID, rest.WithCtx(ctx))
	}
	if err := keepsAccess(target.ID, targetMember, tt); err != nil {
		return err
	}
	ch, err := b.rest.GetChannel(t.ChannelID, rest.WithCtx(ctx))
	if err != nil {
		return err
	}
	if gc, ok := ch.(discord.GuildChannel); ok {
		if _, added := gc.PermissionOverwrites().Member(target.ID); !added {
			return userErr("%s isn't in this ticket.", discord.UserMention(target.ID))
		}
	}
	return b.rest.DeletePermissionOverwrite(t.ChannelID, target.ID, rest.WithCtx(ctx))
}

// keepsAccess explains why removing a member's overwrite from a ticket
// channel wouldn't stop them seeing it, or returns nil if it would.
func keepsAccess(targetID snowflake.ID, target *discord.ResolvedMember, tt *store.TicketType) error {
	if target == nil {
		return nil
	}
	if target.Permissions.Has(discord.PermissionAdministrator) {
		return userErr("%s is an administrator, so they can see every channel. Removing them from the ticket wouldn't change that.",
			discord.UserMention(targetID))
	}
	if tt != nil {
		for _, id := range target.RoleIDs {
			if slices.Contains(tt.SupportRoleIDs, id) {
				return userErr("%s can see this ticket through the %s support role, so removing them wouldn't change that. Take the role away if you need to.",
					discord.UserMention(targetID), discord.RoleMention(id))
			}
		}
	}
	return nil
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
	_, err = b.rest.UpdateChannel(t.ChannelID, update, rest.WithCtx(ctx))
	if errors.Is(err, context.DeadlineExceeded) {
		// disgo waits out rate limits; renames are limited to 2 per 10 minutes.
		return userErr("Discord only allows renaming a channel twice every 10 minutes. Try again later.")
	}
	return err
}
