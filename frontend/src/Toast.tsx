import { useEffect, type ReactNode } from "react";

// Toast is a transient, dismissible status message pinned to the viewport. It
// keeps role="status" so screen readers announce the update, and clears itself
// after durationMs unless dismissed first.
export default function Toast({
  message,
  onDismiss,
  durationMs = 6000,
}: {
  message: ReactNode;
  onDismiss: () => void;
  durationMs?: number;
}) {
  // The countdown is per mount: JobsView keys the toast by notice id, so every
  // new notice remounts it and the timer always restarts from durationMs.
  useEffect(() => {
    const timer = window.setTimeout(onDismiss, durationMs);
    return () => window.clearTimeout(timer);
  }, [onDismiss, durationMs]);

  return (
    <div className="toast" role="status">
      <span className="toast-message">{message}</span>
      <button
        type="button"
        className="toast-close"
        aria-label="Dismiss notification"
        onClick={onDismiss}
      >
        ×
      </button>
    </div>
  );
}
