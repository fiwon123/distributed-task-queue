import { GitHubIcon } from "./icons";
import { DOCS_PATH, REPO_URL } from "./links";
import Logo from "./Logo";
import { AppLink } from "./router";

// About page: why distributed task queues matter, how this implementation works,
// and the conceptual pillars that make it durable and recoverable. No API calls.
// Pure presentation that matches the project's design system and works in dark mode.

const STEPS: { title: string; body: string }[] = [
  {
    title: "Enqueue a job",
    body: "POST /v1/jobs with a type and a JSON payload. Set run_at to schedule it for the future and max_attempts to bound retries (default 3). The job lands in a queue and becomes pending.",
  },
  {
    title: "A worker claims it",
    body: "Workers poll the jobs table and claim due rows inside one transaction using FOR UPDATE SKIP LOCKED, so many workers run concurrently without ever claiming the same job.",
  },
  {
    title: "The handler runs",
    body: "The worker dispatches on the job type. On success the job is marked completed with a timestamp; on error it is marked failed and the error is stored as last_error.",
  },
  {
    title: "Failures retry with backoff",
    body: "A failed attempt is rescheduled by moving run_at further out (exponential backoff), so a transient failure is retried later rather than instantly.",
  },
  {
    title: "Leases recover crashes",
    body: "A claimed job is hidden behind a lease (locked_until). The worker renews its lease while running (heartbeat), so long-running jobs stay claimed; if the worker dies or stops renewing, the lease expires and the job becomes claimable again.",
  },
  {
    title: "Dead-letter and requeue",
    body: "When attempts reach max_attempts the job moves to the dead state instead of looping forever. Inspect it in the console, then POST /v1/jobs/{id}/requeue to send it back to pending.",
  },
];

const CONCEPTS: {
  title: string;
  body: string;
  items?: { label: string; text: string }[];
}[] = [
  {
    title: "Durability",
    body: "PostgreSQL is the single source of truth for job state. The queue is a table, not an in-memory list — so job records survive process restarts and crashes.",
    items: [
      {
        label: "Persistence",
        text: "All job state lives in the jobs table (status, attempts, run_at, locked_until, timestamps, last_error).",
      },
      {
        label: "Recoverability",
        text: "A worker that dies mid-job leaves no orphaned in-memory work; the lease simply expires.",
      },
    ],
  },
  {
    title: "At-least-once delivery & idempotency",
    body: "This queue provides at-least-once, not exactly-once. A worker can complete a handler's side effect but crash before writing the final status update — that job will be reclaimed later and run again.",
    items: [
      {
        label: "Idempotency required",
        text: "Handlers must be safe to run multiple times. Design for re-execution (e.g. checks, dedup keys, or idempotent operations).",
      },
      {
        label: "Why not exactly-once",
        text: "Exactly-once is hard to guarantee in general without a distributed transaction between the job execution and the queue; at-least-once is the practical, correct tradeoff here.",
      },
    ],
  },
  {
    title: "Atomic claims with FOR UPDATE SKIP LOCKED",
    body: "Many workers poll concurrently and must not claim the same job. Using FOR UPDATE SKIP LOCKED in a short transaction means each worker atomically claims a distinct, due job and sets its lease.",
    items: [
      {
        label: "No double-claim",
        text: "SKIP LOCKED lets workers skip rows already locked by others, so concurrent workers stay safe and efficient.",
      },
      {
        label: "Why not SELECT then UPDATE",
        text: "A non-transactional read-modify-write creates a race (two workers can both read the same due job). Claims must happen in one transaction.",
      },
    ],
  },
  {
    title: "Leases & crash recovery",
    body: "Each claimed job is hidden behind a lease (locked_until). The worker holding it renews the lease while the handler runs (heartbeat) so long-running jobs are not reclaimed mid-execution. If the worker dies or stops renewing, the lease expires and the job becomes claimable again.",
  },
  {
    title: "Retries, exponential backoff & bounded attempts",
    body: "Transient failures shouldn't be retried in a tight loop. Failed attempts reschedule the job by pushing run_at further out (exponential backoff), with a configurable max_attempts. Jobs that exhaust max_attempts move to dead rather than retrying forever.",
    items: [
      {
        label: "Capped attempts",
        text: "max_attempts bounds resource consumption and prevents poison jobs from looping indefinitely.",
      },
      {
        label: "Backoff spreads load",
        text: "Exponential backoff reduces retry pressure on downstream systems during outages.",
      },
    ],
  },
  {
    title: "Dead-letter queue & requeue",
    body: "Jobs that have exhausted their attempts land in the dead state. They remain in the table for inspection (last_error, attempts, timestamps) and can be requeued manually (POST /v1/jobs/{id}/requeue) back to pending when the underlying issue is resolved.",
  },
  {
    title: "Priority & scheduling",
    body: "Each job has a priority (default 0). Claims prefer higher priority jobs first, with run_at breaking ties. run_at also enables future scheduling and backoff-driven delays.",
  },
];

const CHARACTERISTICS: { label: string; text: string }[] = [
  {
    label: "Scalability",
    text: "Add more worker processes/instances; FOR UPDATE SKIP LOCKED lets them pull work in parallel without coordination beyond Postgres.",
  },
  {
    label: "Fault tolerance",
    text: "Leases + heartbeat ensure abandoned work is reclaimed after a crash; state persists in Postgres.",
  },
  {
    label: "Load balancing",
    text: "Workers compete for due jobs atomically; no single dispatcher bottleneck in the table-backed design.",
  },
  {
    label: "Asynchronous execution",
    text: "Producers enqueue and return immediately; workers execute jobs independently in the background.",
  },
  {
    label: "Persistence",
    text: "Jobs table is durable; the queue outlives API and worker processes.",
  },
  {
    label: "Retry semantics",
    text: "Failures trigger exponential backoff with bounded attempts; dead-letter captures terminal failures.",
  },
  {
    label: "Observability",
    text: "API exposes per-job state, per-queue stats, and a console for live inspection; OpenAPI + Swagger UI document the contract.",
  },
  {
    label: "Priority",
    text: "Claim order respects priority (desc), then run_at to minimize latency for urgent work.",
  },
  {
    label: "Decoupling",
    text: "Producers and consumers only share the API/DB contract; they can scale and deploy independently.",
  },
];

