# Ticket Tower

A modern, open-source Discord ticket bot with a clean web dashboard. Built in Go and SvelteKit.

## Features

- **Ticket buttons** (called panels in the code): messages with buttons or a dropdown menu, designed in the dashboard with a live preview
- **Ticket types**: each opens a private channel *or* a private thread, with its own support roles, naming, welcome message and per-member limit
- **Ticket actions**: claim, close with a reason, add/remove members, rename (buttons and slash commands)
- **Transcripts**: every ticket message is saved and viewable in a Discord-style viewer, with configurable retention
- **Feedback**: members rate their ticket 1–5 (plus an optional comment) after it closes
- **Analytics**: volume over time, median first response and resolution times, satisfaction, per-type and per-staff breakdowns

## Architecture

```
cmd/bot   Discord gateway process (disgo)
cmd/api   REST API + Discord OAuth2, serves the built dashboard
web/      SvelteKit dashboard (static SPA)
internal/ shared packages: config, store (Postgres), auth, api, ticketbot, panels
```

Both processes share Postgres; migrations are embedded and run automatically on
startup. Redis holds dashboard sessions. The API uses the bot token to publish
panels and read channels/roles, so both processes need `DISCORD_TOKEN`.

## Local development

Requirements: Go 1.26+, Node 22+, Postgres 16+, Redis 7+.

1. Create an application at <https://discord.com/developers/applications>:
   - **Bot** tab: copy the token and enable the **Message Content** privileged
     intent (used for transcripts). The bot will fail to connect without it.
   - **OAuth2** tab: copy the client ID/secret and add the redirect
     `http://localhost:5173/api/auth/callback`.
2. `cp .env.example .env` and fill it in. Set `DEV_GUILD_ID` to your test server
   so slash commands update instantly.
3. In separate terminals:
   ```sh
   make bot   # Discord bot
   make api   # API on :8080
   make web   # dashboard on http://localhost:5173
   ```

Tests: `TEST_DATABASE_URL=postgres://localhost:5432/relay_test?sslmode=disable make test`
(store tests are skipped without it).

## Deploying to Railway

Create one project with **Postgres** and **Redis**, then two services from this repo:

| Service | Dockerfile        | Notes                                                           |
|---------|-------------------|-----------------------------------------------------------------|
| `bot`   | `Dockerfile.bot`  | No public networking needed                                     |
| `api`   | `Dockerfile.api`  | Add the custom domain `tickettower.net`; set `PUBLIC_URL` to it |

Set on both: `DATABASE_URL` (Railway reference to Postgres), `DISCORD_TOKEN`,
`DISCORD_CLIENT_ID`, `PUBLIC_URL`. On the API also: `REDIS_URL`,
`DISCORD_CLIENT_SECRET`. Leave `DEV_GUILD_ID` unset in production so commands
register globally. Add `$PUBLIC_URL/api/auth/callback` as an OAuth2 redirect.

With an `https://` `PUBLIC_URL`, closing DMs include a "View transcript" button.

## License

MIT
