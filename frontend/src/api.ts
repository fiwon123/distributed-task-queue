import type { Spec } from "./openapi";
import type { EnqueueRequest, Job, QueueInfo, Stats, Status } from "./types";

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
  type?: string;
  limit?: number;
  before_created_at?: string;
  before_id?: string;
}

export interface JobsListResponse {
  jobs: Job[];
  count: number;
  next?: { before_created_at: string; before_id: string } | null;
}

export function listJobs(params: JobQuery): Promise<JobsListResponse> {
  const query = new URLSearchParams();
  if (params.queue) query.set("queue", params.queue);
  if (params.status) query.set("status", params.status);
  if (params.type) query.set("type", params.type);
  if (params.limit) query.set("limit", String(params.limit));
  if (params.before_created_at) query.set("before_created_at", params.before_created_at);
  if (params.before_id) query.set("before_id", params.before_id);
  const qs = query.toString();
  return getJSON(`/v1/jobs${qs ? `?${qs}` : ""}`);
}

export function requeueJob(id: string): Promise<Job> {
  return getJSON(`/v1/jobs/${encodeURIComponent(id)}/requeue`, { method: "POST" });
}

export interface RequeueDeadResult {
  queue: string;
  requeued: number;
}

// requeueDeadJobs returns every dead-lettered job in a queue to pending and
// reports how many it revived (0 when there were none).
export function requeueDeadJobs(queue: string): Promise<RequeueDeadResult> {
  return getJSON(`/v1/queues/${encodeURIComponent(queue)}/requeue-dead`, { method: "POST" });
}

export function getJob(id: string): Promise<Job> {
  return getJSON(`/v1/jobs/${encodeURIComponent(id)}`);
}

export function enqueueJob(req: EnqueueRequest): Promise<Job> {
  return getJSON("/v1/jobs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

// getOpenAPI fetches the served spec (a generated artifact; see docs/generate).
export function getOpenAPI(): Promise<Spec> {
  return getJSON("/openapi.json");
}
