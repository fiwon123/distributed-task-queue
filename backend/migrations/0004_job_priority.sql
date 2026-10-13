-- 0004_job_priority.sql — claim priority.
--
-- A producer can assign a numeric priority (default 0, higher = more urgent);
-- claims prefer higher priority, with run_at breaking ties. The old claimable
-- index (queue, run_at) is replaced by (queue, priority DESC, run_at) so the
-- new ordering stays index-backed.

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