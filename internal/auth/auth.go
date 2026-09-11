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
func (m *Manager) LoginURL() string {
	return m.oauth.GenerateAuthorizationURL(oauth2.AuthorizationURLParams{
		RedirectURI: m.redirectURI,
		Scopes:      []discord.OAuth2Scope{discord.OAuth2ScopeIdentify, discord.OAuth2ScopeGuilds},
	})
}

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

// FromRequest loads the session for a request, refreshing the Discord access
// token if it has expired.
func (m *Manager) FromRequest(r *http.Request) (*Session, error) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil, ErrNoSession
	}
	data, err := m.rdb.Get(r.Context(), sessionKey(c.Value)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNoSession
	} else if err != nil {
		return nil, err
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, ErrNoSession
	}
	sess.ID = c.Value

	if sess.OAuth.Expired() {
		refreshed, err := m.oauth.RefreshSession(sess.OAuth, rest.WithCtx(r.Context()))
		if err != nil {
			return nil, ErrNoSession
		}
		sess.OAuth = refreshed
		if err := m.save(r.Context(), &sess); err != nil {
			return nil, err
		}
	}
	return &sess, nil
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
// rate limits this endpoint heavily.
func (m *Manager) Guilds(ctx context.Context, sess *Session) ([]discord.OAuth2Guild, error) {
	key := "user_guilds:" + sess.User.ID.String()
	if data, err := m.rdb.Get(ctx, key).Bytes(); err == nil {
		var guilds []discord.OAuth2Guild
		if json.Unmarshal(data, &guilds) == nil {
			return guilds, nil
		}
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
	s.rdb.Set(context.Background(), "oauth_state:"+state, redirectURI, 10*time.Minute)
	return state
}

func (s *stateController) UseState(state string) string {
	uri, err := s.rdb.GetDel(context.Background(), "oauth_state:"+state).Result()
	if err != nil {
		return ""
	}
	return uri
}
