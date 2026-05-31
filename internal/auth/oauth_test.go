package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	c := &OAuthClient{ClientID: "cid", RedirectURL: "http://app/cb", OAuthBaseURL: "https://github.com"}
	got := c.AuthorizeURL("xyz", "read:user public_repo")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != "cid" || q.Get("state") != "xyz" ||
		q.Get("redirect_uri") != "http://app/cb" || q.Get("scope") != "read:user public_repo" {
		t.Fatalf("bad authorize url: %s", got)
	}
}

func TestExchangeAndGetUser(t *testing.T) {
	// Fake GitHub OAuth token endpoint.
	oauthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login/oauth/access_token" {
			t.Errorf("unexpected oauth path %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("missing Accept json header")
		}
		_ = r.ParseForm()
		if r.Form.Get("code") != "thecode" {
			t.Errorf("code = %q", r.Form.Get("code"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"gho_tok","token_type":"bearer","scope":"repo"}`))
	}))
	defer oauthSrv.Close()

	// Fake GitHub API user endpoint.
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("unexpected api path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer gho_tok" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":99,"login":"octocat","avatar_url":"http://a/x.png"}`))
	}))
	defer apiSrv.Close()

	c := &OAuthClient{
		ClientID:     "cid",
		ClientSecret: "sec",
		RedirectURL:  "http://app/cb",
		OAuthBaseURL: oauthSrv.URL,
		APIBaseURL:   apiSrv.URL,
		HTTP:         oauthSrv.Client(),
	}
	ctx := context.Background()
	tok, scope, err := c.Exchange(ctx, "thecode")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "gho_tok" || scope != "repo" {
		t.Fatalf("exchange = %q scope %q", tok, scope)
	}
	u, err := c.GetUser(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 99 || u.Login != "octocat" || !strings.HasSuffix(u.AvatarURL, "x.png") {
		t.Fatalf("user = %+v", u)
	}
}
