package sync

import (
	"context"
	"errors"
	"fmt"
	stdsync "sync"
	"time"

	"github-stats/internal/backfill"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
)

// ClientFactory mints a GitHub client for a repo (the API wires this to decrypt
// the tracking user's OAuth token). Returning an error fails the job.
type ClientFactory func(repoID int64) (*githubapi.Client, error)

// Config tunes the engine. All fields have sane defaults applied by NewEngine.
type Config struct {
	Now               func() time.Time // injected clock (defaults to time.Now)
	Concurrency       int              // worker goroutines (default 4)
	SchedulerInterval time.Duration    // scheduler tick (default 1m)
	DeltaCadence      time.Duration    // min age before a repo is re-delta'd (default 30m)
	MaxAttempts       int              // job failures before terminal error (default 5)
	FailBackoff       time.Duration    // base backoff between attempts (default 1m)
	IdleWait          time.Duration    // worker sleep when the queue is empty (default 200ms)
	JobRetention      time.Duration    // age after which done/error jobs are pruned (default 7d)
}

// Engine owns the worker pool, the scheduler, and the progress broadcaster.
type Engine struct {
	store     *store.Store
	newClient ClientFactory
	cfg       Config
	bc        *Broadcaster
	budget    *githubapi.Budget // shared rate-limit budget injected into every worker's client

	cancel context.CancelFunc
	wg     stdsync.WaitGroup
}

// NewEngine builds an Engine, applying defaults to any zero Config fields.
func NewEngine(st *store.Store, factory ClientFactory, cfg Config) *Engine {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.SchedulerInterval <= 0 {
		cfg.SchedulerInterval = time.Minute
	}
	if cfg.DeltaCadence <= 0 {
		cfg.DeltaCadence = 30 * time.Minute
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.FailBackoff <= 0 {
		cfg.FailBackoff = time.Minute
	}
	if cfg.IdleWait <= 0 {
		cfg.IdleWait = 200 * time.Millisecond
	}
	if cfg.JobRetention <= 0 {
		cfg.JobRetention = 7 * 24 * time.Hour
	}
	return &Engine{store: st, newClient: factory, cfg: cfg, bc: NewBroadcaster(), budget: githubapi.NewBudget()}
}

// Broadcaster exposes the engine's progress broadcaster (the SSE handler
// subscribes to it).
func (e *Engine) Broadcaster() *Broadcaster { return e.bc }

// Budget returns the engine's shared rate-limit budget (REST + GraphQL). Every
// client minted for a worker shares this single Budget (see processNextJob), so
// the snapshot reflects real usage across all sync jobs. Safe for concurrent use.
func (e *Engine) Budget() *githubapi.Budget { return e.budget }

// TriggerBackfill enqueues a backfill job for repoID, runnable now.
func (e *Engine) TriggerBackfill(ctx context.Context, repoID int64) (int64, error) {
	return e.store.EnqueueJob(ctx, repoID, "backfill", e.cfg.Now())
}

// TriggerDelta enqueues a delta job for repoID, runnable now.
func (e *Engine) TriggerDelta(ctx context.Context, repoID int64) (int64, error) {
	return e.store.EnqueueJob(ctx, repoID, "delta", e.cfg.Now())
}

// processNextJob leases one runnable job and runs it to completion, returning
// whether a job was processed. Job-level errors are recorded via FailJob (and
// do NOT propagate as the returned error); the returned error is reserved for
// infrastructure failures (e.g. the lease query itself). Safe to call from many
// goroutines concurrently — the lease is atomic.
func (e *Engine) processNextJob(ctx context.Context) (bool, error) {
	now := e.cfg.Now()
	job, err := e.store.LeaseNextJob(ctx, now)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}

	client, err := e.newClient(job.RepoID)
	if err != nil {
		e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: "error", Message: err.Error(), Done: true})
		_ = e.store.FailJob(ctx, job.ID, "client: "+err.Error(), now, e.cfg.FailBackoff, e.cfg.MaxAttempts)
		return true, nil
	}
	// Share the engine's single Budget across every worker's client so the
	// rate-limit snapshot (GET /api/rate-limit) reflects real usage. The factory
	// allocates a fresh per-client Budget; we replace it with the shared one
	// before the client issues any request. Budget is concurrency-safe.
	if e.budget != nil && client.Budget != e.budget {
		client.Budget = e.budget
	}

	e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: job.Kind, Message: "started"})

	runErr := e.runJob(ctx, job, client)

	// Success → mark done.
	if runErr == nil {
		_ = e.store.CompleteJob(ctx, job.ID)
		e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: "done", Message: "complete", Done: true})
		return true, nil
	}
	// Rate-limited → reschedule at the bucket reset WITHOUT counting a failure
	// (the cursor is already persisted by backfill/delta). A typed RateLimitError
	// is the only signal we treat this way, so a genuine error that merely
	// coincides with a drained bucket is still recorded as a failure below.
	var rlErr *githubapi.RateLimitError
	if errors.As(runErr, &rlErr) {
		reset := rlErr.Reset
		if !reset.After(e.cfg.Now()) {
			// Unknown or already-past reset: back off rather than hot-loop.
			reset = e.cfg.Now().Add(e.cfg.FailBackoff)
		}
		_ = e.store.RescheduleJob(ctx, job.ID, reset)
		// Surface a non-terminal "throttled" event (Done:false) so the UI can tell
		// the user the sync was rate-limited but will resume on its own — the
		// open SSE stream still receives the eventual completion of the re-run.
		secs := int(reset.Sub(e.cfg.Now()).Round(time.Second).Seconds())
		msg := fmt.Sprintf("GitHub rate-limited this sync — resuming automatically in ~%ds", secs)
		e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: "throttled", Message: msg})
		return true, nil
	}
	// Genuine error → record the failure (reschedule-with-backoff or terminal).
	_ = e.store.FailJob(ctx, job.ID, runErr.Error(), e.cfg.Now(), e.cfg.FailBackoff, e.cfg.MaxAttempts)
	e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: "error", Message: runErr.Error(), Done: true})
	return true, nil
}

