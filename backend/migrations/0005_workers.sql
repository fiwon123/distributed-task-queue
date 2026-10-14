-- Register active workers and their heartbeat. No job->worker attribution.
CREATE TABLE workers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  queues text[] NOT NULL DEFAULT '{}',
  types text[] NOT NULL DEFAULT '{}',
  concurrency integer NOT NULL DEFAULT 1,
  started_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX workers_last_seen_idx ON workers (last_seen_at);
