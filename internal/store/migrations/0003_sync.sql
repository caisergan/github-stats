-- Durable job queue for the sync engine (spec §5/§6).
CREATE TABLE sync_jobs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id      INTEGER NOT NULL,
    kind         TEXT    NOT NULL,              -- 'backfill' | 'delta'
    status       TEXT    NOT NULL DEFAULT 'pending', -- 'pending' | 'running' | 'done' | 'error'
    attempts     INTEGER NOT NULL DEFAULT 0,
    next_run_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at    TIMESTAMP,                     -- NULL when not leased
    last_error   TEXT    NOT NULL DEFAULT '',
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_sync_jobs_runnable ON sync_jobs(status, next_run_at);
CREATE INDEX idx_sync_jobs_repo ON sync_jobs(repo_id);

-- Per-user repo tracking (spec §5). A repo row is shared; tracking is per-user.
CREATE TABLE repo_tracking (
    user_id    INTEGER NOT NULL,
    repo_id    INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, repo_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_repo_tracking_user ON repo_tracking(user_id);
