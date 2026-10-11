import type { Job, QueueInfo, Stats, Status } from "./types";

async function getJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body && typeof body.error === "string") message = body.error;
    } catch {
      // Non-JSON error body; keep the status-line message.
    }
    throw new Error(message);
  }
  return (await res.json()) as T;
}

export function listQueues(): Promise<{ queues: QueueInfo[] }> {
  return getJSON("/v1/queues");
}

export function queueStats(queue: string): Promise<Stats> {
  return getJSON(`/v1/queues/${encodeURIComponent(queue)}/stats`);
}

export interface JobQuery {
  queue?: string;
  status?: Status;
  limit?: number;
}

export function listJobs(params: JobQuery): Promise<{ jobs: Job[]; count: number }> {
  const query = new URLSearchParams();
  if (params.queue) query.set("queue", params.queue);
  if (params.status) query.set("status", params.status);
  if (params.limit) query.set("limit", String(params.limit));
  const qs = query.toString();
  return getJSON(`/v1/jobs${qs ? `?${qs}` : ""}`);
}

export function requeueJob(id: string): Promise<Job> {
  return getJSON(`/v1/jobs/${encodeURIComponent(id)}/requeue`, { method: "POST" });
}
