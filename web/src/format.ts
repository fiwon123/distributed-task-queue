import type { Job, Status } from "./types";

export const STATUSES: Status[] = ["pending", "running", "completed", "failed", "dead"];

// REQUEUEABLE states: only these can be sent back to the queue by the API.
export function isRequeueable(status: Status): boolean {
  return status === "dead" || status === "failed";
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
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