// runJob dispatches by kind. It threads a progress callback that fans per-page
// fetch counts out to the repo's SSE subscribers ("fetched N commits…").
func (e *Engine) runJob(ctx context.Context, job *store.SyncJob, client *githubapi.Client) error {
	emit := func(phase, detail string) {
		e.bc.publish(job.RepoID, Event{RepoID: job.RepoID, Phase: phase, Message: detail})
	}
	switch job.Kind {
	case "backfill":
		return backfill.Run(ctx, e.store, client, job.RepoID, emit)
	case "delta":
		return RunDelta(ctx, e.store, client, job.RepoID, e.cfg.Now, emit)
	default:
		return fmt.Errorf("unknown job kind %q", job.Kind)
	}
}

// enqueueDueDeltas enqueues a delta job for every tracked repo whose last sync
// is older than DeltaCadence and that has no pending/running job already. It is
// the scheduler's body, exposed for direct testing.
func (e *Engine) enqueueDueDeltas(ctx context.Context) error {
	now := e.cfg.Now()
	userIDs, err := e.trackingUserIDs(ctx)
	if err != nil {
		return err
	}
	seen := make(map[int64]bool)
	for _, uid := range userIDs {
		repos, err := e.store.ListTrackedRepos(ctx, uid)
		if err != nil {
			return err
		}
		for _, r := range repos {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true

			ss, err := e.store.GetSyncState(ctx, r.ID)
			if err != nil {
				return err
			}
			// A delta only makes sense once a backfill has completed; until then
			// (never backfilled, or backfill still in flight / terminally failed)
			// leave the repo to its backfill job. This also stops a permanently
			// failing backfill from spawning a delta on every tick.
			if ss.LastBackfillAt == nil {
				continue
			}
			// Throttle by the freshest sync marker. Keying off LastBackfillAt
			// alone never re-arms (backfill stamps it once), so RunDelta also
			// stamps LastDeltaAt and we gate on the more recent of the two.
			lastSync := *ss.LastBackfillAt
			if ss.LastDeltaAt != nil && ss.LastDeltaAt.After(lastSync) {
				lastSync = *ss.LastDeltaAt
			}
			if now.Sub(lastSync) < e.cfg.DeltaCadence {
				continue
			}
			open, err := e.store.HasOpenJob(ctx, r.ID)
			if err != nil {
				return err
			}
			if open {
				continue
			}
			if _, err := e.store.EnqueueJob(ctx, r.ID, "delta", now); err != nil {
				return err
			}
		}
	}
	return nil
}

// trackingUserIDs returns the distinct user ids that track any repo.
func (e *Engine) trackingUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := e.store.DB.QueryContext(ctx, `SELECT DISTINCT user_id FROM repo_tracking`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Start launches the worker pool and the scheduler in background goroutines.
func (e *Engine) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	for i := 0; i < e.cfg.Concurrency; i++ {
		e.wg.Add(1)
		go e.worker(ctx)
	}
	e.wg.Add(1)
	go e.scheduler(ctx)
}

// worker loops processing jobs until ctx is cancelled, idling briefly when the
// queue is empty.
func (e *Engine) worker(ctx context.Context) {
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		ran, err := e.processNextJob(ctx)
		if err != nil || !ran {
			select {
			case <-ctx.Done():
				return
			case <-time.After(e.cfg.IdleWait):
			}
		}
	}
}

// scheduler ticks on SchedulerInterval, enqueuing due delta jobs.
func (e *Engine) scheduler(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(e.cfg.SchedulerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = e.enqueueDueDeltas(ctx)
			_, _ = e.store.PruneTerminalJobs(ctx, e.cfg.Now().Add(-e.cfg.JobRetention))
		}
	}
}

// Stop cancels the engine and waits for all goroutines to exit. Safe to call
// even if Start was never called.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}
