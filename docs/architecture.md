# Architecture

```
            Discord gateway                        Browser (SvelteKit SPA)
                   │                                        │  cookie session
                   ▼                                        ▼
            ┌─────────────┐                          ┌─────────────┐     ┌───────┐
            │  cmd/bot    │                          │  cmd/api    │────▶│ Redis │ sessions, OAuth state,
            │  (disgo)    │                          │  (chi)      │     └───────┘ user guild cache
            └──────┬──────┘                          └──┬───────┬──┘
                   │            ┌──────────┐            │       │ bot-token REST: publish panels,
                   └───────────▶│ Postgres │◀───────────┘       │ list channels/roles, GetMember
                                └──────────┘                    ▼
                                                            Discord REST
```

The bot and API don't talk to each other directly. They share Postgres, and the API calls Discord's REST API itself with the bot token, which gives the dashboard immediate errors such as "missing permissions".

## Data model (`internal/store/migrations`)

| Table | Purpose |
|---|---|
| `guilds` | Servers the bot is or was in (`left_at` set when removed; data kept for re-adds) |
| `guild_settings` | Per-guild settings: `ticket_counter` (sequential ticket numbers), `transcript_retention_days`, `dashboard_roles` (JSONB list of `{role_id, level}` granting dashboard access), `log_channel_id` |
| `entitlements` | Plan tier per guild (absent means free) → `internal/entitlements.ForTier` |
| `ticket_types` | Name, emoji, `mode` (channel/thread), `parent_id` (category or channel), support roles, name format, welcome message, per-member limit, `questions` (JSONB form, up to 5), `auto_close_hours`, who can open it (`required_role_ids`, `blocked_role_ids`, `cooldown_minutes`), `ask_rating` and `rating_prompt`, `claim_lock` with `claim_lock_exempt_role_ids`, `reply_target_minutes` and the reminder settings (`reminder_minutes`, `reminder_repeat`, `reminder_where`, `reminder_ping`) |
| `ticket_blocks` | Members who can't open any ticket in a guild, with the reason (shown to them) and who blocked them |
| `panels` + `panel_ticket_types` | Panel content/style, optional `image_url`, `thumbnail_url` and dropdown `placeholder`, where it's published (`channel_id`, `message_id`), and its ordered ticket types. Each ticket type's `button_style` (Discord colour) and `button_label` decide how it looks on a panel |
| `tickets` | One per ticket: number, type snapshot (`type_name`), channel, opener/claimer/closer with name snapshots, status, timestamps incl. `first_response_at` and `reopened_at`, plus auto-close state (`last_activity_at`, `waiting_on_staff`, `auto_close_warned_at`), the wait clock and reminders (`waiting_since`, `staff_reminded_at`, `reminders_sent`), `on_hold` with `hold_reason` and `channel_cleaned_at` (when the bot deleted or archived the channel after closing) |
| `ticket_messages` | Transcript messages (content, embeds/attachments as JSONB, edit/delete flags, `sent_by` for replies the bot posted for a staff member), plus `search`, a generated `tsvector` over the text and embed text (`ticket_message_text`) with a GIN index |
| `ticket_feedback` | 1–5 rating + comment per ticket |
| `saved_replies` | Answers a team sends often: name (unique per guild, ignoring case) and message |

Names (`opener_name`, `type_name`, …) are snapshots so history reads well after people or types change.

## Key flows

**Opening a ticket.** A panel button (`/panel/open/{typeID}`) or the dropdown (`/panel/select`) triggers `ticketbot.openTicket`. Both it and `formFor` run the same access check first (`ticketbot/access.go`): a server-wide block (`ticket_blocks`), the type's blocked and required roles, the member's open-ticket limit and the type's cooldown since their last ticket of it closed, in that order, so a blocked member learns nothing else. Staff block and unblock with `/ticket block` and `/ticket unblock` (anyone on any type's support team, or a manager) or from Settings. If the type has questions, `formFor` shows a modal (`/ticket-form/{source}/{typeID}/{version}`); its submit then calls `openTicket` with the answers. `version` fingerprints the questions, so answers to a form edited mid-fill are rejected rather than mismatched.
1. Lock the member, check the type exists, check their open-ticket limit, and pair any form answers with the questions.
2. Get the next number from `guild_settings.ticket_counter`.
3. Create the channel (with permission overwrites: deny @everyone, allow the opener, the support roles and the bot) or a private thread (and add the opener).
4. Insert the ticket row. If that fails, delete the channel.
5. Add the channel to `ticketCache`, then post the welcome embed with Claim/Close buttons. Form answers are embed fields; the welcome text is trimmed if needed to stay under Discord's 6,000-character embed limit.

