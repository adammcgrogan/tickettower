package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/disgoorg/disgo/oauth2"
	"github.com/redis/go-redis/v9"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/config"
)

type testEnv struct {
	handler http.Handler
	redis   *miniredis.Miniredis
}

func newTestEnv(t *testing.T, opts ...func(*config.Config)) testEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	root := t.TempDir()
	static := filepath.Join(root, "build")
	writeFile(t, filepath.Join(static, "200.html"), "<!doctype html>spa")
	writeFile(t, filepath.Join(static, "index.html"), "<!doctype html>landing")
	writeFile(t, filepath.Join(static, "help.html"), "<!doctype html>help index")
	writeFile(t, filepath.Join(static, "help", "getting-started.html"), "<!doctype html>getting started")
	writeFile(t, filepath.Join(static, "_app", "immutable", "app.js"), "console.log(1)")
	writeFile(t, filepath.Join(root, "secret.txt"), "secret")

	cfg := config.Config{
		AppName:         "Ticket Tower",
		DiscordClientID: 123456789,
		PublicURL:       "http://localhost:5173",
		StaticDir:       static,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	am := auth.NewManager(cfg.DiscordClientID, "secret", cfg.PublicURL, rdb)
	// The store and Discord client are nil: none of the routes exercised here
	// touch them.
	srv := NewServer(cfg, nil, am, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return testEnv{handler: srv.Handler(), redis: mr}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e testEnv) do(method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e testEnv) doWithAuth(path, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	rec := newTestEnv(t).do("GET", "/healthz", nil)
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestSPA(t *testing.T) {
	env := newTestEnv(t)

	tests := []struct {
		name, path, wantBody string
	}{
		{"client route falls back to the shell", "/servers/123", "spa"},
		{"root serves the prerendered landing page", "/", "landing"},
		{"prerendered page", "/help", "help index"},
		{"prerendered page with a trailing slash", "/help/", "help index"},
		{"nested prerendered page", "/help/getting-started", "getting started"},
		{"static asset", "/_app/immutable/app.js", "console.log(1)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := env.do("GET", tt.path, nil)
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("got %d %q, want body containing %q", rec.Code, rec.Body.String(), tt.wantBody)
			}
		})
	}

	// net/http rejects ".." paths outright; what matters is nothing outside
	// the static dir is ever served.
	for _, path := range []string{"/../secret.txt", "/_app/../../secret.txt"} {
		if rec := env.do("GET", path, nil); strings.Contains(rec.Body.String(), "secret") {
			t.Errorf("%s leaked a file outside the static dir", path)
		}
	}

	// A missing build asset is a 404, not the HTML shell parsed as JavaScript.
	if rec := env.do("GET", "/_app/immutable/gone.js", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing asset = %d, want 404", rec.Code)
	}

	rec := env.do("GET", "/_app/immutable/app.js", nil)
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("immutable asset Cache-Control = %q", cc)
	}
	// The shell must be revalidated every time, or a deploy strands browsers
	// on an index.html that names assets that are gone.
	for _, path := range []string{"/", "/help", "/servers/123"} {
		if cc := env.do("GET", path, nil).Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s Cache-Control = %q, want no-cache", path, cc)
		}
	}
}

func TestUnknownAPIRouteIsJSON404(t *testing.T) {
	rec := newTestEnv(t).do("GET", "/api/nope", nil)
	if rec.Code != 404 || !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("got %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestAuthRequired(t *testing.T) {
	env := newTestEnv(t)
	for _, c := range []*http.Cookie{nil, {Name: "session", Value: "bogus"}} {
		for _, path := range []string{"/api/me", "/api/guilds", "/api/guilds/1"} {
			if rec := env.do("GET", path, c); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s with cookie %v: got %d, want 401", path, c, rec.Code)
			}
		}
	}
}

