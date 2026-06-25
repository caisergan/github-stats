-- GitHub's true total commit count on the default branch (history.totalCount),
-- refreshed on every metadata fetch. 0 until the repo's first sync populates it.
ALTER TABLE repos ADD COLUMN commit_count INTEGER NOT NULL DEFAULT 0;
