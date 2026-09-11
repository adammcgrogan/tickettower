package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/go-chi/chi/v5"

	"github.com/adammcgrogan/tickettower/internal/discordx"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// validationError describes invalid input, optionally tied to a form field.
type validationError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"error"`
}

func (e *validationError) Error() string { return e.Message }

func invalid(field, msg string) error { return &validationError{Field: field, Message: msg} }

// writeFailure responds to an error from a handler, turning validation and
// known Discord errors into helpful messages.
func (s *Server) writeFailure(w http.ResponseWriter, err error) {
	if ve, ok := err.(*validationError); ok {
		writeJSON(w, http.StatusUnprocessableEntity, ve)
		return
	}
	if msg := discordx.Friendly(err); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	s.log.Error("request failed", slog.Any("err", err))
	writeError(w, http.StatusInternalServerError, "Something went wrong. Please try again.")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// normaliseIDs sorts and de-duplicates ids, returning an empty (not nil)
// slice so it encodes as [].
func normaliseIDs(ids []snowflake.ID) []snowflake.ID {
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if ids == nil {
		return []snowflake.ID{}
	}
	return ids
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

// requireJSON rejects state-changing requests that aren't JSON. Browsers
// can't send cross-site JSON without a CORS preflight, so together with
// SameSite cookies this prevents CSRF.
func requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				writeError(w, http.StatusUnsupportedMediaType, "requests must be JSON")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ttlCache is a small in-memory cache for Discord data such as channel and
// role lists.
type ttlCache struct {
	mu    sync.Mutex
	items map[string]cacheItem
}

type cacheItem struct {
	value   any
	expires time.Time
}

func newTTLCache() *ttlCache { return &ttlCache{items: make(map[string]cacheItem)} }

func (c *ttlCache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.items[key]
	if !ok || time.Now().After(it.expires) {
		delete(c.items, key)
		return nil, false
	}
	return it.value, true
}

func (c *ttlCache) set(key string, v any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cacheItem{value: v, expires: time.Now().Add(ttl)}
}
