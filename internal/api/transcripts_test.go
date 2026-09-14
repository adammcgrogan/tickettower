package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/disgoorg/disgo/oauth2"
	"github.com/disgoorg/snowflake/v2"
	"github.com/redis/go-redis/v9"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/config"
	"github.com/adammcgrogan/tickettower/internal/store"
)

// newAuthedStoreTestEnv is newTestEnv with a real store wired in, for routes
// that need one, and a login helper for a session cookie without going
// through Discord OAuth.
func newAuthedStoreTestEnv(t *testing.T, st *store.Store) testEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	root := t.TempDir()
	static := filepath.Join(root, "build")
	writeFile(t, filepath.Join(static, "index.html"), "<!doctype html>spa")

	cfg := config.Config{
		AppName:         "Ticket Tower",
		DiscordClientID: 123456789,
		PublicURL:       "http://localhost:5173",
		StaticDir:       static,
	}
	am := auth.NewManager(cfg.DiscordClientID, "secret", cfg.PublicURL, rdb)
	srv := NewServer(cfg, st, am, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return testEnv{handler: srv.Handler(), redis: mr}
}

// login stores a session directly in redis, as if the user had just finished
// the Discord OAuth flow, and returns the cookie for it.
func (e testEnv) login(t *testing.T, user auth.User) *http.Cookie {
	t.Helper()
	sess := auth.Session{User: user, OAuth: oauth2.Session{AccessToken: "token", Expiration: time.Now().Add(time.Hour)}}
	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatal(err)
	}
	token := "test-" + user.ID.String()
	e.redis.Set("session:"+token, string(data))
	return &http.Cookie{Name: "session", Value: token}
}

func signedURL(name string, expires time.Time) string {
	return fmt.Sprintf("https://cdn.discordapp.com/attachments/1/2/%s?ex=%x&is=0&hm=sig", name, expires.Unix())
}

