package githubapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github-stats/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestETagTransportCachesAndRevalidates(t *testing.T) {
	st := openTestStore(t)

	var hits int32
	var sawINM atomic.Value
	sawINM.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if inm := r.Header.Get("If-None-Match"); inm != "" {
			sawINM.Store(inm)
			if inm == `W/"v1"` {
				w.WriteHeader(http.StatusNotModified) // 304 — free, no body
				return
			}
		}
		w.Header().Set("ETag", `W/"v1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"repo"}`))
	}))
	defer srv.Close()

	rt := &ETagTransport{Store: st, Base: http.DefaultTransport}
	client := &http.Client{Transport: rt}

	// First call: miss → 200, body cached.
	req1, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/repos/a/b", nil)
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if resp1.StatusCode != 200 || string(body1) != `{"name":"repo"}` {
		t.Fatalf("first call: status=%d body=%s", resp1.StatusCode, body1)
	}

	// Second call: cached etag sent; server returns 304; transport serves cached body as 200.
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/repos/a/b", nil)
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("second call status = %d, want 200 (served from cache)", resp2.StatusCode)
	}
	if string(body2) != `{"name":"repo"}` {
		t.Fatalf("second call body = %s, want cached body", body2)
	}
	if sawINM.Load().(string) != `W/"v1"` {
		t.Fatalf("If-None-Match sent = %q, want W/\"v1\"", sawINM.Load())
	}
	if hits != 2 {
		t.Fatalf("server hits = %d, want 2", hits)
	}
}
