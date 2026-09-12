package api

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/adammcgrogan/tickettower/internal/auth"
)

// Request limits. Login is per IP and unauthenticated (each call writes an
// OAuth state to Redis, so a loop could fill it); everything behind a login
// is per user, mainly to keep one dashboard from burning the bot's Discord
// rate limit, which the bot itself shares.
const (
	authLimit   = 10
	authWindow  = time.Minute
	userLimit   = 240
	userWindow  = time.Minute
	limiterIdle = 10 * time.Minute // forget keys quiet this long
)

// rateLimiter allows up to limit requests per key in any window, as a token
// bucket refilled continuously. It's in-process, which matches the single
// API instance; NOTE: revisit if the API is ever run as several replicas.
type rateLimiter struct {
	limit  float64
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: float64(limit), window: window, now: time.Now, buckets: map[string]*bucket{}}
}

// allow reports whether a request for key may proceed, and if not, how long
// until one may.
func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.swept) > limiterIdle {
		for k, b := range l.buckets {
			if now.Sub(b.last) > limiterIdle {
				delete(l.buckets, k)
			}
		}
		l.swept = now
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.limit, last: now}
		l.buckets[key] = b
	}
	perToken := l.window / time.Duration(l.limit)
	b.tokens = min(l.limit, b.tokens+float64(now.Sub(b.last))/float64(perToken))
	b.last = now
	if b.tokens < 1 {
		return false, time.Duration((1 - b.tokens) * float64(perToken))
	}
	b.tokens--
	return true, 0
}

// middleware rejects requests over the limit with a 429 and Retry-After.
// key picks what the limit applies to; an empty key skips the check.
func (l *rateLimiter) middleware(key func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if k := key(r); k != "" {
				if ok, wait := l.allow(k); !ok {
					w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
					writeError(w, http.StatusTooManyRequests, "Too many requests. Please wait a moment and try again.")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// byIP keys on the client address (middleware.RealIP has already applied
// X-Forwarded-For from the proxy).
func byIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr // a bare address, e.g. from a proxy header
}

// byUser keys on the logged-in user.
func byUser(r *http.Request) string {
	if sess := auth.FromContext(r.Context()); sess != nil {
		return sess.User.ID.String()
	}
	return ""
}
