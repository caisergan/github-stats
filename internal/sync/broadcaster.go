// Package sync is the github-stats sync engine: a durable job queue worker
// pool, a periodic delta scheduler, the delta-sync routine, and a per-repo
// progress broadcaster for SSE. It orchestrates only — fetching is delegated to
// githubapi and persistence to store (spec §4/§6). The stdlib sync package is
// imported under the alias stdsync to avoid the package-name clash.
package sync

import stdsync "sync"

// Event is one progress update for a repo's sync.
type Event struct {
	RepoID  int64  `json:"repo_id"`
	Phase   string `json:"phase"`   // "backfill" | "delta" | "commits" | "prs" | "issues" | "releases" | "throttled" | "done" | "error"
	Message string `json:"message"` // human-readable detail
	Done    bool   `json:"done"`    // terminal event for this run
}

const subscriberBuffer = 32

// Broadcaster fans progress events out to per-repo subscribers.
type Broadcaster struct {
	mu   stdsync.Mutex
	subs map[int64]map[chan Event]struct{}
}

// NewBroadcaster builds an empty Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[int64]map[chan Event]struct{})}
}

// Subscribe returns a buffered channel of events for repoID plus a cancel func
// that unsubscribes and closes the channel. Cancel is idempotent.
func (b *Broadcaster) Subscribe(repoID int64) (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)
	b.mu.Lock()
	if b.subs[repoID] == nil {
		b.subs[repoID] = make(map[chan Event]struct{})
	}
	b.subs[repoID][ch] = struct{}{}
	b.mu.Unlock()

	var once stdsync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if set, ok := b.subs[repoID]; ok {
				if _, ok := set[ch]; ok {
					delete(set, ch)
					close(ch)
				}
				if len(set) == 0 {
					delete(b.subs, repoID)
				}
			}
			b.mu.Unlock()
		})
	}
	return ch, cancel
}

// PublishForTest exposes publish for cross-package tests (e.g. the api SSE
// handler test). Production code in this package calls publish directly.
func (b *Broadcaster) PublishForTest(repoID int64, ev Event) { b.publish(repoID, ev) }

// publish delivers ev to every subscriber of ev.RepoID. It never blocks. If a
// subscriber's buffer is full it drops the OLDEST queued event and enqueues ev,
// so a lagging subscriber keeps the most recent state — importantly the terminal
// Done event, which the SSE handler relies on to close the stream.
func (b *Broadcaster) publish(repoID int64, ev Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[repoID] {
		select {
		case ch <- ev:
		default:
			// Buffer full: evict one old event (non-blocking), then retry the
			// send (also non-blocking). Worst case under heavy contention we
			// still drop ev, but never stall the worker.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}
}
