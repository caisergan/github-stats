package auth

import (
	"context"
	"net/http"

	"github-stats/internal/store"
)

type ctxKey int

const userCtxKey ctxKey = 0

// UserFromContext returns the authenticated user set by RequireUser.
func UserFromContext(ctx context.Context) (*store.User, bool) {
	u, ok := ctx.Value(userCtxKey).(*store.User)
	return u, ok
}

// RequireUser is middleware that loads the session user or returns 401.
func (s *Service) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		sess, err := s.Store.GetSession(r.Context(), c.Value)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		user, err := s.Store.GetUserByID(r.Context(), sess.UserID)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
