package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/disgoorg/disgo/oauth2"
	"github.com/redis/go-redis/v9"
)

func newTestManager(t *testing.T) (*Manager, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return NewManager(1, "secret", "http://localhost:5173", rdb), mr
}

func expiredSession() *Session {
	return &Session{
		ID:    "s1",
		User:  User{ID: 42},
		OAuth: oauth2.Session{AccessToken: "old", RefreshToken: "r", Expiration: time.Now().Add(-time.Hour)},
	}
}

func saveSession(t *testing.T, mr *miniredis.Miniredis, sess *Session) {
	t.Helper()
	data, _ := json.Marshal(sess)
	mr.Set(sessionKey(sess.ID), string(data))
}

// A cached guild list is served without touching the token at all, so an
// expired token never fails a request that doesn't need Discord.
func TestGuildsUsesCacheWithoutRefreshing(t *testing.T) {
	m, mr := newTestManager(t)
	sess := expiredSession()
	saveSession(t, mr, sess)
	mr.Set("user_guilds:42", "[]")

	guilds, err := m.Guilds(context.Background(), sess)
	if err != nil || len(guilds) != 0 {
		t.Fatalf("got %v, %v", guilds, err)
	}
	if sess.OAuth.AccessToken != "old" {
		t.Error("token was refreshed although the cache was warm")
	}
}

// While another request holds the refresh lock, this one waits and then
// picks up the session that request saved.
func TestRefreshWaitsForTheRequestHoldingTheLock(t *testing.T) {
	m, mr := newTestManager(t)
	sess := expiredSession()
	saveSession(t, mr, sess)
	mr.Set("oauth_refresh:s1", "1")

	go func() {
		time.Sleep(250 * time.Millisecond)
		fresh := expiredSession()
		fresh.OAuth = oauth2.Session{AccessToken: "new", RefreshToken: "r2", Expiration: time.Now().Add(time.Hour)}
		saveSession(t, mr, fresh)
		mr.Del("oauth_refresh:s1")
	}()

	if err := m.refreshIfExpired(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	if sess.OAuth.AccessToken != "new" {
		t.Errorf("token = %q, want the one the other request saved", sess.OAuth.AccessToken)
	}
}

// If the request holding the lock gives up without a new token, the login is
// over for everyone waiting too.
func TestRefreshFailureEndsTheSession(t *testing.T) {
	m, mr := newTestManager(t)
	sess := expiredSession()
	saveSession(t, mr, sess)
	mr.Set("oauth_refresh:s1", "1")

	go func() {
		time.Sleep(250 * time.Millisecond)
		mr.Del("oauth_refresh:s1")
	}()

	if err := m.refreshIfExpired(context.Background(), sess); !errors.Is(err, ErrNoSession) {
		t.Fatalf("got %v, want ErrNoSession", err)
	}
}
