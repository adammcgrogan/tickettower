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
| `guild_settings` | Per-guild settings: `ticket_counter` (sequential ticket numbers), `transcript_retention_days`, `dashboard_role_ids` (extra roles allowed into the dashboard) |
| `entitlements` | Plan tier per guild (absent means free) → `internal/entitlements.ForTier` |
| `ticket_types` | Name, emoji, `mode` (channel/thread), `parent_id` (category or channel), support roles, name format, welcome message, per-member limit |
| `panels` + `panel_ticket_types` | Panel content/style, where it's published (`channel_id`, `message_id`), and its ordered ticket types |
| `tickets` | One per ticket: number, type snapshot (`type_name`), channel, opener/claimer/closer with name snapshots, status, timestamps incl. `first_response_at` |
| `ticket_messages` | Transcript messages (content, embeds/attachments as JSONB, edit/delete flags) |
| `ticket_feedback` | 1–5 rating + comment per ticket |

Names (`opener_name`, `type_name`, …) are snapshots so history reads well after people or types change.

## Key flows

**Opening a ticket.** A panel button (`/panel/open/{typeID}`) or the dropdown (`/panel/select`) triggers `ticketbot.openTicket`:
1. Lock the member, check the type exists, and check their open-ticket limit.
2. Get the next number from `guild_settings.ticket_counter`.
3. Create the channel (with permission overwrites: deny @everyone, allow the opener, the support roles and the bot) or a private thread (and add the opener).
4. Insert the ticket row. If that fails, delete the channel.
5. Add the channel to `ticketCache`, then post the welcome embed with Claim/Close buttons.

**Transcripts.** `onMessageCreate/Update/Delete` only act on channels in `ticketCache`, which is loaded from open tickets at startup, so normal server traffic costs nothing. The first non-bot message from someone other than the opener sets `first_response_at`. `purgeTranscripts` runs hourly to apply retention.

**Closing a ticket.**
1. The Close button opens a reason modal; `/close` and `/ticket close` close directly.
2. `store.CloseTicket` is conditional, so it only closes once.
3. The close embed is posted.
4. `finishClose` runs:
   - removes the channel from the cache
   - DMs the opener a rating (plus a transcript link if `PUBLIC_URL` is https)
   - deletes the channel after 5s, or archives and locks a thread

Deleting a ticket channel by hand also closes the ticket.

**Feedback.** The DM rating buttons (`/rate/{ticketID}/{n}`) save the rating and swap the buttons for "Add a comment" (a modal). Only the opener can rate.

**Publishing a panel.** `POST /api/guilds/{id}/panels/{panelID}/publish`:
- If the panel is already in that channel, edit the message in place.
- If it moved channels, post a new message and delete the old one.
- If the old message is gone (unknown message), post a new one.

Editing a ticket type re-renders every published panel that uses it.

**Dashboard auth.**
1. `/api/auth/login` goes to Discord OAuth (`identify guilds`).
2. The callback stores the session in Redis and sets an HttpOnly cookie.
3. Access to a guild needs the bot in the guild, plus either Owner, Administrator or Manage Server (from the OAuth guild list), or one of the guild's `dashboard_role_ids` (checked via bot-token `GetMember`, cached 30s). `internal/api/access.go` has the logic.
4. Dashboard-role members can do everything except change the dashboard roles themselves; `can_manage` on the guild response tells the frontend which kind of user it has.
5. Transcripts (`/api/transcripts/{id}`) are visible to anyone with dashboard access, the ticket's opener, and that type's support roles. Anyone else gets a 404, so ticket IDs can't be probed.

## API surface (`internal/api/server.go`)

Public: `/healthz`, `/api/config`, `/api/invite`, `/api/auth/{login,callback,logout}`

Authenticated: `/api/me`, `/api/guilds`, `/api/transcripts/{ticketID}`, and under `/api/guilds/{guildID}`:
- `channels`, `roles`, `stats`, `analytics?days=7|30|90`, `tickets?status=`, `settings` (GET/PATCH; PATCH is partial, so omitted fields are kept)
- `ticket-types` (GET/POST), `ticket-types/{id}` (PATCH/DELETE)
- `panels` (GET/POST), `panels/{id}` (PATCH/DELETE), `panels/{id}/publish` (POST)

## Deployment

Railway: two services from one repo (`Dockerfile.bot`, `Dockerfile.api`) plus Postgres and Redis. The API image builds the SPA and serves it from `STATIC_DIR`. See the README for environment variables.
