package store

import (
	"context"
	"database/sql"
)

// RecomputeDailyStats rebuilds daily_repo_stats and daily_contributor_stats for
// the inclusive UTC date window [fromDate, toDate] (each "YYYY-MM-DD") from the
// event tables, which are the source of truth. It deletes existing aggregate
// rows in the window first, so the operation is idempotent and corrects drift
// from late edits (a PR merging, an issue closing) — see spec §5/§6/§12.
//
// Each metric is attributed to the UTC date of the relevant event timestamp:
// commits → committed_at; prs_opened/issues_opened → created_at;
// prs_merged → merged_at; prs_closed/issues_closed → closed_at;
// comments → PR/issue created_at; releases → published_at.
func (s *Store) RecomputeDailyStats(ctx context.Context, repoID int64, fromDate, toDate string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		// Clear the window so stale rows for now-empty days disappear.
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM daily_repo_stats WHERE repo_id = ? AND date >= ? AND date <= ?`,
			repoID, fromDate, toDate); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM daily_contributor_stats WHERE repo_id = ? AND date >= ? AND date <= ?`,
			repoID, fromDate, toDate); err != nil {
			return err
		}

		// Build per-date repo aggregates by unioning each metric source, then
		// summing into daily_repo_stats. Timestamps are stored by modernc sqlite in
		// time.Time.String() form ("YYYY-MM-DD HH:MM:SS +0000 UTC"), which SQLite's
		// date() cannot parse, so substr(col,1,10) extracts the 'YYYY-MM-DD' UTC day.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO daily_repo_stats (
				repo_id, date, commits, additions, deletions,
				prs_opened, prs_merged, prs_closed,
				issues_opened, issues_closed, comments, releases, active_contributors)
			SELECT ?1, day,
				SUM(commits), SUM(additions), SUM(deletions),
				SUM(prs_opened), SUM(prs_merged), SUM(prs_closed),
				SUM(issues_opened), SUM(issues_closed), SUM(comments), SUM(releases),
				MAX(active_contributors)
			FROM (
				-- Commits and per-day distinct contributor count.
				SELECT substr(committed_at, 1, 10) AS day,
					COUNT(*) AS commits, SUM(additions) AS additions, SUM(deletions) AS deletions,
					0 AS prs_opened, 0 AS prs_merged, 0 AS prs_closed,
					0 AS issues_opened, 0 AS issues_closed, 0 AS comments, 0 AS releases,
					COUNT(DISTINCT author_login) AS active_contributors
				FROM commits WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT substr(created_at, 1, 10) AS day,
					0,0,0, COUNT(*), 0, 0, 0,0,0,0, 0
				FROM pull_requests WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT substr(merged_at, 1, 10) AS day,
					0,0,0, 0, COUNT(*), 0, 0,0,0,0, 0
				FROM pull_requests WHERE repo_id = ?1 AND merged_at IS NOT NULL GROUP BY day
				UNION ALL
				SELECT substr(closed_at, 1, 10) AS day,
					0,0,0, 0, 0, COUNT(*), 0,0,0,0, 0
				FROM pull_requests WHERE repo_id = ?1 AND closed_at IS NOT NULL GROUP BY day
				UNION ALL
				SELECT substr(created_at, 1, 10) AS day,
					0,0,0, 0,0,0, COUNT(*), 0, 0,0, 0
				FROM issues WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT substr(closed_at, 1, 10) AS day,
					0,0,0, 0,0,0, 0, COUNT(*), 0,0, 0
				FROM issues WHERE repo_id = ?1 AND closed_at IS NOT NULL GROUP BY day
				UNION ALL
				SELECT substr(created_at, 1, 10) AS day,
					0,0,0, 0,0,0, 0,0, SUM(comments_count), 0, 0
				FROM pull_requests WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT substr(created_at, 1, 10) AS day,
					0,0,0, 0,0,0, 0,0, SUM(comments_count), 0, 0
				FROM issues WHERE repo_id = ?1 GROUP BY day
				UNION ALL
				SELECT substr(published_at, 1, 10) AS day,
					0,0,0, 0,0,0, 0,0,0, COUNT(*), 0
				FROM releases WHERE repo_id = ?1 AND published_at IS NOT NULL GROUP BY day
			)
			WHERE day IS NOT NULL AND day >= ?2 AND day <= ?3
			GROUP BY day`,
			repoID, fromDate, toDate); err != nil {
			return err
		}

		// Per-contributor commit aggregates.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO daily_contributor_stats (repo_id, date, login, commits, additions, deletions)
			SELECT ?1, substr(committed_at, 1, 10) AS day, author_login,
				COUNT(*), SUM(additions), SUM(deletions)
			FROM commits
			WHERE repo_id = ?1 AND substr(committed_at, 1, 10) >= ?2 AND substr(committed_at, 1, 10) <= ?3
			GROUP BY day, author_login`,
			repoID, fromDate, toDate); err != nil {
			return err
		}
		return nil
	})
}
