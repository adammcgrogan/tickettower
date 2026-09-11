# Roadmap

## Done (v1 milestones 1–6)

- Foundation: config, migrations, bot connects, Discord OAuth dashboard login, server picker, invite flow
- Ticket types (channel or private thread), panels (buttons/dropdown) with live preview and publish/republish
- Ticket lifecycle: open (with per-member limit), claim/unclaim, close with reason, add/remove member, rename; slash commands mirror the buttons
- Transcripts: capture, Discord-style viewer, retention setting, access for opener/managers/support staff
- Feedback (1–5 + comment via DM) and analytics (volume, median first response/resolution, satisfaction, by type, by staff)
- Landing page, privacy page, CI workflow, README deploy guide

## Not yet verified by hand

Click through a real ticket end to end in Discord: open from a panel in both channel and thread modes, claim, add/remove, rename, close, the DM rating and comment, then the transcript in the dashboard.

## Next up (suggested order)

1. **Deploy to Railway**: set up the services and env vars, set a real `PUBLIC_URL` (which enables transcript links in DMs), unset `DEV_GUILD_ID`, and add the production OAuth redirect. Reset the bot token/secret first (they were shared in a chat).
2. **Dashboard roles**: let members with configured roles use the dashboard, not just Manage Server (`guild_settings.dashboard_role_ids` already exists; see the TODO on `canManage` in `internal/api/server.go`).
3. **Log channel**: post open/close/claim events (and optionally transcripts) to a chosen channel.
4. **Pre-ticket forms**: questions shown in a modal when opening a ticket, with answers posted in the welcome message. This is part of the original plan's "Forms & automation".
5. **Auto-close**: close inactive tickets after N hours, with a warning first.
6. **Reopen**: reopen threads within a grace window (channels are deleted, so reopening them would need a delay or archiving instead).
7. **Canned responses / tags**: `/tag` for common replies, managed in the dashboard.
8. **Transcript attachments**: archive attachments to object storage (e.g. Cloudflare R2), since Discord CDN links expire.
9. **Sharding**: before large scale, remove the single-process assumptions (the `onGuildsReady` reconcile, in-memory `ticketCache` and `openLocks`). Options are Redis-backed locks and cache, or per-shard reconcile.
10. **Premium**: Discord App Subscriptions → `entitlements` rows → higher `Limits`.

## Nice to have

- Light theme (the tokens are ready for it)
- Timezone-aware analytics (currently UTC)
- Search and pagination on the Tickets page (currently the newest 200)
- Custom bot branding per server
- Rename the product and pick a domain
