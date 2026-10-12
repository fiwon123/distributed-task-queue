import { STATUSES } from "./format";
import type { Stats } from "./types";

// QueueComposition draws a stacked proportion bar over the same counts as the
// stat cards, so the mix of a queue reads at a glance. The bar is labelled for
// assistive technology; the cards remain the source of truth for the numbers.
export default function QueueComposition({ counts }: { counts: Stats["counts"] }) {
  const present = STATUSES.filter((status) => (counts[status] ?? 0) > 0);
  if (present.length === 0) return null;

  const label = present.map((status) => `${counts[status]} ${status}`).join(", ");

  return (
    <div className="composition">
      <div className="composition-bar" role="img" aria-label={`Queue composition: ${label}`}>
        {present.map((status) => (
          <span
            key={status}
            className={`composition-segment status-${status}`}
            style={{ flexGrow: counts[status] ?? 0 }}
          />
        ))}
      </div>
      <ul className="composition-legend">
        {present.map((status) => (
          <li key={status} className={`status-${status}`}>
            <span className="composition-swatch" />
            {status} <span className="muted">{counts[status]}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
