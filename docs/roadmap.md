# Roadmap

## Done (v1 milestones 1–6)

- Foundation: config, migrations, bot connects, Discord OAuth dashboard login, server picker, invite flow
- Ticket types (channel or private thread), panels (buttons/dropdown) with live preview and publish/republish
- Ticket lifecycle: open (with per-member limit), claim/unclaim, close with reason, add/remove member, rename; slash commands mirror the buttons
- Transcripts: capture, Discord-style viewer, retention setting, access for opener/managers/support staff
- Feedback (1–5 + comment via DM) and analytics (volume, median first response/resolution, satisfaction, by type, by staff)
- Landing page, privacy page, CI workflow, README deploy guide

## Done since v1

- Dashboard roles: members with roles chosen in Settings can use the dashboard (everything except changing those roles)
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

- Who can open tickets: per ticket type, required roles, blocked roles and a wait between a member's tickets of that type; plus a server-wide block list (`/ticket block` with a reason the member sees, `/ticket unblock`, and a Blocked members section in Settings)

## Not yet verified by hand

Click through a real ticket end to end in Discord: open from a panel in both channel and thread modes, claim, add/remove, rename, close, the DM rating and comment, then the transcript in the dashboard.

## Next up (suggested order)

1. **Deploy to Railway**: set up the services and env vars, set a real `PUBLIC_URL` (which enables transcript links in DMs), unset `DEV_GUILD_ID`, and add the production OAuth redirect. Reset the bot token/secret first (they were shared in a chat).
2. **Reopen**: reopen threads within a grace window (channels are deleted, so reopening them would need a delay or archiving instead).
3. **Transcript attachments**: archive attachments to object storage (e.g. Cloudflare R2), since Discord CDN links expire.
4. **Sharding**: before large scale, remove the single-process assumptions (the `onGuildsReady` reconcile, in-memory `ticketCache` and `openLocks`). Options are Redis-backed locks and cache, or per-shard reconcile. The auto-close job is already safe to run in several processes.
5. **Premium**: Discord App Subscriptions → `entitlements` rows → higher `Limits`.

## Nice to have

- Light theme (the tokens are ready for it)
- Timezone-aware analytics (currently UTC)
- Custom bot branding per server
