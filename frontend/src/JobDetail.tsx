import { useState } from "react";
import { requeueJob } from "./api";
import { errorMessage, isRequeueable, isTerminal, prettyJSON, shortID } from "./format";
import { AppLink } from "./router";
import type { Job } from "./types";
import { useJobPolling } from "./useJobPolling";

// JobDetail shows one job's full record and, while the job is non-terminal,
// keeps it fresh by polling (see useJobPolling). pollIntervalMs is exposed for
// tests; callers normally leave it unset.
export default function JobDetail({ id, pollIntervalMs }: { id: string; pollIntervalMs?: number }) {
  const { job, error, loading, refresh } = useJobPolling(id, pollIntervalMs);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onRequeue(current: Job) {
    setBusy(true);
    setActionError(null);
    try {
      await requeueJob(current.id);
      refresh();
    } catch (err) {
      setActionError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const banner = error ?? actionError;

  return (
    <section aria-label="job detail">
      <p>
        <AppLink to="/">← All jobs</AppLink>
      </p>
      {banner && (
        <p className="banner error" role="alert">
          {banner}
        </p>
      )}
      {loading && job === null && <p className="muted">Loading…</p>}
      {job !== null && (
        <>
          <h2>
            Job <span className="mono">{shortID(job.id)}</span>{" "}
            <span className={`badge status-${job.status}`}>{job.status}</span>
          </h2>
          {!isTerminal(job.status) && (
            <p className="muted" role="status">
              Live — refreshing…
            </p>
          )}
          <dl className="detail">
            <dt>ID</dt>
            <dd className="mono">{job.id}</dd>
            <dt>Type</dt>
            <dd>{job.type}</dd>
            <dt>Queue</dt>
            <dd>{job.queue}</dd>
            <dt>Attempts</dt>
            <dd>
              {job.attempts} / {job.max_attempts}
            </dd>
            <dt>Run at</dt>
            <dd className="mono">{job.run_at}</dd>
            <dt>Lease (locked until)</dt>
            <dd className="mono">{job.locked_until ?? "—"}</dd>
            <dt>Created</dt>
            <dd className="mono">{job.created_at}</dd>
            <dt>Completed</dt>
            <dd className="mono">{job.completed_at ?? "—"}</dd>
            <dt>Last error</dt>
            <dd className="error-cell" title={job.last_error ?? ""}>
              {job.last_error ?? "—"}
            </dd>
          </dl>
          <h3>Payload</h3>
          <pre className="payload mono">{prettyJSON(job.payload)}</pre>
          {isRequeueable(job.status) && (
            <button type="button" onClick={() => void onRequeue(job)} disabled={busy}>
              Requeue
            </button>
          )}
        </>
      )}
    </section>
  );
}
