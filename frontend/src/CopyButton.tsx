import { useEffect, useRef, useState } from "react";

// CopyButton copies a value to the clipboard and briefly confirms the copy. When
// the Clipboard API is unavailable (insecure context, older browser) the click
// is a no-op rather than an error.
export default function CopyButton({ value, label = "Copy" }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      timer.current = window.setTimeout(() => setCopied(false), 1200);
    } catch {
      // Clipboard unavailable — leave the label unchanged.
    }
  }

  return (
    <button type="button" className="copy-button" onClick={() => void copy()}>
      {copied ? "Copied" : label}
    </button>
  );
}
