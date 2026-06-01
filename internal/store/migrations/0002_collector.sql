-- Tracked repositories.
CREATE TABLE repos (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    github_id       INTEGER NOT NULL UNIQUE,
    full_name       TEXT    NOT NULL UNIQUE,   -- "owner/name"
    is_private      INTEGER NOT NULL DEFAULT 0,
    default_branch  TEXT    NOT NULL DEFAULT '',
    description     TEXT    NOT NULL DEFAULT '',
    stargazers      INTEGER NOT NULL DEFAULT 0,
    forks           INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Event tables (lean — source of truth).
CREATE TABLE commits (
    repo_id        INTEGER NOT NULL,
    sha            TEXT    NOT NULL,
    author_login   TEXT    NOT NULL DEFAULT '',
    committed_at   TIMESTAMP NOT NULL,
    additions      INTEGER NOT NULL DEFAULT 0,
    deletions      INTEGER NOT NULL DEFAULT 0,
    is_bot         INTEGER NOT NULL DEFAULT 0,
    msg_first_line TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, sha),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_commits_repo_date ON commits(repo_id, committed_at);

CREATE TABLE pull_requests (
    repo_id        INTEGER NOT NULL,
    number         INTEGER NOT NULL,
    author_login   TEXT    NOT NULL DEFAULT '',
    state          TEXT    NOT NULL,           -- 'OPEN' | 'CLOSED' | 'MERGED'
    created_at     TIMESTAMP NOT NULL,
    merged_at      TIMESTAMP,
    closed_at      TIMESTAMP,
    additions      INTEGER NOT NULL DEFAULT 0,
    deletions      INTEGER NOT NULL DEFAULT 0,
    changed_files  INTEGER NOT NULL DEFAULT 0,
    comments_count INTEGER NOT NULL DEFAULT 0,
    first_review_at TIMESTAMP,
    is_bot         INTEGER NOT NULL DEFAULT 0,
    title          TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, number),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_prs_repo_created ON pull_requests(repo_id, created_at);

CREATE TABLE issues (
    repo_id        INTEGER NOT NULL,
    number         INTEGER NOT NULL,
    author_login   TEXT    NOT NULL DEFAULT '',
    state          TEXT    NOT NULL,           -- 'OPEN' | 'CLOSED'
    created_at     TIMESTAMP NOT NULL,
    closed_at      TIMESTAMP,
    comments_count INTEGER NOT NULL DEFAULT 0,
    is_bot         INTEGER NOT NULL DEFAULT 0,
    title          TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, number),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);
CREATE INDEX idx_issues_repo_created ON issues(repo_id, created_at);

CREATE TABLE releases (
    repo_id        INTEGER NOT NULL,
    tag            TEXT    NOT NULL,
    name           TEXT    NOT NULL DEFAULT '',
    published_at   TIMESTAMP,
    author_login   TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (repo_id, tag),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

-- Materialized aggregates (charts read these).
CREATE TABLE daily_repo_stats (
    repo_id             INTEGER NOT NULL,
    date                TEXT    NOT NULL,       -- 'YYYY-MM-DD' (UTC)
    commits             INTEGER NOT NULL DEFAULT 0,
    additions           INTEGER NOT NULL DEFAULT 0,
    deletions           INTEGER NOT NULL DEFAULT 0,
    prs_opened          INTEGER NOT NULL DEFAULT 0,
    prs_merged          INTEGER NOT NULL DEFAULT 0,
    prs_closed          INTEGER NOT NULL DEFAULT 0,
    issues_opened       INTEGER NOT NULL DEFAULT 0,
    issues_closed       INTEGER NOT NULL DEFAULT 0,
    comments            INTEGER NOT NULL DEFAULT 0,
    releases            INTEGER NOT NULL DEFAULT 0,
    active_contributors INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (repo_id, date),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

CREATE TABLE daily_contributor_stats (
    repo_id    INTEGER NOT NULL,
    date       TEXT    NOT NULL,                -- 'YYYY-MM-DD' (UTC)
    login      TEXT    NOT NULL,
    commits    INTEGER NOT NULL DEFAULT 0,
    additions  INTEGER NOT NULL DEFAULT 0,
    deletions  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (repo_id, date, login),
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

-- Sync bookkeeping (cursors per repo). sync_jobs is intentionally deferred to M3.
CREATE TABLE sync_state (
    repo_id          INTEGER PRIMARY KEY,
    last_commit_at   TIMESTAMP,
    last_pr_cursor   TEXT NOT NULL DEFAULT '',
    last_issue_cursor TEXT NOT NULL DEFAULT '',
    last_commit_cursor TEXT NOT NULL DEFAULT '',
    last_release_cursor TEXT NOT NULL DEFAULT '',
    last_backfill_at TIMESTAMP,
    status           TEXT NOT NULL DEFAULT '',  -- '' | 'backfilling' | 'complete'
    FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
);

-- ETag cache for conditional REST GETs (304s are free; see spec §3).
CREATE TABLE etags (
    url           TEXT PRIMARY KEY,
    etag          TEXT NOT NULL,
    status        INTEGER NOT NULL,
    body          BLOB NOT NULL,
    last_modified TEXT NOT NULL DEFAULT '',
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
