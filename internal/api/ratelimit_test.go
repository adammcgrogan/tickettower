package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

// A client can't dodge the login limit by claiming to be someone else: with
// no trusted proxy, forwarded headers are ignored altogether.
func TestLoginLimitIgnoresSpoofedIPHeaders(t *testing.T) {
	env := newTestEnv(t)
	for i := range authLimit {
		req := httptest.NewRequest("GET", "/api/auth/login", nil)
		req.Header.Set("X-Real-IP", fmt.Sprintf("10.0.0.%d", i))
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.1.0.%d", i))
		req.Header.Set("True-Client-IP", fmt.Sprintf("10.2.0.%d", i))
		rec := httptest.NewRecorder()
		env.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			t.Fatalf("login %d: got %d", i+1, rec.Code)
		}
	}
	req := httptest.NewRequest("GET", "/api/auth/login", nil)
	req.Header.Set("X-Real-IP", "10.9.9.9")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("spoofed header bypassed the limit: got %d", rec.Code)
	}
}

// Behind a trusted proxy, the address it appends to X-Forwarded-For is the
// client, and anything the client put in front of it is ignored.
func TestByIPBehindTrustedProxy(t *testing.T) {
	var got string
	h := clientIP(1)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = byIP(r) }))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.7")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "203.0.113.7" {
		t.Errorf("client ip = %q, want the proxy-added entry", got)
	}
}
