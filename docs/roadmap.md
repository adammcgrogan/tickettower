# Roadmap

## Done (v1 milestones 1–6)

- Foundation: config, migrations, bot connects, Discord OAuth dashboard login, server picker, invite flow
- Ticket types (channel or private thread), panels (buttons/dropdown) with live preview and publish/republish
- Ticket lifecycle: open (with per-member limit), claim/unclaim, close with reason, add/remove member, rename; slash commands mirror the buttons
- Transcripts: capture, Discord-style viewer, retention setting, access for opener/managers/support staff
- Feedback (1–5 + comment via DM) and analytics (volume, median first response/resolution, satisfaction, by type, by staff)
- Landing page, privacy page, CI workflow, README deploy guide

## Done since v1

- Dashboard roles: members with roles chosen in Settings can use the dashboard, at a level per role: viewer (read only), support (acts on tickets) or admin (everything except changing those roles)
- Log channel: ticket opened/claimed/closed events are posted to a channel chosen in Settings, with a transcript link on close when `PUBLIC_URL` is https
- Pre-ticket forms: up to 5 questions per ticket type, asked in a modal when a member opens a ticket; answers are posted in the welcome message (and so appear in transcripts)
- Dashboard redesign: new visual identity (ink navy, signal amber, Big Shoulders Display for titles and ticket numbers), a sidebar with a server switcher, a Home page with a "waiting on your team" queue and one-step quick setup, a two-pane Tickets inbox with search and inline transcripts, and live Discord previews in the ticket type and panel editors
- Auto-close: per ticket type, close tickets after 12h–1 week without messages, but only when the team is waiting on the member. The member gets a "Still need help?" warning first (a quarter of the window ahead, at most a day), and any message or the "I still need help" button keeps it open
- Deeper analytics: 7 days to all time (weekly or monthly buckets past 90 days), a ticket type filter, comparison with the previous period, open-now and backlog trend, a busiest-times heatmap, first response by hour, one-reply resolution, messages per ticket, tickets closed without a reply, how tickets were closed (team, member, auto-close, channel deleted; `tickets.auto_closed`), top close reasons, rating distribution, per-type performance and a team table with replies

- Setup friction pass:
  - a setup check finds missing bot permissions, deleted channels, ticket types nobody can choose and broken ticket buttons, and Home and the ticket type pages show each problem with a link to its fix
  - tickets can be closed from the dashboard
  - quick setup asks who handles tickets
  - a new ticket type can go straight onto existing ticket buttons
  - logging in from a transcript link comes back to the transcript
  - the bot posts a welcome with a dashboard link when it's added to a server

- Replies from the dashboard: a reply box under an open ticket's conversation on the Tickets page. The bot posts it as an embed with the staff member's name and avatar (no extra permissions needed), and it's saved to the transcript, counts as the team's first response and restarts the auto-close clock