**Transcripts.** `onMessageCreate/Update/Delete` only act on channels in `ticketCache`, which is loaded from open tickets at startup, so normal server traffic costs nothing. Authors are saved under their server nickname when they have one. The transcript response includes the guild's current role and channel names by ID (`mentionNames`), which the viewer uses to render `@Moderators` and `#billing` mentions; a deleted role or channel shows as such. Each captured message is marked `author_staff` if its author has one of the ticket type's support roles or manages the server (owner, Administrator or Manage Server, from the gateway cache); the opener, bots and people brought in with `/ticket add` are the member's side. The team's first message sets `first_response_at`, and only the team's messages end the wait clock; Analytics uses the same flag. `purgeTranscripts` runs hourly to apply retention.

Attachment links are Discord CDN URLs, which are signed and expire after about a day. When a transcript is viewed, `refreshAttachments` (`api/transcripts.go`) asks Discord's `POST /attachments/refresh-urls` for fresh links for any that expire within the hour, caches them until shortly before they expire again, and keeps the stored link if Discord won't renew it. Nothing is downloaded or stored.

**Closing a ticket.**
1. The Close button opens a reason modal; `/close` and `/ticket close` close directly.
2. `store.CloseTicket` is conditional, so it only closes once.
3. The close embed is posted.
4. `finishClose` runs:
   - removes the channel from the cache
   - DMs the opener a rating (plus a transcript link if `PUBLIC_URL` is https)
   - deletes the channel after 5s, or archives and locks a thread, and sets `channel_cleaned_at`. If that fails (a restart during the delay, or Manage Channels revoked), the 5-minute auto-close loop retries it for closed tickets older than 30s with `channel_cleaned_at` still NULL.

Deleting a ticket channel by hand also closes the ticket. So does the bot being removed from the server: `onGuildLeave` (and the startup reconcile) closes its open tickets with "The bot was removed from the server" and marks their channels as dealt with, since they're out of reach, and the auto-close, reminder and cleanup queries skip guilds with `left_at` set. When the bot rejoins, tickets opened before a restart whose channel has since gone are closed by the same check that runs at startup.

**Reopening.** Thread tickets can be reopened, since the thread is only archived: `reopenTicket` (`ticketbot/tickets.go`) unarchives and unlocks the thread, runs the conditional `store.ReopenTicket` (status back to open, close fields cleared, `reopened_at` set, waiting on the team), re-adds the thread to `ticketCache`, logs it and posts a "Ticket reopened" message with Claim and Close buttons. It's offered as a Reopen button on the close message in the thread (opener or staff), on the opener's closing DM, and on the Tickets page (`POST …/tickets/{id}/reopen`, via `Dashboard.Reopen`). A dashboard reopen happens in the API process, so the bot also listens for `ThreadUpdate` and re-caches a ticket thread it sees unarchived. Channel tickets can't be reopened because the channel is deleted. A member who types `/close` on their own ticket without a reason is asked to confirm first (`closeConfirmButtonID`); staff and reasoned closes go straight through.

Tickets can also be closed from the dashboard (`POST …/tickets/{id}/close`). The API uses `ticketbot.Dashboard`, which is the bot's close logic with a plain REST client in place of the gateway (`Bot.rest`), so the close message, log entry, DM and cleanup match a close in Discord. `Dashboard.Reply` (`POST …/tickets/{id}/reply`) posts a staff reply the same way: the bot sends an embed with the staff member's server nickname and avatar as its author and a footer saying the server's staff sent it from the dashboard (so no extra permissions are needed), saves it to the transcript straight away, and records the first response and activity itself, since the bot's message capture ignores bot messages. The bot drops a deleted channel from its `ticketCache` as usual. An archived thread stays cached until the bot restarts, which only matters if someone posts in the locked thread.

**Searching tickets.** `store.ListTickets` matches the number and names with `ILIKE`, and words with Postgres full-text search over `ticket_messages.search` using the `simple` configuration (no stemming, so order numbers, usernames and any language match as typed). `searchQuery` turns the input into "every word, prefix on each" (`refun:* & polic:*`), and deleted messages are skipped. For tickets found this way, `messageMatches` returns the newest matching message with a `ts_headline` snippet; the matching words are wrapped in private-use characters (`MatchStart`, `MatchEnd`) that the Tickets page turns into highlights.

