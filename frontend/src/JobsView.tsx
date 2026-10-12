import { useCallback, useEffect, useState, type ReactNode } from "react";
import { listJobs, listQueues, queueStats, requeueDeadJobs, requeueJob } from "./api";
import EnqueueForm from "./EnqueueForm";
import { STATUSES, errorMessage, isRequeueable, jobSummary, shortID } from "./format";
import QueueComposition from "./QueueComposition";
import { AppLink } from "./router";
import Toast from "./Toast";
import type { Job, QueueInfo, Stats, Status } from "./types";

// JobsView is the console: a queue/status filter over the job list, per-queue
// stats, an enqueue form, and a Requeue action for dead-lettered jobs.
export default function JobsView() {
  const [queues, setQueues] = useState<QueueInfo[]>([]);
  const [selected, setSelected] = useState("");
  const [statusFilter, setStatusFilter] = useState<Status | "">("");
  const [stats, setStats] = useState<Stats | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [error, setError] = useState<string | null>(null);
  // Each notice carries an id so a new message remounts the toast (fresh
  // countdown) instead of mutating the one already on screen.
  const [notice, setNotice] = useState<{ id: number; node: ReactNode } | null>(null);
  const [busy, setBusy] = useState(false);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [showEnqueue, setShowEnqueue] = useState(false);

  const loadQueues = useCallback(async () => {
    try {
      const { queues: nextQueues } = await listQueues();
      setQueues(nextQueues);
      setSelected((current) => current || nextQueues[0]?.name || "");
    } catch (err) {
      setError(errorMessage(err));
    }
  }, []);

  const loadQueue = useCallback(async () => {
    if (!selected) {
      setStats(null);
      setJobs([]);
      return;
    }
    setBusy(true);
    try {
      const [nextStats, listed] = await Promise.all([
        queueStats(selected),
        listJobs({ queue: selected, status: statusFilter || undefined, limit: 50 }),
      ]);
      setStats(nextStats);
      setJobs(listed.jobs);
      setError(null);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }, [selected, statusFilter]);

  useEffect(() => {
    void loadQueues();
  }, [loadQueues]);

  useEffect(() => {
    void loadQueue();
  }, [loadQueue]);

  const showNotice = useCallback((node: ReactNode) => {
    setNotice((current) => ({ id: (current?.id ?? 0) + 1, node }));
  }, []);

  async function onRequeue(job: Job) {
    setNotice(null);
    setError(null);
    try {
      await requeueJob(job.id);
      showNotice(`Requeued ${shortID(job.id)}`);
      await Promise.all([loadQueue(), loadQueues()]);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function onRequeueDead() {
    setError(null);
    setBulkBusy(true);
    try {
      const { requeued } = await requeueDeadJobs(selected);
      showNotice(
        requeued === 0
          ? "No dead jobs to requeue"
          : `Requeued ${requeued} dead ${requeued === 1 ? "job" : "jobs"}`,
      );
      await Promise.all([loadQueue(), loadQueues()]);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBulkBusy(false);
    }
  }

  async function onEnqueued(job: Job) {
    setError(null);
    showNotice(
      <>
        Enqueued{" "}
        <AppLink to={`/jobs/${job.id}`}>
          <span className="mono">{shortID(job.id)}</span>
        </AppLink>
      </>,
    );
    await Promise.all([loadQueues(), loadQueue()]);
  }

  async function refresh() {
    setNotice(null);
    await Promise.all([loadQueues(), loadQueue()]);
  }

  const dismissNotice = useCallback(() => setNotice(null), []);

  return (
    <>
      <div className="controls">
        <label>
          Queue
          <select value={selected} onChange={(e) => setSelected(e.target.value)}>
            {queues.length === 0 && <option value="">(no queues)</option>}
            {queues.map((q) => (
              <option key={q.name} value={q.name}>
                {q.name} ({q.total})
              </option>
            ))}
          </select>
        </label>
        <label>
          Status
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as Status | "")}
          >
            <option value="">all</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
        <button type="button" onClick={() => void refresh()} disabled={busy}>
          {busy ? "Refreshing…" : "Refresh"}
        </button>
        <button
          type="button"
          className="primary"
          aria-expanded={showEnqueue}
          onClick={() => setShowEnqueue((shown) => !shown)}
        >
          {showEnqueue ? "Close" : "New job"}
        </button>
      </div>

      {error && (
        <p className="banner error" role="alert">
          {error}
        </p>
      )}
      {notice && <Toast key={notice.id} message={notice.node} onDismiss={dismissNotice} />}

      {showEnqueue && <EnqueueForm onEnqueued={onEnqueued} onError={setError} />}

      <section aria-label="queue statistics" className="stats">
        {stats &&
          STATUSES.map((s) => (
            <div key={s} className={`card status-${s}`}>
              <span className="card-count">{stats.counts[s] ?? 0}</span>
              <span className="card-label">{s}</span>
            </div>
          ))}
        {stats && <QueueComposition counts={stats.counts} />}
        {stats && (stats.counts.dead ?? 0) > 0 && (
          <button
            type="button"
            className="primary stats-action"
            onClick={() => void onRequeueDead()}
            disabled={bulkBusy}
          >
            {bulkBusy ? "Requeuing…" : "Requeue all dead"}
          </button>
        )}
      </section>

      <section aria-label="jobs">
        <h2>
          Jobs <span className="muted">({jobs.length})</span>
        </h2>
        {jobs.length === 0 ? (
          <div className="empty-state">
            <p>No jobs match this filter.</p>
            <p className="muted">Use New job to enqueue one, then refresh.</p>
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>ID</th>
                <th>Type</th>
                <th>Summary</th>
                <th>Status</th>
                <th>Attempts</th>
                <th>Last error</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => (
                <tr key={job.id}>
                  <td className="mono" title={job.id}>
                    <AppLink to={`/jobs/${job.id}`}>{shortID(job.id)}</AppLink>
                  </td>
                  <td>{job.type}</td>
                  <td className="summary" title={jobSummary(job)}>
                    {jobSummary(job)}
                  </td>
                  <td>
                    <span className={`badge status-${job.status}`}>{job.status}</span>
                  </td>
                  <td>
                    {job.attempts}/{job.max_attempts}
                  </td>
                  <td className="error-cell" title={job.last_error ?? ""}>
                    {job.last_error ?? "—"}
                  </td>
                  <td>
                    {isRequeueable(job.status) && (
                      <button type="button" onClick={() => void onRequeue(job)}>
                        Requeue
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}
