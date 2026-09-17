package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/disgoorg/disgo/oauth2"
	"github.com/disgoorg/snowflake/v2"

	"github.com/adammcgrogan/tickettower/internal/auth"
	"github.com/adammcgrogan/tickettower/internal/config"
)

func TestAdminRequiresSuperadmin(t *testing.T) {
	env := newTestEnv(t, func(c *config.Config) { c.SuperadminUserID = 999 })

	login := func(userID int64) *http.Cookie {
		cookie := strconv.FormatInt(userID, 10)
		sess := auth.Session{
			User:  auth.User{ID: snowflake.ID(userID), Username: "u"},
			OAuth: oauth2.Session{AccessToken: "token", Expiration: time.Now().Add(time.Hour)},
		}
		data, _ := json.Marshal(sess)
		env.redis.Set("session:"+cookie, string(data))
		return &http.Cookie{Name: "session", Value: cookie}
	}

	// Someone else's valid session gets a 404, not a 403: the panel's
	// existence isn't advertised to other logged-in users.
	if rec := env.do("GET", "/api/admin/overview", login(42)); rec.Code != http.StatusNotFound {
		t.Errorf("non-owner: got %d, want 404", rec.Code)
	}
	// No session at all is still a plain 401 from the auth middleware.
	if rec := env.do("GET", "/api/admin/overview", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no session: got %d, want 401", rec.Code)
	}

	rec := env.do("GET", "/api/me", login(999))
	if rec.Code != 200 {
		t.Fatalf("me: got %d", rec.Code)
	}
	var me struct {
		IsSuperadmin bool `json:"is_superadmin"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if !me.IsSuperadmin {
		t.Error("owner session: is_superadmin = false, want true")
	}
}

func TestAdminReportRequiresToken(t *testing.T) {
	// No token configured: the route is hidden even from a correct-looking header.
	if rec := newTestEnv(t).doWithAuth("/api/ops/report", "Bearer anything"); rec.Code != http.StatusNotFound {
		t.Errorf("no token configured = %d, want 404", rec.Code)
	}
	env := newTestEnv(t, func(c *config.Config) { c.AdminAPIToken = "s3cret" })
	for _, auth := range []string{"", "s3cret", "Bearer wrong", "Bearer s3cre"} {
		if rec := env.doWithAuth("/api/ops/report", auth); rec.Code != http.StatusNotFound {
			t.Errorf("Authorization %q = %d, want 404", auth, rec.Code)
		}
	}
}
