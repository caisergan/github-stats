-- Track the last successful delta sync per repo. The scheduler throttles delta
-- cadence off the freshest of last_backfill_at / last_delta_at; without this the
-- one-time last_backfill_at could never re-arm the cadence guard after a repo's
-- first 30 minutes, so deltas would re-enqueue on every scheduler tick.
ALTER TABLE sync_state ADD COLUMN last_delta_at TIMESTAMP;
