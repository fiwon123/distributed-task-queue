import { useCallback, useEffect, useRef, useState } from "react";
import { getJob } from "./api";
import { errorMessage, isTerminal } from "./format";
import type { Job } from "./types";

// POLL_INTERVAL_MS is how often an in-flight (pending/running) job is re-fetched.
export const POLL_INTERVAL_MS = 2000;

export interface JobPoll {
  job: Job | null;
  error: string | null;
  loading: boolean;
  refresh: () => void;
}

// useJobPolling fetches a job by id and, while it is non-terminal, re-fetches it
// every intervalMs until it reaches a terminal state, the id changes, or the
// component unmounts.
//
// The caller should key the component on the id so a different job starts fresh;
// refresh() re-fetches in place (e.g. after a requeue). seqRef invalidates a
// superseded in-flight response so a slow fetch cannot overwrite a newer one,
// and every cleanup clears the pending timer — React 19's StrictMode
// double-invoke cannot leak a poll.
export function useJobPolling(id: string, intervalMs?: number): JobPoll {
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const ms = intervalMs ?? POLL_INTERVAL_MS;
  const timerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const seqRef = useRef(0);

  const stop = useCallback(() => {
    if (timerRef.current !== undefined) clearTimeout(timerRef.current);
    timerRef.current = undefined;
  }, []);

  const tick = useCallback(
    async function tick(): Promise<void> {
      const seq = ++seqRef.current;
      try {
        const next = await getJob(id);
        if (seq !== seqRef.current) return; // superseded by refresh/remount
        setJob(next);
        setError(null);
        setLoading(false);
        if (!isTerminal(next.status)) {
          timerRef.current = setTimeout(() => void tick(), ms);
        } else {
          timerRef.current = undefined;
        }
      } catch (err) {
        if (seq !== seqRef.current) return;
        setError(errorMessage(err));
        setLoading(false);
      }
    },
    [id, ms],
  );

  const refresh = useCallback(() => {
    stop();
    setLoading(true);
    void tick();
  }, [stop, tick]);

  useEffect(() => {
    void tick();
    return () => {
      stop();
      seqRef.current += 1; // invalidate any in-flight response after unmount
    };
  }, [tick, stop]);

  return { job, error, loading, refresh };
}
