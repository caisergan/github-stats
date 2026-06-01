package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// fakeBackfillGraphQL answers a full backfill (meta + one page each) so a
// 'backfill' job can run to completion in-process.
func fakeBackfillGraphQL(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		w.Header().Set("Content-Type", "application/json")
		const rl = `"rateLimit":{"cost":1,"remaining":4990,"resetAt":"2026-06-01T13:00:00Z"}`
		switch {
		case strings.Contains(req.Query, "databaseId"):
			w.Write([]byte(`{"data":{"repository":{"databaseId":1,"nameWithOwner":"octocat/hello",
				"isPrivate":false,"description":"hi","stargazerCount":1,"forkCount":0,
				"defaultBranchRef":{"name":"main"}},` + rl + `}}`))
		case strings.Contains(req.Query, "history"):
			w.Write([]byte(`{"data":{"repository":{"ref":{"target":{"history":{
				"pageInfo":{"endCursor":"C1","hasNextPage":false},
				"nodes":[{"oid":"sha1","additions":1,"deletions":0,
					"committedDate":"2026-05-01T08:00:00Z","messageHeadline":"x",
					"author":{"user":{"login":"neo"}}}]}}}}},` + rl + `}}`))
		case strings.Contains(req.Query, "pullRequests"):
			w.Write([]byte(`{"data":{"repository":{"pullRequests":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		case strings.Contains(req.Query, "issues"):
			w.Write([]byte(`{"data":{"repository":{"issues":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		case strings.Contains(req.Query, "releases"):
			w.Write([]byte(`{"data":{"repository":{"releases":{"pageInfo":{"endCursor":"","hasNextPage":false},"nodes":[]}},` + rl + `}}`))
		default:
			t.Errorf("unexpected query: %s", req.Query)
		}
	}
}

func newEngine(t *testing.T, st *store.Store, srvURL string, now func() time.Time) *Engine {
	t.Helper()
	factory := func(repoID int64) (*githubapi.Client, error) {
		return githubapi.NewClient(githubapi.Options{
			Token: "gho_test", GraphQLURL: srvURL, RESTBaseURL: srvURL, Store: st, HTTP: &http.Client{},
		}), nil
	}
	return NewEngine(st, factory, Config{
		Now:               now,
		Concurrency:       2,
		SchedulerInterval: time.Minute,
		DeltaCadence:      30 * time.Minute,
		MaxAttempts:       3,
		FailBackoff:       5 * time.Minute,
	})
}

func TestProcessNextJobRunsBackfill(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	srv := httptest.NewServer(fakeBackfillGraphQL(t))
	defer srv.Close()

	now := ptime("2026-05-21T00:00:00Z")
	eng := newEngine(t, st, srv.URL, func() time.Time { return now })

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	jobID, err := eng.TriggerBackfill(ctx, repoID)
	if err != nil {
		t.Fatal(err)
	}

	ran, err := eng.processNextJob(ctx)
	if err != nil {
		t.Fatalf("processNextJob: %v", err)
	}
	if !ran {
		t.Fatal("expected a job to be processed")
	}

	// Job marked done.
	jobs, _ := st.ListJobsForRepo(ctx, repoID)
	if len(jobs) != 1 || jobs[0].ID != jobID || jobs[0].Status != "done" {
		t.Fatalf("job not done: %+v", jobs)
	}
	// Backfill wrote a commit.
	var n int
	st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM commits WHERE repo_id=?`, repoID).Scan(&n)
	if n != 1 {
		t.Fatalf("commits = %d, want 1", n)
	}
}

func TestProcessNextJobNoWork(t *testing.T) {
	st := openTestStore(t)
	eng := newEngine(t, st, "http://unused", func() time.Time { return ptime("2026-05-21T00:00:00Z") })
	ran, err := eng.processNextJob(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("expected no job to process on an empty queue")
	}
}

func TestProcessNextJobFailIsRecorded(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	// A server that errors every GraphQL query forces the backfill to fail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":null,"errors":[{"message":"boom"}]}`))
	}))
	defer srv.Close()

	now := ptime("2026-05-21T00:00:00Z")
	eng := newEngine(t, st, srv.URL, func() time.Time { return now })
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	if _, err := eng.TriggerBackfill(ctx, repoID); err != nil {
		t.Fatal(err)
	}

	ran, err := eng.processNextJob(ctx)
	if err != nil {
		t.Fatalf("processNextJob should not surface job errors: %v", err)
	}
	if !ran {
		t.Fatal("expected the job to be processed (and fail)")
	}
	jobs, _ := st.ListJobsForRepo(ctx, repoID)
	if len(jobs) != 1 || jobs[0].Attempts != 1 || jobs[0].LastError == "" {
		t.Fatalf("failed job not recorded: %+v", jobs)
	}
	// With MaxAttempts 3 the first failure reschedules to pending.
	if jobs[0].Status != "pending" {
		t.Fatalf("status = %q, want pending after first failure", jobs[0].Status)
	}
}

func TestEnqueueDueDeltasRespectsCadence(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	uid := mustUser(t, st)
	now := ptime("2026-05-21T12:00:00Z")
	eng := newEngine(t, st, "http://unused", func() time.Time { return now })

	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	if err := st.TrackRepo(ctx, uid, repoID); err != nil {
		t.Fatal(err)
	}

	// Stale repo: last backfill long ago → a delta job is enqueued.
	old := now.Add(-2 * time.Hour)
	st.UpsertSyncState(ctx, &store.SyncState{RepoID: repoID, LastBackfillAt: &old, Status: "complete"})

	if err := eng.enqueueDueDeltas(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, _ := st.ListJobsForRepo(ctx, repoID)
	deltas := 0
	for _, j := range jobs {
		if j.Kind == "delta" {
			deltas++
		}
	}
	if deltas != 1 {
		t.Fatalf("expected 1 delta enqueued, got %d", deltas)
	}

	// Running again immediately must NOT enqueue a second (cadence not elapsed,
	// and a pending delta already exists).
	if err := eng.enqueueDueDeltas(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, _ = st.ListJobsForRepo(ctx, repoID)
	deltas = 0
	for _, j := range jobs {
		if j.Kind == "delta" {
			deltas++
		}
	}
	if deltas != 1 {
		t.Fatalf("cadence guard failed: %d delta jobs", deltas)
	}
}

func TestStartStopLifecycleDrainsQueue(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	srv := httptest.NewServer(fakeBackfillGraphQL(t))
	defer srv.Close()

	now := ptime("2026-05-21T00:00:00Z")
	eng := newEngine(t, st, srv.URL, func() time.Time { return now })
	repoID, _ := st.UpsertRepo(ctx, &store.Repo{GitHubID: 1, FullName: "octocat/hello", DefaultBranch: "main"})
	if _, err := eng.TriggerBackfill(ctx, repoID); err != nil {
		t.Fatal(err)
	}

	eng.Start(ctx)
	// Poll the DB (no fixed sleep) until the job is done or we time out.
	deadline := time.Now().Add(5 * time.Second)
	done := false
	for time.Now().Before(deadline) {
		jobs, _ := st.ListJobsForRepo(ctx, repoID)
		if len(jobs) == 1 && jobs[0].Status == "done" {
			done = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	eng.Stop()
	if !done {
		t.Fatal("worker pool did not drain the backfill job")
	}
}

func mustUser(t *testing.T, st *store.Store) int64 {
	t.Helper()
	id, err := st.UpsertUser(context.Background(), &store.User{GitHubID: 1, Login: "u"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
