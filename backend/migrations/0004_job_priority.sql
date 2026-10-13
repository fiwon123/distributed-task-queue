-- 0004_job_priority.sql — claim priority.
--
-- A producer can assign a numeric priority (default 0, higher = more urgent);
-- claims prefer higher priority, with run_at breaking ties. The old claimable
-- index (queue, run_at) is replaced by (queue, priority DESC, run_at) so the
-- new ordering stays index-backed.
--
-- Lock note: the CHECK and the non-concurrent index build both take heavy
-- locks (the CHECK scans the table; CREATE INDEX blocks writes) while the api
-- and worker run this migration at startup, so a non-empty table briefly stops
-- enqueuing/claiming. CREATE INDEX CONCURRENTLY cannot be used here because
-- migrations run inside a transaction.

ALTER TABLE jobs
    ADD COLUMN priority integer NOT NULL DEFAULT 0;

ALTER TABLE jobs
    ADD CONSTRAINT jobs_priority_check CHECK (priority >= 0);

-- Claims now scan (queue, priority DESC, run_at); the pre-priority index would
-- not serve the sort direction, so the partial index is rebuilt around it.
DROP INDEX IF EXISTS jobs_claimable_idx;

CREATE INDEX IF NOT EXISTS jobs_claimable_idx
    ON jobs (queue, priority DESC, run_at)
    WHERE status IN ('pending', 'failed');