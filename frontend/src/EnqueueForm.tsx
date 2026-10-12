import { useState, type FormEvent } from "react";
import { enqueueJob } from "./api";
import { errorMessage } from "./format";
import type { EnqueueRequest, Job } from "./types";

// SUGGESTED_TYPES are the handlers registered by backend/cmd/worker, offered as
// autocomplete hints; the API accepts any type string.
const SUGGESTED_TYPES = ["echo", "fail", "sleep"];

// EnqueueForm posts a new job to POST /v1/jobs. On success it resets the
// payload/schedule fields and reports the created job to its parent; validation
// and API errors are surfaced inline and via onError respectively.
export default function EnqueueForm({
  onEnqueued,
  onError,
}: {
  onEnqueued: (job: Job) => void;
  onError: (message: string) => void;
}) {
  const [type, setType] = useState("echo");
  const [queue, setQueue] = useState("");
  const [payload, setPayload] = useState("{}");
  const [runAt, setRunAt] = useState("");
  const [maxAttempts, setMaxAttempts] = useState("3");
  const [validationError, setValidationError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setValidationError(null);

    const trimmedType = type.trim();
    if (trimmedType === "") {
      setValidationError("Type is required.");
      return;
    }

    let parsedPayload: unknown;
    if (payload.trim() !== "") {
      try {
        parsedPayload = JSON.parse(payload);
      } catch {
        setValidationError("Payload must be valid JSON.");
        return;
      }
    }

    let attempts: number | undefined;
    if (maxAttempts.trim() !== "") {
      attempts = Number(maxAttempts);
      if (!Number.isInteger(attempts) || attempts < 1) {
        setValidationError("Max attempts must be a whole number of at least 1.");
        return;
      }
    }

    let runAtISO: string | undefined;
    if (runAt.trim() !== "") {
      const when = new Date(runAt);
      if (Number.isNaN(when.getTime())) {
        setValidationError("Run at must be a valid date and time.");
        return;
      }
      runAtISO = when.toISOString();
    }

    const req: EnqueueRequest = { type: trimmedType };
    if (parsedPayload !== undefined) req.payload = parsedPayload;
    if (queue.trim() !== "") req.queue = queue.trim();
    if (runAtISO !== undefined) req.run_at = runAtISO;
    if (attempts !== undefined) req.max_attempts = attempts;

    setBusy(true);
    try {
      const job = await enqueueJob(req);
      setPayload("{}");
      setRunAt("");
      onEnqueued(job);
    } catch (err) {
      onError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="enqueue" aria-label="enqueue a job" onSubmit={(e) => void onSubmit(e)}>
      <div className="form-row">
        <label>
          Type
          <input list="job-types" value={type} onChange={(e) => setType(e.target.value)} required />
        </label>
        <datalist id="job-types">
          {SUGGESTED_TYPES.map((t) => (
            <option key={t} value={t} />
          ))}
        </datalist>
        <label>
          Queue
          <input value={queue} placeholder="default" onChange={(e) => setQueue(e.target.value)} />
        </label>
        <label>
          Max attempts
          <input
            type="number"
            min={1}
            value={maxAttempts}
            onChange={(e) => setMaxAttempts(e.target.value)}
          />
        </label>
        <label>
          Run at
          <input type="datetime-local" value={runAt} onChange={(e) => setRunAt(e.target.value)} />
        </label>
      </div>
      <label className="payload-field">
        Payload (JSON)
        <textarea
          rows={3}
          spellCheck={false}
          value={payload}
          onChange={(e) => setPayload(e.target.value)}
        />
      </label>
      {validationError && (
        <p className="banner error" role="alert">
          {validationError}
        </p>
      )}
      <button type="submit" className="primary" disabled={busy}>
        {busy ? "Enqueuing…" : "Enqueue job"}
      </button>
    </form>
  );
}