func TestMeWithValidSession(t *testing.T) {
	env := newTestEnv(t)
	sess := auth.Session{
		User:  auth.User{ID: 42, Username: "adam", DisplayName: "Adam"},
		OAuth: oauth2.Session{AccessToken: "token", Expiration: time.Now().Add(time.Hour)},
	}
	data, _ := json.Marshal(sess)
	env.redis.Set("session:valid", string(data))

	rec := env.do("GET", "/api/me", &http.Cookie{Name: "session", Value: "valid"})
	if rec.Code != 200 {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var user auth.User
	if err := json.NewDecoder(rec.Body).Decode(&user); err != nil {
		t.Fatal(err)
	}
	if user.ID != 42 || user.Username != "adam" {
		t.Errorf("got %+v", user)
	}
}

func TestLoginRedirectStoresState(t *testing.T) {
	env := newTestEnv(t)
	rec := env.do("GET", "/api/auth/login", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "discord.com") || !strings.Contains(loc, "state=") {
		t.Fatalf("unexpected redirect %q", loc)
	}
	if keys := env.redis.Keys(); len(keys) != 1 || !strings.HasPrefix(keys[0], "oauth_state:") {
		t.Errorf("redis keys = %v, want one oauth_state key", keys)
	}
}

func TestLoginRemembersWhereToReturn(t *testing.T) {
	env := newTestEnv(t)
	env.do("GET", "/api/auth/login?next=%2Ftranscripts%2F42", nil)
	var next string
	for _, k := range env.redis.Keys() {
		if strings.HasPrefix(k, "oauth_next:") {
			next, _ = env.redis.Get(k)
		}
	}
	if next != "/transcripts/42" {
		t.Errorf("stored next = %q, want /transcripts/42", next)
	}
}

func TestLoginIgnoresOffsiteNext(t *testing.T) {
	for _, next := range []string{"https://evil.test", "//evil.test", "/\\evil.test", "evil", "/api/auth/logout"} {
		env := newTestEnv(t)
		env.do("GET", "/api/auth/login?next="+url.QueryEscape(next), nil)
		if keys := env.redis.Keys(); len(keys) != 1 {
			t.Errorf("next=%q stored keys %v, want only the state", next, keys)
		}
	}
}

func TestCallbackCancelledRedirectsHome(t *testing.T) {
	rec := newTestEnv(t).do("GET", "/api/auth/callback?error=access_denied", nil)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestCallbackWithUnknownStateFails(t *testing.T) {
	rec := newTestEnv(t).do("GET", "/api/auth/callback?code=abc&state=forged", nil)
	if loc := rec.Header().Get("Location"); loc != "/?error=login_failed" {
		t.Fatalf("got %d -> %q", rec.Code, loc)
	}
}

func TestInviteRedirect(t *testing.T) {
	rec := newTestEnv(t).do("GET", "/api/invite?guild_id=987654321", nil)
	loc := rec.Header().Get("Location")
	perms := "permissions=" + strconv.FormatInt(int64(config.BotPermissions), 10)
	for _, want := range []string{"client_id=123456789", "guild_id=987654321", perms, "applications.commands"} {
		if !strings.Contains(loc, want) {
			t.Errorf("invite URL %q missing %q", loc, want)
		}
	}
}

func TestInviteSource(t *testing.T) {
	for ref, want := range map[string]string{
		"":           "direct",
		"topgg":      "topgg",
		" TikTok ":   "tiktok",
		"made-up":    "other",
		"<script>":   "other",
		"transcript": "transcript",
	} {
		if got := inviteSource(ref); got != want {
			t.Errorf("inviteSource(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	env := newTestEnv(t)
	want := map[string]string{
		"Content-Security-Policy": "frame-ancestors 'none'",
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
	}
	for _, path := range []string{"/servers/123", "/api/me", "/api/nope", "/_app/immutable/app.js"} {
		rec := env.do("GET", path, nil)
		for name, value := range want {
			if got := rec.Header().Get(name); got != value {
				t.Errorf("%s: %s = %q, want %q", path, name, got, value)
			}
		}
		if hsts := rec.Header().Get("Strict-Transport-Security"); hsts != "" {
			t.Errorf("%s: HSTS %q set on a plain http request", path, hsts)
		}
	}

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	if hsts := rec.Header().Get("Strict-Transport-Security"); !strings.Contains(hsts, "max-age=") {
		t.Errorf("HSTS behind an https proxy = %q", hsts)
	}
}

// An expired Discord token doesn't log the user out by itself: the session
// still works, and the token is only refreshed when Discord is needed.
func TestExpiredDiscordTokenKeepsSession(t *testing.T) {
	env := newTestEnv(t)
	sess := auth.Session{
		User:  auth.User{ID: 42, Username: "adam", DisplayName: "Adam"},
		OAuth: oauth2.Session{AccessToken: "old", RefreshToken: "r", Expiration: time.Now().Add(-time.Hour)},
	}
	data, _ := json.Marshal(sess)
	env.redis.Set("session:expired", string(data))

	cookie := &http.Cookie{Name: "session", Value: "expired"}
	if rec := env.do("GET", "/api/me", cookie); rec.Code != 200 {
		t.Fatalf("/api/me with an expired token: got %d", rec.Code)
	}
}
