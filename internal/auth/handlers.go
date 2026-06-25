package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"strings"

	"github-stats/internal/store"
)

const stateCookie = "gs_oauth_state"

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) secureCookies(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return strings.HasPrefix(s.Cfg.BaseURL, "https://")
}

// Login starts the OAuth flow: set a state cookie and redirect to GitHub.
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.Redirect(w, r, s.OAuth.AuthorizeURL(state, s.Cfg.GitHubScopes), http.StatusFound)
}

// Callback completes the OAuth flow.
func (s *Service) Callback(w http.ResponseWriter, r *http.Request) {
	stateParam := r.URL.Query().Get("state")
	c, err := r.Cookie(stateCookie)
	if err != nil || stateParam == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(stateParam)) != 1 {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	token, scope, err := s.OAuth.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "oauth exchange failed", http.StatusBadGateway)
		return
	}
	ghUser, err := s.OAuth.GetUser(r.Context(), token)
	if err != nil {
		http.Error(w, "fetch user failed", http.StatusBadGateway)
		return
	}

	// Verify the granted scope against what we requested. A shortfall (e.g. the
	// user declined `repo`) is informational, not fatal — private tracking still
	// works for public repos; we just log a warning so it can be diagnosed.
	if missing := MissingScopes(s.Cfg.GitHubScopes, scope); missing != nil {
		log.Printf("oauth: user %s granted scope %q, missing %v (private repos may be inaccessible)",
			ghUser.Login, scope, missing)
	}

	uid, err := s.Store.UpsertUser(r.Context(), &store.User{
		GitHubID: ghUser.ID, Login: ghUser.Login, AvatarURL: ghUser.AvatarURL,
	})
	if err != nil {
		http.Error(w, "persist user failed", http.StatusInternalServerError)
		return
	}
	encToken, err := s.Cipher.Encrypt([]byte(token))
	if err != nil {
		http.Error(w, "encrypt failed", http.StatusInternalServerError)
		return
	}
	if err := s.Store.UpsertCredential(r.Context(), &store.Credential{
		UserID: uid, Kind: "oauth", EncToken: encToken, Scopes: scope,
	}); err != nil {
		http.Error(w, "persist credential failed", http.StatusInternalServerError)
		return
	}
	sess, err := s.Store.CreateSession(r.Context(), uid, s.Cfg.SessionTTL)
	if err != nil {
		http.Error(w, "session failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	// Clear the state cookie.
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout deletes the current session (POST + CSRF). Replaces the M1 GET redirect.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.VerifyCSRF(r) {
		http.Error(w, "csrf", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.Store.DeleteSession(r.Context(), c.Value)
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// LogoutEverywhere deletes ALL of the user's sessions (POST + CSRF).
func (s *Service) LogoutEverywhere(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.VerifyCSRF(r) {
		http.Error(w, "csrf", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if sess, err := s.Store.GetSession(r.Context(), c.Value); err == nil {
			_, _ = s.Store.DeleteSessionsForUser(r.Context(), sess.UserID)
		}
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
}
