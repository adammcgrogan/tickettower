package api

import (
	"net/http"
	"testing"
	"time"
)

func TestRateLimiterRefills(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := newRateLimiter(3, 30*time.Second) // a token every 10s
	l.now = func() time.Time { return now }

	for i := range 3 {
		if ok, _ := l.allow("a"); !ok {
			t.Fatalf("request %d refused under the limit", i+1)
		}
	}
	ok, wait := l.allow("a")
	if ok || wait <= 0 || wait > 10*time.Second {
		t.Fatalf("4th request: ok=%v wait=%v, want refused for up to 10s", ok, wait)
	}
	if ok, _ := l.allow("b"); !ok {
		t.Error("another key is limited by the first")
	}
	now = now.Add(10 * time.Second)
	if ok, _ := l.allow("a"); !ok {
		t.Error("no token after the refill interval")
	}
	if ok, _ := l.allow("a"); ok {
		t.Error("two tokens after one refill interval")
	}
	now = now.Add(time.Hour)
	for range 3 {
		if ok, _ := l.allow("a"); !ok {
			t.Error("bucket not full again after an hour")
		}
	}
}

func TestLoginIsRateLimitedPerIP(t *testing.T) {
	env := newTestEnv(t)
	for i := range authLimit {
		if rec := env.do("GET", "/api/auth/login", nil); rec.Code != http.StatusFound {
			t.Fatalf("login %d: got %d", i+1, rec.Code)
		}
	}
	rec := env.do("GET", "/api/auth/login", nil)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("over the limit: got %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if keys := env.redis.Keys(); len(keys) != authLimit {
		t.Errorf("redis keys = %d, want %d (the refused login wrote nothing)", len(keys), authLimit)
	}
	// Unrelated routes are unaffected.
	if rec := env.do("GET", "/api/config", nil); rec.Code != http.StatusOK {
		t.Errorf("/api/config after auth limit: %d", rec.Code)
	}
}
