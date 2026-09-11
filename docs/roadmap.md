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

## Not yet verified by hand

Click through a real ticket end to end in Discord: open from a panel in both channel and thread modes, claim, add/remove, rename, close, the DM rating and comment, then the transcript in the dashboard.

## Next up (suggested order)

1. **Deploy to Railway**: set up the services and env vars, set a real `PUBLIC_URL` (which enables transcript links in DMs), unset `DEV_GUILD_ID`, and add the production OAuth redirect. Reset the bot token/secret first (they were shared in a chat).
2. **Reopen**: reopen threads within a grace window (channels are deleted, so reopening them would need a delay or archiving instead).
3. **Canned responses / tags**: `/tag` for common replies, managed in the dashboard.
4. **Transcript attachments**: archive attachments to object storage (e.g. Cloudflare R2), since Discord CDN links expire.
5. **Sharding**: before large scale, remove the single-process assumptions (the `onGuildsReady` reconcile, in-memory `ticketCache` and `openLocks`). Options are Redis-backed locks and cache, or per-shard reconcile. The auto-close job is already safe to run in several processes.
6. **Premium**: Discord App Subscriptions → `entitlements` rows → higher `Limits`.

## Nice to have

- Light theme (the tokens are ready for it)
- Timezone-aware analytics (currently UTC)
- Search and pagination on the Tickets page (currently the newest 200)
- Custom bot branding per server
