package auth

import (
	"context"
	"errors"
	"net/http"
)

type ctxKey struct{}

// Middleware rejects requests without a valid session and stores the session
// in the request context.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := m.FromRequest(r)
		if errors.Is(err, ErrNoSession) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		} else if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	})
}

// FromContext returns the session set by Middleware.
func FromContext(ctx context.Context) *Session {
	sess, _ := ctx.Value(ctxKey{}).(*Session)
	return sess
}
