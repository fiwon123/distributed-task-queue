import { useId } from "react";

// Logo is the console's brand mark: a violet tile of queue rows with an amber
// "active job" dot. Same artwork as frontend/public/favicon.svg, kept in sync by
// hand so the in-app and tab branding match. Fixed brand colours (not theme
// variables) so the mark reads identically in light and dark. Decorative: it
// always sits next to the wordmark or a heading, so it is hidden from screen
// readers and the gradient id is made unique with useId (the logo renders twice
// on /about).
export default function Logo({ size = 30 }: { size?: number }) {
  const gradientId = `dtq-logo-${useId().replace(/[^a-zA-Z0-9]/g, "")}`;
  return (
    <svg
      className="logo"
      width={size}
      height={size}
      viewBox="0 0 32 32"
      aria-hidden="true"
      focusable="false"
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#a78bfa" />
          <stop offset="1" stopColor="#7c3aed" />
        </linearGradient>
      </defs>
      <rect x="1" y="1" width="30" height="30" rx="9" fill={`url(#${gradientId})`} />
      <rect x="7.5" y="7.6" width="17" height="3.4" rx="1.7" fill="#ffffff" opacity="0.95" />
      <rect x="7.5" y="14" width="17" height="3.4" rx="1.7" fill="#ffffff" opacity="0.72" />
      <rect x="7.5" y="20.4" width="10" height="3.4" rx="1.7" fill="#ffffff" opacity="0.5" />
      <circle cx="24" cy="22.1" r="2.9" fill="#fbbf24" />
    </svg>
  );
}
