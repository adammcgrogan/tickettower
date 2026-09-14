# Ticket Tower: Discord ticket bot + web dashboard

Open-source Discord ticket bot (a simpler, better-looking alternative to Ticket Tool / tickets.bot). The owner hosts it on Railway so anyone can add it. The bar: **professional, easy to use, sleek and modern**. Fewer options done well beats many confusing ones.

Named "Ticket Tower" (tickettower.net), configurable via `APP_NAME` (Go) and `VITE_APP_NAME` (web). `VITE_SUPPORT_URL` sets the "Get support" link (defaults to GitHub issues).

More detail: `docs/architecture.md` (how it fits together), `docs/development.md` (local setup and the Railway deployment) and `docs/roadmap.md` (what's done and what's next).

## Stack

- **Go 1.26**, one module, two binaries: `cmd/bot` (Discord gateway, disgo v0.19) and `cmd/api` (REST + OAuth, serves the SPA)
- **Postgres** via pgx with **hand-written SQL** (no ORM, no sqlc). Migrations are goose files embedded in `internal/store/migrations/`, run automatically on startup with an advisory lock.
- **Redis**: dashboard sessions, OAuth state, a 60s cache of each user's guild list
- **SvelteKit** (Svelte 5 runes, TypeScript) built as a static SPA (`adapter-static`, `ssr = false`), plus Tailwind v4

## Commands

```sh
make bot | make api | make web      # run locally (all read .env)
TEST_DATABASE_URL=postgres://localhost:5432/relay_test?sslmode=disable go test -p 1 ./...
go vet ./...
cd web && npm run check && npm run build
```

- Local Postgres 16 and Redis are Homebrew services. Databases: `relay` (dev) and `relay_test` (tests). Store tests are skipped without `TEST_DATABASE_URL`.
- Locally the API runs on **PORT=8081**, because something else on the machine uses 8080. `make web` passes `PORT` to the Vite `/api` proxy.
- The system Node is 20.17 but Vite 8 needs ≥20.19 or ≥22.12. If `npm` complains, run it via `npx -y -p node@22 -- npm …`.
- Never commit `.env`: it holds real Discord credentials.

## Layout

```
internal/config       env config, BotPermissions
internal/store        all SQL. One file per area (guilds, settings, ticket_types, panels, tickets, transcripts, analytics,
                      saved_replies, notes)
internal/ticketbot    bot: bot.go (wiring/events), tickets.go (ticket logic), commands.go (slash/buttons),
                      transcripts.go (message capture, retention), feedback.go (ratings), log.go (log channel),
                      forms.go (pre-ticket question modals), autoclose.go (inactivity warnings and closing),
                      replies.go (/reply, staff reply placeholders), dashboard.go (closing/replying from the API)
internal/api          HTTP handlers; server.go has all routes, access.go decides who can use a guild's dashboard
internal/auth         Discord OAuth2 + Redis sessions
internal/panels       renders panel messages (shared by API publish + bot interaction IDs)
internal/discordx     Discord error codes → friendly messages
internal/entitlements per-tier limits (premium-ready; everything is free today)
web/src/lib           api.ts (types + fetch wrapper), components/, format.ts, markdown.ts, toast.svelte.ts,
                      help.ts (the help guide list; link to one with Field's help="slug")
web/src/routes        / (landing), /servers, /servers/[id]/{ticket-types,buttons,replies,tickets,analytics,settings},
                      /transcripts/[ticketId], /privacy, /help and /help/[slug] (public guides)
```

## Conventions

**Go**
- Discord IDs are `snowflake.ID` in Go and `BIGINT` in SQL. Convert explicitly (`int64(id)` in, `snowflake.ID(v)` out); use the helpers in `store/helpers.go` (`nullableID`, `idFromNullable`, `toInt64s`). Don't rely on pgx's reflection for snowflakes.
- Every store query is scoped by `guild_id`, so cross-guild access returns `store.ErrNotFound`.
- Bot errors shown to users: return `userErr("…")`. `b.describe(err)` turns any error into a message (user errors pass through, known Discord errors get friendly text, anything else is logged and shown as a generic message).
- API validation: return `invalid("field", "Human message.")`. `s.writeFailure` sends a 422 with `{error, field}`, which the frontend shows under that form field.
- All state-changing API requests must be JSON (`requireJSON` middleware). Together with SameSite=Lax cookies, that's the CSRF protection.
- Handlers use `r.Context()`; bot handlers wrap `e.Ctx` in a timeout.
- Race-prone updates are conditional SQL that reports whether it applied (`ClaimTicket`, `CloseTicket`). Keep it that way.
- Tests: store tests run against real Postgres (`testStore(t)` truncates). The API tests use miniredis, except billing tests, which also hit real Postgres directly. Because `internal/store` and `internal/api` share one `TEST_DATABASE_URL` instance, always run `go test` with `-p 1` so their package binaries don't truncate/insert concurrently. Add tests with new store queries.