- Help: short guides at `/help` (public, no login), linked from the sidebar, the landing page, "Learn more" on form hints (`Field`'s `help` prop) and setup check problems, plus a "Get support" link (`VITE_SUPPORT_URL`, defaulting to GitHub issues)

- Placeholders: welcome messages take `{user}`, `{username}`, `{number}`, `{type}`, `{server}`, `{support}` and `{answer1}`–`{answer5}`; channel and thread names take `{number}`, `{username}`, `{type}` and the answers (slugged). The editor lists them as clickable chips and fills them in the previews

- Moving tickets: `/ticket move` (with autocomplete) and a Move action on the Tickets page change a ticket's type. Channels move to the new category and swap support role access; threads ping the new roles in. Moving between channel and thread types isn't supported

- Landing page that sells: reassurances under the hero, three setup steps, a five-step walkthrough of one ticket drawn as Discord shows it (`landing/Walkthrough.svelte`, copy mirrors the bot's real embeds), a dashboard Home demo (`landing/QueueDemo.svelte`), a 12-item feature grid, slash commands, open source and privacy, an FAQ and a closing call to action

- Starter templates: quick setup (`QuickSetup.svelte`, on Home for a new server and on an empty ticket types page) offers Community or gaming, Store or business and Creator templates from `lib/templates.ts`, each creating three ticket types with questions, welcome messages and auto-close, plus ticket buttons for them. "Start simple" keeps the single named type

- Saved replies: a Saved replies page under Setup (name and message, up to 50 per server, `MaxSavedReplies`), a picker on the Tickets page reply box that inserts one to edit before sending, and `/reply` (with autocomplete, support staff only) that posts one as a staff reply embed. Staff replies take `{user}`, `{username}`, `{number}`, `{type}`, `{server}` and `{staff}` (`replyText`), and `/reply` counts as the team's response like dashboard replies

- Tickets page search and pagination: search (number, member, claimer, type), the status and ticket type filters run in the database, and "Load older tickets" pages back through every ticket

- Search inside messages: the Tickets page search also matches words in a ticket's messages (including dashboard replies and form answers), with a highlighted snippet under each result

- Who can open tickets: per ticket type, required roles, blocked roles and a wait between a member's tickets of that type; plus a server-wide block list (`/ticket block` with a reason the member sees, `/ticket unblock`, and a Blocked members section in Settings)

- Rating controls: per ticket type, turn the closing rating request off or reword it (`{staff}` and friends); ratings and comments are posted to the log channel as they arrive, and Analytics says "Not asked" for types that don't ask

- Reopen: closed thread tickets can be reopened from the close message, the closing DM or the Tickets page (the thread is unarchived and tracked again; channel tickets can't be, since the channel is gone). Members typing `/close` on their own ticket without a reason are asked to confirm

- Ticket buttons that match the server: per ticket type, a button colour (blurple, grey, green, red) and a label that can differ from the name; per set of buttons, a banner image, a thumbnail and the dropdown's prompt. All shown in the editors' previews

- Claim lock: per ticket type, claiming a channel ticket can make it read only for the rest of the team or hide it from them, with roles that keep full access
- On hold: `/ticket hold`, `/ticket resume` and the Tickets page park tickets waiting on something else; they leave the queue, reminders and auto-close until the member writes or staff resume
- Reply targets and staff reminders: per ticket type, a reply target measured in Analytics (overall and per type) with overdue tickets marked on Home and the Tickets page, and reminders to the team after a wait, in the ticket or the log channel, mentioning the claimer or the roles, once or repeating

- Easier dashboard navigation: the ticket type editor is split into tabs (Basics, Button, Questions and welcome, Who can open, While open, Closing) with the button and welcome previews always beside it; a save error opens the tab with the problem. Its Button tab shows and changes which ticket buttons the type is on, for new and existing types, and the Ticket types list says where each type appears. The open Tickets queue is grouped by whose turn it is, with rarer ticket actions in a More menu. Settings uses quiet rows with a save bar scoped to the settings it saves, apart from Blocked members, which apply straight away

- Reopen channel tickets: per ticket type (Closing tab), a closed channel can be kept in a closed category, read only (or hidden from the member), for 1–30 days before it's deleted (`closed_parent_id`, `closed_member_access`, `closed_keep_days`; `tickets.channel_kept_until`). Kept tickets reopen from the close message, the DM or the Tickets page, which restores the channel's access and category; the auto-close sweep deletes kept channels when due. The setup check warns when the closed category is deleted or full

- Claim and assign from the dashboard: Claim and Unclaim next to Close ticket on the Tickets page, and "Assign to someone" in the More menu with a name search of the type's support staff and server managers (`…/tickets/{id}/claim` with an optional `user_id`, `…/unclaim`, `…/assignees?q=`). The bot posts the usual claim message, moves any claim lock and logs it (`Dashboard.Claim/Assign/Unclaim`)

- Ratings next to the ticket: a closed ticket's stars, comment and when it was rated show in its summary on the Tickets page and the transcript, and the Tickets list marks rated tickets with their score (`Ticket.feedback`, a `LEFT JOIN ticket_feedback` in every ticket read)

- Data deletion: server managers can delete everything stored for their server from Settings (typed confirmation, only once every ticket is closed; published ticket buttons are removed from Discord too), and the data of servers the bot has left is purged after 30 days (`leftGuildRetention`, run with the hourly transcript purge). `/privacy` says so

## Not yet verified by hand

Click through a real ticket end to end in Discord: open from a panel in both channel and thread modes, claim, add/remove, rename, close, the DM rating and comment, then the transcript in the dashboard.

## Next up (suggested order)

1. **Deploy to Railway**: set up the services and env vars, set a real `PUBLIC_URL` (which enables transcript links in DMs), unset `DEV_GUILD_ID`, and add the production OAuth redirect. Reset the bot token/secret first (they were shared in a chat).
2. **Transcript attachments**: archive attachments to object storage (e.g. Cloudflare R2), since Discord CDN links expire.
3. **Sharding**: before large scale, remove the single-process assumptions (the `onGuildsReady` reconcile, in-memory `ticketCache` and `openLocks`). Options are Redis-backed locks and cache, or per-shard reconcile. The auto-close job is already safe to run in several processes.
4. **Premium**: Discord App Subscriptions → `entitlements` rows → higher `Limits`.

## Nice to have

- Light theme (the tokens are ready for it)
- Timezone-aware analytics (currently UTC)
- Custom bot branding per server
