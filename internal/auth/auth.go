// Package auth implements Discord OAuth2 login and Redis-backed dashboard
// sessions.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/oauth2"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/redis/go-redis/v9"
)

const (
	cookieName    = "session"
	sessionTTL    = 30 * 24 * time.Hour
	guildCacheTTL = 60 * time.Second
	// stateTTL is how long someone has to finish logging in with Discord.
	stateTTL = 10 * time.Minute
)

var ErrNoSession = errors.New("no session")

type User struct {
	ID          snowflake.ID `json:"id"`
	Username    string       `json:"username"`
	DisplayName string       `json:"display_name"`
	AvatarURL   string       `json:"avatar_url"`
}

type Session struct {
	ID    string         `json:"-"`
	User  User           `json:"user"`
	OAuth oauth2.Session `json:"oauth"`
}

type Manager struct {
	oauth       *oauth2.Client
	rdb         *redis.Client
	redirectURI string
	secure      bool
}

func NewManager(clientID snowflake.ID, clientSecret, publicURL string, rdb *redis.Client) *Manager {
	return &Manager{
		oauth:       oauth2.New(clientID, clientSecret, oauth2.WithStateController(&stateController{rdb: rdb})),
		rdb:         rdb,
		redirectURI: publicURL + "/api/auth/callback",
		secure:      strings.HasPrefix(publicURL, "https://"),
	}
}

// LoginURL returns the Discord authorization URL to redirect the user to.
// next, if not empty, is the path to return to after logging in (see Next).
func (m *Manager) LoginURL(ctx context.Context, next string) string {
	url, state := m.oauth.GenerateAuthorizationURLState(oauth2.AuthorizationURLParams{
		RedirectURI: m.redirectURI,
		Scopes:      []discord.OAuth2Scope{discord.OAuth2ScopeIdentify, discord.OAuth2ScopeGuilds},
	})
	if next != "" {
		m.rdb.Set(ctx, nextKey(state), next, stateTTL)
	}
	return url
}

// Next returns, once, the path a login with this state should return to, or
// "" if it didn't ask for one.
func (m *Manager) Next(ctx context.Context, state string) string {
	next, _ := m.rdb.GetDel(ctx, nextKey(state)).Result()
	return next
}

func nextKey(state string) string { return "oauth_next:" + state }

// Complete exchanges an OAuth2 code for a session and sets the session cookie.
func (m *Manager) Complete(ctx context.Context, w http.ResponseWriter, code, state string) (*Session, error) {
	oauthSession, _, err := m.oauth.StartSession(code, state, rest.WithCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	u, err := m.oauth.GetUser(oauthSession, rest.WithCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("fetch user: %w", err)
	}

	sess := &Session{
		ID:    randomToken(),
		OAuth: oauthSession,
		User: User{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.EffectiveName(),
			AvatarURL:   u.EffectiveAvatarURL(),
		},
	}
	if err := m.save(ctx, sess); err != nil {
		return nil, err
	}
	http.SetCookie(w, m.cookie(sess.ID, int(sessionTTL.Seconds())))
	return sess, nil
}

// FromRequest loads the session for a request. The Discord token is only
// refreshed when something needs it (see Guilds), not on every request.
func (m *Manager) FromRequest(r *http.Request) (*Session, error) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil, ErrNoSession
	}
	return m.load(r.Context(), c.Value)
}

// load reads a session from Redis by ID.
func (m *Manager) load(ctx context.Context, id string) (*Session, error) {
	data, err := m.rdb.Get(ctx, sessionKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNoSession
	} else if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, ErrNoSession
	}
	sess.ID = id
	return &sess, nil
}

// refreshLockTTL bounds how long one request may hold a session's refresh
// lock, and how long others wait for it.
const refreshLockTTL = 10 * time.Second

