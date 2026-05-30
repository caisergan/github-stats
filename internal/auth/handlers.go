package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github-stats/internal/store"
)

const stateCookie = "gs_oauth_state"

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) secureCookies() bool {
	return strings.HasPrefix(s.Cfg.BaseURL, "https://")
}

// Login starts the OAuth flow: set a state cookie and redirect to GitHub.
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	state := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.Redirect(w, r, s.OAuth.AuthorizeURL(state, s.Cfg.GitHubScopes), http.StatusFound)
}

// Callback completes the OAuth flow.
func (s *Service) Callback(w http.ResponseWriter, r *http.Request) {
	stateParam := r.URL.Query().Get("state")
	c, err := r.Cookie(stateCookie)
	if err != nil || stateParam == "" || c.Value != stateParam {
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
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	// Clear the state cookie.
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout deletes the session and clears the cookie.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.Store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}
