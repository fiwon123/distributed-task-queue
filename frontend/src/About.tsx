import { GitHubIcon } from "./icons";
import { DOCS_PATH, REPO_URL } from "./links";
import Logo from "./Logo";
import { AppLink } from "./router";

// About is a static, offline how-to page: what the queue is, the request path,
// and the six-step lifecycle a job goes through. No API calls, so it renders
// even when the backend is unreachable.
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
    body: "A claimed job is hidden behind a lease (locked_until). If the worker dies mid-job, the lease expires and the job becomes claimable again — abandoned work is recovered automatically.",
  },
  {
    title: "Dead-letter and requeue",
    body: "When attempts reach max_attempts the job moves to the dead state instead of looping forever. Inspect it here, then POST /v1/jobs/{id}/requeue to send it back to pending.",
  },
];

export default function About() {
  return (
    <section aria-label="about" className="about">
      <div className="about-lead">
        <Logo size={40} />
        <div>
          <h2>About the queue</h2>
          <p className="lede">
            A durable, PostgreSQL-backed distributed task queue. The queue <em>is</em> a table: job
            state outlives the workers, so a crashed process never loses work.
          </p>
        </div>
      </div>

      <div
        className="pipeline"
        role="img"
        aria-label="Request path: client to HTTP API to PostgreSQL, with a worker pool"
      >
        <span className="step-name">Client</span>
        <span className="sep">→</span>
        <span className="step-name">HTTP API</span>
        <span className="sep">→</span>
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

      <h3>Using the console</h3>
      <p className="muted">
        The <strong>Jobs</strong> tab lists queues and jobs, enqueues new work, and exposes a
        Requeue action for dead-lettered jobs. Click a job id to watch it live while it is pending
        or running. The <strong>Workbench</strong> tab drives the same API straight from the served
        OpenAPI spec, and the Swagger UI at <span className="mono">/docs</span> documents every
        endpoint.
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
