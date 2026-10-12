import { useEffect, useState, type MouseEvent, type ReactNode } from "react";

// The dashboard routes client-side with the History API. The API server falls
// back to index.html for unknown non-/v1 paths (see spaHandler in
// backend/internal/httpapi/server.go), so a deep link like /jobs/<id> survives a
// reload; the Vite dev server does the same in development.

// navigate pushes a new history entry and notifies the router. It is a no-op
// when the target is already the current path.
export function navigate(to: string): void {
  if (to === window.location.pathname) return;
  window.history.pushState({}, "", to);
  window.dispatchEvent(new Event("popstate"));
}

// usePath returns the current pathname and re-renders on back/forward navigation.
export function usePath(): string {
  const [path, setPath] = useState(() => window.location.pathname);
  useEffect(() => {
    const onChange = () => setPath(window.location.pathname);
    window.addEventListener("popstate", onChange);
    return () => window.removeEventListener("popstate", onChange);
  }, []);
  return path;
}

// matchJobPath returns the job id for a /jobs/{id} route, or null for any other
// path (including "/", which is the job list).
export function matchJobPath(path: string): string | null {
  const match = /^\/jobs\/([^/]+)\/?$/.exec(path);
  return match ? decodeURIComponent(match[1]) : null;
}

// AppLink renders an anchor that routes client-side. Modifier-clicks and
// middle-clicks fall through to the browser so "open in new tab" still works.
export function AppLink({
  to,
  className,
  children,
}: {
  to: string;
  className?: string;
  children: ReactNode;
}) {
  function onClick(event: MouseEvent<HTMLAnchorElement>) {
    if (event.defaultPrevented || event.button !== 0) return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    navigate(to);
  }
  return (
    <a href={to} className={className} onClick={onClick}>
      {children}
    </a>
  );
}
