package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	syncpkg "github-stats/internal/sync"
)

func TestSyncStreamForwardsEvents(t *testing.T) {
	srv, st, cookie := serverWithGitHub(t, "http://unused")
	repoID, _ := mustTrackedRepo(t, st)

	// Use the real chi server over a listener so streaming + flush behave.
	httpSrv := httptest.NewServer(srv.Router())
	defer httpSrv.Close()

	req, _ := http.NewRequest(http.MethodGet,
		httpSrv.URL+"/api/repos/"+strconv.FormatInt(repoID, 10)+"/sync/stream", nil)
	req.AddCookie(cookie)

	// Cancel the request after we have read one event so the handler returns.
	reqCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req = req.WithContext(reqCtx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	// Publish a terminal event from the engine broadcaster; the handler should
	// forward it and then close.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Give the handler a moment to subscribe, then publish.
		for i := 0; i < 200; i++ {
			srv.engine.Broadcaster().PublishForTest(repoID, syncpkg.Event{RepoID: repoID, Phase: "done", Message: "complete", Done: true})
			time.Sleep(5 * time.Millisecond)
		}
	}()

	reader := bufio.NewReader(resp.Body)
	var got string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if strings.HasPrefix(line, "data:") {
			got = line
			break
		}
		if err != nil {
			break
		}
	}
	cancel()
	wg.Wait()

	if !strings.Contains(got, `"phase":"done"`) || !strings.Contains(got, `"done":true`) {
		t.Fatalf("did not receive forwarded SSE event, got %q", got)
	}
}

func TestSyncStreamRejectsUntracked(t *testing.T) {
	srv, _, cookie := serverWithGitHub(t, "http://unused")
	req := httptest.NewRequest(http.MethodGet, "/api/repos/999/sync/stream", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for untracked repo", rec.Code)
	}
}