export default function About() {
  return (
    <section aria-label="about" className="about">
      <div className="about-lead">
        <Logo size={40} />
        <div>
          <h2>Concepts &amp; how it works</h2>
          <p className="lede">
            A durable, PostgreSQL-backed distributed task queue. The queue <em>is</em> a table: job
            state outlives the workers, so a crashed process never loses work.
          </p>
        </div>
      </div>

      <h3>Why distributed task queues?</h3>
      <p className="muted">
        A single-process task runner loses in-flight work when the process crashes and cannot spread
        load across machines. A distributed task queue decouples <strong>producers</strong> (which
        enqueue work) from <strong>workers</strong> (which execute it) behind a durable shared
        queue. This makes work resilient to failures, easy to scale horizontally, and observable
        over time.
      </p>

      <div
        className="pipeline"
        role="img"
        aria-label="Request path: client to HTTP API to PostgreSQL, with a worker pool"
      >
        <span className="step-name">Client</span>
        <span className="sep">-</span>
        <span className="step-name">HTTP API</span>
        <span className="sep">-</span>
        <span className="step-name">PostgreSQL</span>
        <span className="sep">↔</span>
        <span className="step-name">Worker pool</span>
      </div>

      <h3>How it works</h3>
      <ol className="steps">
        {STEPS.map((step) => (
          <li key={step.title}>
            <h3>{step.title}</h3>
            <p>{step.body}</p>
          </li>
        ))}
      </ol>

      <h3>Core concepts</h3>
      <div className="concepts-grid">
        {CONCEPTS.map((concept) => (
          <article className="concept-card" key={concept.title}>
            <h4>{concept.title}</h4>
            <p>{concept.body}</p>
            {concept.items && (
              <ul className="concept-list">
                {concept.items.map((item) => (
                  <li key={item.label}>
                    <strong>{item.label}:</strong> {item.text}
                  </li>
                ))}
              </ul>
            )}
          </article>
        ))}
      </div>

      <h3>Job lifecycle (state machine)</h3>
      <p className="muted">
        Jobs transition through states as they are claimed, executed, retried, or dead-lettered. A
        job can be requeued from <code>dead</code> (or <code>failed</code> in the API sense after
        inspection) back to <code>pending</code>.
      </p>
      <div
        className="state-diagram"
        role="img"
        aria-label="Job state machine: pending to running to completed or failed; failed to pending on retry; failed to dead when attempts exhausted; dead to pending on requeue"
      >
        <div className="state-row">
          <span className="state">pending</span>
          <span className="arrow">-</span>
          <span className="state">running</span>
          <span className="arrow">-</span>
          <span className="state success">completed</span>
        </div>
        <div className="state-row">
          <span className="state">running</span>
          <span className="arrow">-</span>
          <span className="state warn">failed</span>
          <span className="arrow">↻</span>
          <span className="state">pending</span>
          <span className="arrow">-</span>
          <span className="state">running</span>
          <span className="arrow">-</span>
          <span className="state danger">dead</span>
        </div>
        <div className="state-row">
          <span className="state danger">dead</span>
          <span className="arrow">-</span>
          <span className="state">pending</span>
          <span className="arrow">(requeue)</span>
        </div>
      </div>

      <h3>Key characteristics</h3>
      <div className="table-wrap">
        <table className="kv-table">
          <thead>
            <tr>
              <th>Characteristic</th>
              <th>How this implementation delivers it</th>
            </tr>
          </thead>
          <tbody>
            {CHARACTERISTICS.map((c) => (
              <tr key={c.label}>
                <td>{c.label}</td>
                <td>{c.text}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <h3>What this project deliberately is (and isn't)</h3>
      <p className="muted">
        This is an MVP focused on the <em>database-backed queue itself</em>. It's intentionally
        simple and educational: <strong>Postgres as the queue</strong> (no separate broker in
        scope),
        <strong> leases + heartbeat</strong> for crash recovery,{" "}
        <strong>bounded retries + backoff</strong>, and a minimal HTTP console. It does not aim for
        exactly-once delivery, global ordering across independent jobs, built-in auth/TLS, or a
        separate result store.
      </p>

      <h3>Using the console</h3>
      <p className="muted">
        The <strong>Jobs</strong> tab lists queues and jobs, enqueues new work, and exposes actions
        for dead-lettered jobs. Click a job id to watch it live while it is pending or running. The
        <strong> Workbench</strong> tab drives the same API straight from the served OpenAPI spec,
        and the Swagger UI at <span className="mono">/docs</span> documents every endpoint.
      </p>

      <p className="muted">
        Delivery is at-least-once: a worker can finish a job's side effect before its status update
        is written, so handlers must be idempotent. There is no exactly-once guarantee.
      </p>

      <div className="about-actions">
        <a className="button-link primary" href={REPO_URL} target="_blank" rel="noreferrer">
          <GitHubIcon /> View source
        </a>
        <a className="button-link" href={DOCS_PATH}>
          API docs
        </a>
        <AppLink to="/workbench" className="button-link">
          Open the workbench
        </AppLink>
      </div>
    </section>
  );
}
