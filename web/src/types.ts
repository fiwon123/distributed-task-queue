export type Status = "pending" | "running" | "completed" | "failed" | "dead";

export interface Job {
  id: string;
  type: string;
  payload: unknown;
  queue: string;
  status: Status;
  attempts: number;
  max_attempts: number;
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
