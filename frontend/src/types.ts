export type Status = "pending" | "running" | "completed" | "failed" | "dead";

export interface Job {
  id: string;
  type: string;
  payload: unknown;
  queue: string;
  priority: number;
  status: Status;
  attempts: number;
  max_attempts: number;
  timeout_seconds?: number;
  run_at: string;
  locked_until?: string;
  last_error?: string;
  created_at: string;
  completed_at?: string;
}

export interface QueueInfo {
  name: string;
  total: number;
}

export interface Stats {
  queue: string;
  counts: Record<Status, number>;
  total: number;
}

// EnqueueRequest is the POST /v1/jobs body. Only `type` is required; the API
// defaults the rest (queue "default", payload {}, run_at now, max_attempts 3,
// no timeout, priority 0).
export interface EnqueueRequest {
  type: string;
  payload?: unknown;
  queue?: string;
  priority?: number;
  run_at?: string;
  max_attempts?: number;
  timeout_seconds?: number;
}
