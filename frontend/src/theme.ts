import { useCallback, useEffect, useState } from "react";

// The theme system is deliberately dependency-free: the no-flash script in
// index.html stamps data-theme on <html> before paint, this hook keeps that
// attribute in sync with React, and ThemeToggle flips it.

export type Theme = "light" | "dark";

// Storage key shared by the hook and the inline bootstrap script (index.html).
export const THEME_STORAGE_KEY = "dtq-theme";

// THEME_COLOR is the browser chrome colour for each theme; applyTheme keeps the
// <meta name="theme-color"> tag in sync with the active theme (the bootstrap
// script also writes it before first paint).
export const THEME_COLOR: Record<Theme, string> = {
  dark: "#0d0a17",
  light: "#f7f4ff",
};

// applyTheme stamps the theme on <html> and updates the theme-color meta tag.
export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute("content", THEME_COLOR[theme]);
}

// systemTheme reports the OS preference, defaulting to dark where matchMedia is
// unavailable (for example some jsdom test environments).
export function systemTheme(): Theme {
  if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
    return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  return "dark";
}

// initialTheme prefers the theme already on <html> (set by the bootstrap script
// from storage or the OS), then the stored choice, then the system preference.
export function initialTheme(): Theme {
  const attr = document.documentElement.dataset.theme;
  if (attr === "light" || attr === "dark") return attr;
  try {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    if (stored === "light" || stored === "dark") return stored;
  } catch {
    // localStorage may be unavailable; fall through to the system preference.
  }
  return systemTheme();
}

// useTheme returns the active theme and a toggle that persists the choice.
export function useTheme() {
  const [theme, setTheme] = useState<Theme>(initialTheme);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  const toggle = useCallback(() => {
    setTheme((current) => {
      const next: Theme = current === "dark" ? "light" : "dark";
      try {
        localStorage.setItem(THEME_STORAGE_KEY, next);
      } catch {
        // Ignore storage failures; the in-memory theme still switches.
      }
      return next;
    });
  }, []);

  return { theme, toggle };
}