**Naming**
- What the code calls a **panel** (the message with buttons or a dropdown that members click to open a ticket) is called **ticket buttons** everywhere people read it: the dashboard, bot replies and API error messages. The code, API routes (`/api/guilds/{id}/panels`), store and tables keep `panel`; the dashboard URL is `/servers/[id]/buttons` (old `/panels` links redirect).

**Frontend**
- Design ("dispatch desk"): ink-navy surfaces and one signal-amber accent `#f2b544` (text on it uses `on-accent`). Amber means "needs action": primary buttons and tickets waiting on the team, nothing decorative. Instrument Sans for the UI; Big Shoulders Display (`font-display`) only for page titles, big figures and ticket numbers. The one loud element is `TicketStub` (a ticket number shaped like an admission stub); keep everything around it quiet: hairline dividers rather than stacks of cards, sentence-case labels, no all-caps eyebrows, no "A · B" meta strings.
- Use the tokens in `app.css` (`bg`, `surface`, `elevated`, `border`, `fg`, `muted`, `subtle`, `accent`, `on-accent`, `chart`, `success`, `danger`) and the component classes `.input .btn .btn-primary|secondary|ghost|danger .card .label .hint .stub`. Don't introduce new colours. The bot's embed colour (`colorAccent` in `ticketbot/tickets.go`) matches the accent.
- Layout: `servers/[id]/+layout.svelte` is the app shell (sidebar with server switcher; Home, Tickets, Analytics, then a Setup group). Form pages stay within `max-w-3xl`; editors put the form left and a Discord-style preview right.
- Reuse the components: `PageHeader`, `Field`, `Dialog`, `Segmented`, `ChannelSelect`, `RolePicker`, `Toaster` (`toast()`), `Icon` (add paths to its map), `TicketStub`, `TicketSummary`, `TranscriptView`, `PanelPreview`, `WelcomePreview`, `SetupProblems` (results of `GET …/setup-check`, each with a Fix link). `ticketState()` in `format.ts` decides the "waiting on your team / the member" wording.
- Every page handles loading (skeleton), empty (dashed box + CTA) and error states. Copy is plain and friendly, with no jargon.
- Call the API with `api<T>(path, send('POST', body))`. A 401 redirects to login automatically.
- Use `{@html}` only with `renderMarkdown` output, which escapes everything first (it uses `\u0000` placeholders; keep them as escape text, never raw NUL bytes).
- Charts: a single series in the `chart` colour (`#c28622`, the accent stepped into the dataviz validator's dark lightness band), with no legend, hover and focus tooltips, and a "Show as table" view. Validate any new palette with the dataviz skill's validator before using it.
- Keep `npm run check` at **0 errors and 0 warnings**.

## Gotchas

- disgo's **debug logs include the bot token**. Debug logging is opt-in (`LOG_LEVEL=debug`), so don't make it the default.
- **Discord rejects link buttons to non-https URLs.** Transcript links are only added when `PUBLIC_URL` is https.
- The Message Content intent is privileged. The bot won't connect without it enabled in the Developer Portal, and past 100 servers it needs verification (transcripts are the justification). Keep `/privacy` accurate if data handling changes.
- **Single shard assumed**: `onGuildsReady` reconciliation, the in-memory `ticketCache` and the `openLocks` all assume one bot process.
- **Slash commands:** with `DEV_GUILD_ID` set they register to that guild instantly; without it they register globally.
- The API calls Discord with the bot token (publishing panels, listing channels/roles, `GetMember`), so both services need `DISCORD_TOKEN`.
- Discord's own limits: 25 buttons per message (5×5), 25 select options, 500 channels per server, 2 channel renames per 10 minutes, 5 inputs per modal (45-character labels), 6,000 characters across a message's embeds. Form limits live in `store` (`MaxQuestions` etc.) and are sized so five answers plus the welcome message fit one embed.
- A modal must be the first response to an interaction, within 3 seconds, so the form lookup in `formFor` runs before anything is deferred.
