CREATE TABLE users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    github_id   INTEGER NOT NULL UNIQUE,
    login       TEXT    NOT NULL,
    avatar_url  TEXT    NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE credentials (
    user_id    INTEGER NOT NULL,
    kind       TEXT    NOT NULL,             -- 'oauth' | 'pat'
    enc_token  TEXT    NOT NULL,
    scopes     TEXT    NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, kind),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE sessions (
    id         TEXT    PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_sessions_user ON sessions(user_id);
