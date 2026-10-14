import { useState, type FormEvent } from "react";
import { enqueueJob } from "./api";
import { errorMessage } from "./format";
import type { EnqueueRequest, Job } from "./types";

// SUGGESTED_TYPES are the handlers registered by backend/cmd/worker, offered as
// autocomplete hints; the API accepts any type string.
const SUGGESTED_TYPES = ["echo", "fail", "sleep"];

// payloadIssue returns a message when the payload text is non-empty but cannot
// be parsed as JSON, and null when it is valid or empty (empty means "omit",
// which the API stores as {}). It is pure so the live and submit paths share
// exactly one validation rule.
function payloadIssue(value: string): string | null {
  if (value.trim() === "") return null;
  try {
    JSON.parse(value);
    return null;
  } catch {
    return "Payload must be valid JSON.";
  }
}

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
  const [priority, setPriority] = useState("");
  const [payload, setPayload] = useState("{}");
  const [runAt, setRunAt] = useState("");
  const [maxAttempts, setMaxAttempts] = useState("3");
  const [timeoutSeconds, setTimeoutSeconds] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Live payload validity, recomputed on every render (i.e. every keystroke),
  // so the field can flag a malformed payload before the user submits.
  const payloadError = payloadIssue(payload);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setValidationError(null);

    const trimmedType = type.trim();
    if (trimmedType === "") {
      setValidationError("Type is required.");
      return;
    }

    // The inline message under the field already explains a bad payload, so
    // stop here rather than stacking a second banner on top of it.
    if (payloadError !== null) {
      return;
    }

    let parsedPayload: unknown;
    if (payload.trim() !== "") {
      parsedPayload = JSON.parse(payload);
    }

    let attempts: number | undefined;
    if (maxAttempts.trim() !== "") {
      attempts = Number(maxAttempts);
      if (!Number.isInteger(attempts) || attempts < 1) {
        setValidationError("Max attempts must be a whole number of at least 1.");
        return;
      }
    }

    let priorityValue: number | undefined;
    if (priority.trim() !== "") {
      priorityValue = Number(priority);
      if (!Number.isInteger(priorityValue) || priorityValue < 0) {
        setValidationError("Priority must be a whole number of at least 0.");
        return;
      }
      // Mirrors the server: the column is int4, larger values would 500.
      if (priorityValue > 2147483647) {
        setValidationError("Priority must be at most 2147483647.");
        return;
      }
    }

    let timeout: number | undefined;
    if (timeoutSeconds.trim() !== "") {
      timeout = Number(timeoutSeconds);
      if (!Number.isInteger(timeout) || timeout < 0) {
        setValidationError("Timeout must be a whole number of seconds (0 for none).");
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
    if (priorityValue !== undefined) req.priority = priorityValue;
    if (runAtISO !== undefined) req.run_at = runAtISO;
    if (attempts !== undefined) req.max_attempts = attempts;
    if (timeout !== undefined) req.timeout_seconds = timeout;

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
          Priority
          <input
            type="number"
            min={0}
            placeholder="0"
            value={priority}
            onChange={(e) => setPriority(e.target.value)}
          />
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
          Timeout (s)
          <input
            type="number"
            min={0}
            placeholder="none"
            value={timeoutSeconds}
            onChange={(e) => setTimeoutSeconds(e.target.value)}
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
          aria-invalid={payloadError !== null}
          aria-describedby={payloadError !== null ? "payload-error" : undefined}
        />
      </label>
      {payloadError !== null && (
        <p id="payload-error" className="field-error" role="alert">
          {payloadError}
        </p>
      )}
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
