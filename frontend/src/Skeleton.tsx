// Skeleton renders shimmering placeholder bars while a view loads. It is
// decorative — the view that shows it marks itself aria-busy — so the bars are
// hidden from assistive technology.
export default function Skeleton({ lines = 4 }: { lines?: number }) {
  return (
    <div className="skeleton" aria-hidden="true">
      {Array.from({ length: lines }, (_, i) => (
        <span key={i} className="skeleton-line" />
      ))}
    </div>
  );
}
