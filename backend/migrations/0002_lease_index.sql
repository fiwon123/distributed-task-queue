-- 0002_lease_index.sql — index for reclaiming expired leases.
--
-- A worker that dies mid-job leaves the row in status 'running' until its
-- locked_until passes. Claim scans for exactly that, so index the lease expiry
-- of running rows.
CREATE INDEX IF NOT EXISTS jobs_lease_idx
    ON jobs (locked_until)
    WHERE status = 'running';
