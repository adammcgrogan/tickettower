# Development and deployment

Maintainer notes. Ticket Tower is a hosted bot, so this is for working on it,
not for running your own copy.

## Local development

Requirements: Go 1.26+, Node 22+, Postgres 16+, Redis 7+.

1. Use a separate Discord application for development (not the production one),
   from <https://discord.com/developers/applications>:
   - **Bot** tab: copy the token and enable the **Message Content** privileged
     intent (used for transcripts). The bot will fail to connect without it.
   - **OAuth2** tab: copy the client ID/secret and add the redirect
     `http://localhost:5173/api/auth/callback`.
2. `cp .env.example .env` and fill it in. Set `DEV_GUILD_ID` to your test server
   so slash commands update instantly.
3. In separate terminals:
   ```sh
   make bot   # Discord bot
   make api   # API on $PORT
   make web   # dashboard on http://localhost:5173
   ```

Tests: `TEST_DATABASE_URL=postgres://localhost:5432/relay_test?sslmode=disable make test`
(store tests are skipped without it).

## Production (Railway)

One project, `tickettower`, with **Postgres**, **Redis** and two services built
from this repo:

| Service | Dockerfile       | Notes                                          |
|---------|------------------|------------------------------------------------|
| `bot`   | `Dockerfile.bot` | No public networking                           |
| `api`   | `Dockerfile.api` | Custom domain `tickettower.net`, port 8080     |

Variables on both: `DATABASE_URL` (reference to Postgres), `DISCORD_TOKEN`,
`DISCORD_CLIENT_ID`, `PUBLIC_URL=https://tickettower.net`, `APP_NAME`, and
`RAILWAY_DOCKERFILE_PATH`. On the API also: `REDIS_URL`, `DISCORD_CLIENT_SECRET`,
`PORT=8080`. `DEV_GUILD_ID` stays unset so commands register globally.

The production Discord application has `https://tickettower.net/api/auth/callback`
as its OAuth2 redirect.

Deploy by hand with `railway up -s api` and `railway up -s bot`, or connect the
services to the GitHub repo to deploy on every push to `main`.
