import type { Job, Status } from "./types";

export const STATUSES: Status[] = ["pending", "running", "completed", "failed", "dead"];

// REQUEUEABLE states: only these can be sent back to the queue by the API.
export function isRequeueable(status: Status): boolean {
  return status === "dead" || status === "failed";
}

// isTerminal reports whether a job has reached a final state. Non-terminal jobs
// (pending, running) are still moving, so the console keeps polling them.
export function isTerminal(status: Status): boolean {
  return status === "completed" || status === "failed" || status === "dead";
}

// prettyJSON renders a value as indented JSON, falling back to String() for
// cycles or other values JSON cannot represent.
export function prettyJSON(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

// requeueCurl builds the copy-pasteable command that sends a job back to the
// queue. It is a pure function of the API origin and the job id so it can be
// unit-tested without a DOM.
export function requeueCurl(origin: string, id: string): string {
  return `curl -X POST ${origin}/v1/jobs/${id}/requeue`;
}

export function shortID(id: string): string {
  return id.slice(0, 8);
}

// jobSummary renders a one-line description from a job's payload, preferring
// common fields and falling back to compact JSON.
export function jobSummary(job: Job): string {
  const p = job.payload;
  if (p && typeof p === "object" && !Array.isArray(p)) {
    const obj = p as Record<string, unknown>;
    for (const key of ["name", "message", "to", "url", "id"]) {
      const v = obj[key];
      if (typeof v === "string" || typeof v === "number") return String(v);
    }
    const keys = Object.keys(obj);
    if (keys.length === 0) return "{}";
  }
  try {
    return JSON.stringify(p);
  } catch {
    return String(p);
  }
}
