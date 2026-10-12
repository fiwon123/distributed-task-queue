import { useCallback, useEffect, useState } from "react";
import { listJobs, listQueues, queueStats, requeueJob } from "./api";
import { STATUSES, errorMessage, isRequeueable, jobSummary, shortID } from "./format";
import type { Job, QueueInfo, Stats, Status } from "./types";

export default function App() {
  const [queues, setQueues] = useState<QueueInfo[]>([]);
  const [selected, setSelected] = useState("");
  const [statusFilter, setStatusFilter] = useState<Status | "">("");
  const [stats, setStats] = useState<Stats | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

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

  async function onRequeue(job: Job) {
    setNotice(null);
    setError(null);
    try {
      await requeueJob(job.id);
      setNotice(`Requeued ${shortID(job.id)}`);
      await Promise.all([loadQueue(), loadQueues()]);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function refresh() {
    setNotice(null);
    await Promise.all([loadQueues(), loadQueue()]);
  }

  return (
    <main className="app">
      <header>
        <h1>distributed-task-queue</h1>
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
        </div>
      </header>

      {error && (
        <p className="banner error" role="alert">
          {error}
        </p>
      )}
      {notice && (
        <p className="banner notice" role="status">
          {notice}
        </p>
      )}

      <section aria-label="queue statistics" className="stats">
        {stats &&
          STATUSES.map((s) => (
            <div key={s} className={`card status-${s}`}>
              <span className="card-count">{stats.counts[s] ?? 0}</span>
              <span className="card-label">{s}</span>
            </div>
          ))}
      </section>

      <section aria-label="jobs">
        <h2>
          Jobs <span className="muted">({jobs.length})</span>
        </h2>
        {jobs.length === 0 ? (
          <p className="muted">No jobs match this filter.</p>
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
                    {shortID(job.id)}
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
    </main>
  );
}
