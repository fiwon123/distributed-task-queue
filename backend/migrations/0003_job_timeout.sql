-- 0003_job_timeout.sql — per-job execution timeout.
--
-- timeout_seconds bounds a single handler run: the worker cancels the handler
-- when the deadline passes and records the attempt as failed, which retries or
-- dead-letters per the usual rules. NULL or 0 means no per-job timeout; the
-- lease/heartbeat still bounds how long a claim stays hidden.

ALTER TABLE jobs
    ADD COLUMN timeout_seconds integer;

ALTER TABLE jobs
    ADD CONSTRAINT jobs_timeout_seconds_check
        CHECK (timeout_seconds IS NULL OR timeout_seconds >= 0);