**Setup check.** `GET …/setup-check` (`api/checks.go`) looks for problems that would stop tickets working, before a member hits them:
- permissions the bot lacks where a ticket type opens tickets, worked out from its roles and the channel's overwrites (`discordx.Permissions`)
- support roles the bot can't ping: roles that aren't mentionable when the bot lacks Mention Everyone there. In thread mode that ping is what adds staff to the thread, so it's reported as tickets not opening
- categories and channels that were deleted
- categories that are full or nearly full (Discord allows 50 channels per category; the bot also explains this when a ticket fails to open)
- published ticket buttons whose channel is gone or that have no ticket types
- ticket types that aren't on any published ticket buttons
- a log channel the bot can't post in

Home, the ticket types list and the ticket type editor show the problems, each linking to its fix.

**Joining a server.** When the bot is added it posts a short welcome in the server's system channel (if it has one and can post there), with a link to that server's dashboard.

**Ticket log.** When a guild has `log_channel_id` set, `ticketbot/log.go` posts an embed there when a ticket is opened, claimed or closed (including when its channel is deleted by hand). The close entry links to the transcript if `PUBLIC_URL` is https. Posting is best effort: failures are logged, never shown to members.

**Claim lock.** A ticket type's `claim_lock` decides what claiming a channel ticket does to the rest of the support team: nothing, read only (the support roles lose Send Messages) or hidden (they lose View Channel), except roles in `claim_lock_exempt_role_ids`. `applyClaimLock` (`ticketbot/claimlock.go`) edits the channel's permission overwrites on claim and gives the claimer their own overwrite; unclaiming restores them, and a move re-applies the lock against the new type's roles. Threads can't restrict a role, so the API refuses a lock on thread types.

**On hold.** `/ticket hold [reason]`, `/ticket resume` and the Tickets page set `tickets.on_hold` (`ticketbot/hold.go`). A held ticket keeps `status = open` but leaves the team's queue (Home, the sidebar badge, `waiting_now`), staff reminders and auto-close. The member's next message lifts the hold (`RecordActivity`), as does resuming, which restarts the wait clock.

**Reply targets and reminders.** `tickets.waiting_since` is when the team started owing a reply: set by the member's first unanswered message (or opening with form answers, or a reopen), kept across their follow-ups, cleared by a staff reply. A type's `reply_target_minutes` is the SLA: Analytics reports how many tickets got their first reply within it (`target_measured`, `target_met`, overall and per type) and how many open tickets are past it (`overdue_now`); Home and the Tickets page mark those overdue. `reminder_minutes` posts a reminder once a ticket has waited that long (`ticketbot/reminders.go`, in the auto-close loop): in the ticket, the log channel or both (`reminder_where`), mentioning the claimer or the support roles or nobody (`reminder_ping`), once or repeating every interval (`reminder_repeat`). `MarkReminded` is conditional, so a reply that lands first wins.

**Auto-close.** Every human message in a ticket calls `store.RecordActivity`, which bumps `last_activity_at`, clears any pending warning and sets `waiting_on_staff` (true if the opener wrote it). A ticket opened with form answers starts as waiting on staff. `ticketbot/autoclose.go` runs every 5 minutes:
1. Tickets whose type has `auto_close_hours`, that aren't waiting on staff, and that are within the warning lead (a quarter of the window, at most 24h) get a "Still need help?" message with "I still need help" and Close buttons. `auto_close_warned_at` is set first and cleared again if posting fails, so a ticket is never closed unwarned.
2. Warned tickets that have been inactive for the whole window, and warned for the full lead, are closed with `store.AutoCloseTicket` (conditional, so a last-moment reply wins). `closed_by` is NULL, and the usual close message, log entry, DM and cleanup follow via `finishClose`.

The SQL rules live in `store/autoclose.go`; the dashboard hint repeats the lead formula.

**Feedback.** The closing DM (`closedDM` in `ticketbot/tickets.go`) asks for a rating unless the ticket's type has `ask_rating` off, in which case it only says the ticket closed and links the transcript. A type can reword the request (`rating_prompt`, with `{staff}` for the claimer). The DM rating buttons (`/rate/{ticketID}/{n}`) save the rating and swap the buttons for "Add a comment" (a modal). Only the opener can rate. Each rating, and each comment, is also posted to the log channel (`ratedLog`). Analytics returns `asks_rating` per type so the dashboard can say "Not asked" instead of showing a blank.

**Publishing a panel** (shown as "ticket buttons" in the dashboard). `POST /api/guilds/{id}/panels/{panelID}/publish`:
- If the panel is already in that channel, edit the message in place.
- If it moved channels, post a new message and delete the old one.
- If the old message is gone (unknown message), post a new one.

