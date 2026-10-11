-- 0001_jobs.sql — the jobs table: the queue itself.
--
-- Postgres is the single source of truth for job state. A job row moves
-- pending -> running -> completed | failed | dead; workers claim due rows with
-- FOR UPDATE SKIP LOCKED and hold them behind a lease (locked_until). This
-- migration creates the table and the indexes the later phases claim with.

CREATE TABLE IF NOT EXISTS jobs (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    type          text        NOT NULL,
    payload       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    queue         text        NOT NULL DEFAULT 'default',
    status        text        NOT NULL DEFAULT 'pending',
    attempts      integer     NOT NULL DEFAULT 0,
    max_attempts  integer     NOT NULL DEFAULT 3,
    run_at        timestamptz NOT NULL DEFAULT now(),
    locked_until  timestamptz,
    last_error    text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    completed_at  timestamptz,

    CONSTRAINT jobs_status_check
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'dead')),
    CONSTRAINT jobs_attempts_check     CHECK (attempts >= 0),
    CONSTRAINT jobs_max_attempts_check CHECK (max_attempts >= 1)
);

-- Claiming scans for due, claimable jobs within a queue. The partial index
-- keeps it small: only rows a worker could actually pick up are indexed.
CREATE INDEX IF NOT EXISTS jobs_claimable_idx
    ON jobs (queue, run_at)
    WHERE status IN ('pending', 'failed');

-- Per-queue stats group by queue and filter by status.
CREATE INDEX IF NOT EXISTS jobs_queue_status_idx
    ON jobs (queue, status);