// TestGetMyTicketsListsAcrossGuilds needs a real store (the query it exercises
// isn't guild-scoped) and a real session, so it's built like the billing
// tests: real Postgres, plus miniredis for the session cookie.
func TestGetMyTicketsListsAcrossGuilds(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	mine, other := snowflake.ID(90_100_000_000_000_001), snowflake.ID(90_100_000_000_000_002)
	var ticketIDs []int64
	t.Cleanup(func() {
		// DeleteGuildData refuses while a ticket is open, so close them first.
		for _, id := range ticketIDs {
			st.CloseTicket(ctx, id, 1, "test", "") //nolint:errcheck
		}
		st.DeleteGuildData(ctx, mine)  //nolint:errcheck
		st.DeleteGuildData(ctx, other) //nolint:errcheck
	})
	for _, g := range []snowflake.ID{mine, other} {
		if err := st.UpsertGuild(ctx, store.Guild{ID: g, Name: "Server " + g.String(), OwnerID: 1}); err != nil {
			t.Fatal(err)
		}
	}
	tt1 := store.TicketType{GuildID: mine, Name: "General", Mode: store.ModeChannel}
	if err := st.CreateTicketType(ctx, &tt1); err != nil {
		t.Fatal(err)
	}
	tt2 := store.TicketType{GuildID: other, Name: "General", Mode: store.ModeChannel}
	if err := st.CreateTicketType(ctx, &tt2); err != nil {
		t.Fatal(err)
	}

	// Distinctive user IDs, unlikely to collide with hardcoded openers in
	// internal/store's own fixtures sharing this database.
	const me, someoneElse = snowflake.ID(90_100_900_000_000_001), snowflake.ID(90_100_900_000_000_002)
	mk := func(guildID snowflake.ID, tt store.TicketType, channelID int64, opener snowflake.ID) {
		t.Helper()
		number, err := st.NextTicketNumber(ctx, guildID)
		if err != nil {
			t.Fatal(err)
		}
		tk := store.Ticket{GuildID: guildID, Number: number, TicketTypeID: &tt.ID, TypeName: tt.Name, Mode: store.ModeChannel,
			ChannelID: snowflake.ID(channelID), OpenerID: opener, OpenerName: "member"}
		if err := st.CreateTicket(ctx, &tk); err != nil {
			t.Fatal(err)
		}
		ticketIDs = append(ticketIDs, tk.ID)
	}
	mk(mine, tt1, 90_100_000_000_100_001, me)
	mk(other, tt2, 90_100_000_000_100_002, me)
	mk(mine, tt1, 90_100_000_000_100_003, someoneElse)

	env := newAuthedStoreTestEnv(t, st)
	cookie := env.login(t, auth.User{ID: me, Username: "adam"})

	rec := env.do("GET", "/api/me/tickets", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Tickets []struct {
			Ticket store.Ticket  `json:"ticket"`
			Guild  myTicketGuild `json:"guild"`
		} `json:"tickets"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Tickets) != 2 {
		t.Fatalf("got %d tickets, want 2", len(resp.Tickets))
	}
	for _, entry := range resp.Tickets {
		if entry.Ticket.OpenerID != me {
			t.Errorf("got ticket opened by %v, want %v", entry.Ticket.OpenerID, me)
		}
		if entry.Guild.Name == "" || entry.Guild.Name == "Unknown server" {
			t.Errorf("guild name not resolved: %+v", entry.Guild)
		}
	}
}

func TestRenewAttachmentLinks(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	expired := signedURL("old.png", now.Add(-2*time.Hour))
	soon := signedURL("soon.png", now.Add(10*time.Minute))
	fine := signedURL("fine.png", now.Add(6*time.Hour))
	unsigned := "https://cdn.discordapp.com/attachments/1/2/plain.png"
	messages := []store.TicketMessage{
		{Attachments: []store.Attachment{{URL: expired}, {URL: fine}}},
		{Attachments: []store.Attachment{{URL: soon}, {URL: unsigned}, {URL: expired}}},
	}

	var asked [][]string
	newExpiry := now.Add(24 * time.Hour)
	refresh := func(urls []string) (map[string]string, error) {
		asked = append(asked, slices.Clone(urls))
		return map[string]string{expired: signedURL("old.png", newExpiry)}, nil // Discord didn't renew soon.png
	}
	cache := newTTLCache()
	if err := renewAttachmentLinks(messages, now, cache, refresh); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || !slices.Equal(asked[0], []string{expired, soon}) {
		t.Errorf("asked to refresh %v, want [expired soon] once", asked)
	}
	if got := messages[0].Attachments[0].URL; got != signedURL("old.png", newExpiry) {
		t.Errorf("expired link = %s, want renewed", got)
	}
	if messages[1].Attachments[2].URL != messages[0].Attachments[0].URL {
		t.Error("the same link in another message wasn't renewed")
	}
	for _, want := range []string{fine, soon, unsigned} {
		found := slices.ContainsFunc(messages, func(m store.TicketMessage) bool {
			return slices.ContainsFunc(m.Attachments, func(a store.Attachment) bool { return a.URL == want })
		})
		if !found {
			t.Errorf("link %s should be untouched", want)
		}
	}

	// A second view uses the cache instead of asking Discord again.
	messages[0].Attachments[0].URL = expired
	asked = nil
	refresh = func(urls []string) (map[string]string, error) {
		asked = append(asked, urls)
		return nil, errors.New("discord down")
	}
	err := renewAttachmentLinks(messages[:1], now, cache, refresh)
	if err != nil || len(asked) != 0 || messages[0].Attachments[0].URL != signedURL("old.png", newExpiry) {
		t.Errorf("cached renewal: err=%v asked=%v url=%s", err, asked, messages[0].Attachments[0].URL)
	}

	// A failed refresh keeps the stored links and reports the error.
	messages[1].Attachments[0].URL = soon
	if err := renewAttachmentLinks(messages[1:], now, cache, refresh); err == nil {
		t.Error("refresh failure not reported")
	}
	if messages[1].Attachments[0].URL != soon {
		t.Error("link changed despite a failed refresh")
	}
}

func TestRenderTranscriptMarkdown(t *testing.T) {
	users := map[string]string{"1": "Ada"}
	roles := map[string]string{"2": "Moderators"}
	channels := map[string]string{"3": "billing"}
	cases := map[string]string{
		"<script>alert(1)</script>": "&lt;script&gt;alert(1)&lt;/script&gt;",
		"**bold** and _em_":         "<strong>bold</strong> and <em>em</em>",
		"<@1> ping":                 `<span class="md-mention">@Ada</span> ping`,
		"<@&2> ping":                `<span class="md-mention">@Moderators</span> ping`,
		"<#3> channel":              `<span class="md-mention">#billing</span>`,
		"<@999> ghost":              `unknown-user`,
		"`inline`":                  `<code class="md-code">inline</code>`,
		"see https://example.com":   `<a href="https://example.com" target="_blank" rel="noopener noreferrer nofollow" class="md-link">https://example.com</a>`,
	}
	for input, want := range cases {
		got := renderTranscriptMarkdown(input, users, roles, channels)
		if !strings.Contains(got, want) {
			t.Errorf("renderTranscriptMarkdown(%q) = %q, want to contain %q", input, got, want)
		}
	}
}

func TestRenderTranscriptHTML(t *testing.T) {
	opened := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)
	data := transcriptData{
		Ticket: store.Ticket{
			Number:     7,
			TypeName:   "Support",
			OpenerID:   snowflake.ID(1),
			OpenerName: "Ada",
			Status:     store.StatusOpen,
			OpenedAt:   opened,
		},
		Messages: []store.TicketMessage{
			{
				ID: snowflake.ID(1), AuthorID: snowflake.ID(1), AuthorName: "Ada",
				Content: "Hello <script>evil()</script> **team**", CreatedAt: opened,
				Attachments: []store.Attachment{{Name: "log.txt", URL: "https://cdn.discordapp.com/x.txt", Size: 2048}},
			},
		},
		Guild: transcriptGuild{Name: "Acme Support"},
	}

	var buf bytes.Buffer
	if err := renderTranscriptHTML(&buf, data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"Ticket #7", "Acme Support", "Ada",
		"<strong>team</strong>",
		"log.txt", "2 KB",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "<script>evil()</script>") {
		t.Error("message content wasn't escaped")
	}
}