Editing a ticket type re-renders every published panel that uses it. A custom emoji on a ticket type must be one of the server's own (checked against `GetEmojis` when the type is saved), since Discord only lets the bot put emoji from servers it's in on buttons; if Discord still rejects an emoji at publish time, the error says so.

**Rate limits** (`api/ratelimit.go`, in-process token buckets): the auth routes allow 10 requests a minute per IP, since each login writes an OAuth state to Redis; everything behind a login allows 240 a minute per user, mostly so one dashboard can't burn through the Discord rate limit the API shares with the bot. Over the limit is a 429 with `Retry-After`, shown by the dashboard as a toast. The client address comes from `TRUSTED_PROXIES`: with N proxies in front, it's the X-Forwarded-For entry N hops from the end; with none, the connection's address, and forwarded headers are ignored (chi's `RealIP`, which trusted them, is deprecated as spoofable). The buckets live in the API process, so running several API replicas would multiply the limits.

**Security headers.** Every API response carries `frame-ancestors 'none'`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy` and a `Permissions-Policy` (`securityHeaders` in `api/server.go`), plus HSTS when the request arrived over https. The SPA's full Content-Security-Policy is a `<meta>` tag that SvelteKit writes at build time with the hash of its bootstrap script (`csp` in `web/vite.config.ts`): scripts, styles, fonts and API calls are same-origin only; images may come from any https host because Discord embeds can link anywhere.

**Dashboard auth.**
1. `/api/auth/login` goes to Discord OAuth (`identify guilds`). `?next=` (a local path) is kept in Redis next to the OAuth state, so someone opening a transcript link from a DM lands back on it after logging in. The frontend adds it on any 401.
2. The callback stores the session in Redis, sets an HttpOnly cookie and redirects to `next`, or to `/servers` if there isn't one. The Discord token is refreshed lazily, when the guild list is needed and its cache is cold: one request takes a short Redis lock (`oauth_refresh:<session>`) and refreshes, the rest wait for it and re-read the session, since Discord accepts each refresh token once. Only a failed refresh ends the session (a 401, which sends the dashboard back to login).
3. Access to a guild needs the bot in the guild, plus either Owner, Administrator or Manage Server (from the OAuth guild list), or one of the guild's `dashboard_roles` (checked via bot-token `GetMember`, cached 30s). `internal/api/access.go` has the logic.
4. Every user has a level: managers are `owner`; a dashboard role grants `viewer` (read everything), `support` (also act on tickets: reply, close, reopen, move, hold, block members) or `admin` (also change ticket types, buttons, saved replies and settings). The highest of a member's roles wins. `requireLevel` in `api/server.go` enforces the minimum per route group; only owners can change the dashboard roles (`validateSettings`). The guild response carries `level` (and `can_manage` for owners) so the frontend hides what the user can't do.
5. Transcripts (`/api/transcripts/{id}`) are visible to anyone with dashboard access, the ticket's opener, and that type's support roles. Anyone else gets a 404, so ticket IDs can't be probed.

## API surface (`internal/api/server.go`)

Public: `/healthz`, `/api/config`, `/api/invite`, `/api/auth/{login,callback,logout}`

Authenticated: `/api/me`, `/api/guilds`, `/api/transcripts/{ticketID}`, and under `/api/guilds/{guildID}`:
- `channels`, `roles`, `stats`, `setup-check`, `analytics?days=7|30|90|365|all&type={ticketTypeID}`, `tickets?status=open|closed&type={ticketTypeID}&q={search}&before={ticketID}&limit=` (newest first, at most 200 per page; `before` pages past a ticket; `q` matches the number, names and words in the ticket's messages, and a ticket found through its messages carries a `match` with a snippet), `tickets/{id}/close`, `tickets/{id}/reply`, `tickets/{id}/move`, `tickets/{id}/reopen`, `tickets/{id}/hold`, `tickets/{id}/resume` (all POST, support level), `settings` (GET/PATCH; PATCH is partial, so omitted fields are kept)
- `ticket-types` (GET/POST), `ticket-types/{id}` (PATCH/DELETE)
- `panels` (GET/POST), `panels/{id}` (PATCH/DELETE), `panels/{id}/publish` (POST)
- `saved-replies` (GET/POST), `saved-replies/{id}` (PATCH/DELETE)

## Deployment

Railway: two services from one repo (`Dockerfile.bot`, `Dockerfile.api`) plus Postgres and Redis. The API image builds the SPA and serves it from `STATIC_DIR`. See `docs/development.md` for environment variables.
