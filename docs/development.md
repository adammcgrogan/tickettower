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

Dependencies: `go.mod` pins the exact Go patch release (`go 1.26.x`), which the
Dockerfiles and CI use too, so standard-library fixes reach production. CI fails
on vulnerabilities reachable from the code (`govulncheck`) and on high-severity
npm advisories (`npm audit`); Dependabot opens weekly PRs for Go, npm, Docker
and Actions updates. When Dependabot bumps the toolchain, bump the
`golang:<version>-alpine` tag in both Dockerfiles to match.

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
`PORT=8080`, `TRUSTED_PROXIES=1` (Railway's edge proxy, so the per-IP login
limit sees real client addresses), `SUPERADMIN_USER_ID` (your Discord user
ID; unlocks the `/admin` install/usage panel for you only) and, for premium
billing, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` (from the webhook
endpoint's settings in the Stripe dashboard, pointed at
`https://tickettower.net/stripe/webhook`) and `STRIPE_PRICE_ID` (the premium
plan's price). All three are optional — leaving them unset just hides the
dashboard's Plan section. `DEV_GUILD_ID` stays unset so commands register
globally.

The production Discord application has `https://tickettower.net/api/auth/callback`
as its OAuth2 redirect.

Both services deploy from the GitHub repo on every push to `main`, once CI
passes ("Wait for CI"). Root Directory is empty, since the Dockerfiles build
from the repo root. `bot` has watch paths (`/cmd/bot/**`, `/internal/**`,
`go.mod`, `go.sum`, `Dockerfile.bot`) so dashboard-only changes don't restart
it; `api` redeploys on any change. `railway up -s api` / `railway up -s bot`
still deploy the local checkout by hand.