// refreshIfExpired makes sure sess carries a usable Discord token. A
// dashboard page fires several requests at once, and Discord accepts each
// refresh token only once, so exactly one request refreshes (under a Redis
// lock) and the rest wait for its result and re-read the session. It returns
// ErrNoSession only when the refresh itself failed, meaning the login is
// really over.
func (m *Manager) refreshIfExpired(ctx context.Context, sess *Session) error {
	if !sess.OAuth.Expired() {
		return nil
	}
	lockKey := "oauth_refresh:" + sess.ID
	locked, err := m.rdb.SetNX(ctx, lockKey, "1", refreshLockTTL).Result()
	if err != nil {
		return err
	}
	if locked {
		defer m.rdb.Del(context.WithoutCancel(ctx), lockKey)
		// Another request may have finished refreshing just before we took
		// the lock.
		if cur, err := m.load(ctx, sess.ID); err == nil && !cur.OAuth.Expired() {
			sess.OAuth = cur.OAuth
			return nil
		}
		refreshed, err := m.oauth.RefreshSession(sess.OAuth, rest.WithCtx(ctx))
		if err != nil {
			return ErrNoSession
		}
		sess.OAuth = refreshed
		return m.save(ctx, sess)
	}

	// Someone else is refreshing: wait for the lock to go, then use what
	// they saved.
	deadline := time.Now().Add(refreshLockTTL)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		cur, err := m.load(ctx, sess.ID)
		if err != nil {
			return err
		}
		if !cur.OAuth.Expired() {
			sess.OAuth = cur.OAuth
			return nil
		}
		if n, _ := m.rdb.Exists(ctx, lockKey).Result(); n == 0 {
			return ErrNoSession // the refresh failed
		}
	}
	return errors.New("timed out waiting for the session's token refresh")
}

func (m *Manager) Logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(cookieName); err == nil {
		if err := m.rdb.Del(r.Context(), sessionKey(c.Value)).Err(); err != nil {
			return err
		}
	}
	http.SetCookie(w, m.cookie("", -1))
	return nil
}

// Guilds returns the guilds the user is in, cached briefly because Discord
// rate limits this endpoint heavily. It returns ErrNoSession if the Discord
// login behind the session can no longer be refreshed.
func (m *Manager) Guilds(ctx context.Context, sess *Session) ([]discord.OAuth2Guild, error) {
	key := "user_guilds:" + sess.User.ID.String()
	if data, err := m.rdb.Get(ctx, key).Bytes(); err == nil {
		var guilds []discord.OAuth2Guild
		if json.Unmarshal(data, &guilds) == nil {
			return guilds, nil
		}
	}

	if err := m.refreshIfExpired(ctx, sess); err != nil {
		return nil, err
	}
	guilds, err := m.oauth.GetGuilds(sess.OAuth, rest.WithCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("fetch guilds: %w", err)
	}
	if data, err := json.Marshal(guilds); err == nil {
		m.rdb.Set(ctx, key, data, guildCacheTTL)
	}
	return guilds, nil
}

// InvalidateGuilds drops the cached guild list, e.g. after the user adds the
// bot to a new server.
func (m *Manager) InvalidateGuilds(ctx context.Context, userID snowflake.ID) {
	m.rdb.Del(ctx, "user_guilds:"+userID.String())
}

func (m *Manager) save(ctx context.Context, sess *Session) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	return m.rdb.Set(ctx, sessionKey(sess.ID), data, sessionTTL).Err()
}

func (m *Manager) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func sessionKey(id string) string { return "session:" + id }

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// stateController stores OAuth2 state in Redis so logins work across API
// restarts and multiple replicas.
type stateController struct {
	rdb *redis.Client
}

func (s *stateController) NewState(redirectURI string) string {
	state := randomToken()
	s.rdb.Set(context.Background(), "oauth_state:"+state, redirectURI, stateTTL)
	return state
}

func (s *stateController) UseState(state string) string {
	uri, err := s.rdb.GetDel(context.Background(), "oauth_state:"+state).Result()
	if err != nil {
		return ""
	}
	return uri
}
