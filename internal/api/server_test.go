package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	root := t.TempDir()
	static := filepath.Join(root, "build")
	writeFile(t, filepath.Join(static, "index.html"), "<!doctype html>spa")
	writeFile(t, filepath.Join(static, "_app", "immutable", "app.js"), "console.log(1)")
	writeFile(t, filepath.Join(root, "secret.txt"), "secret")

	cfg := config.Config{
		AppName:         "Ticket Tower",
		DiscordClientID: 123456789,
		PublicURL:       "http://localhost:5173",
		StaticDir:       static,
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
		{"client route falls back to index", "/servers/123", "spa"},
		{"root serves index", "/", "spa"},
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

	rec := env.do("GET", "/_app/immutable/app.js", nil)
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("immutable asset Cache-Control = %q", cc)
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
