import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { applyTheme, initialTheme, systemTheme, THEME_COLOR } from "./theme";

describe("theme resolution", () => {
  beforeEach(() => {
    // jsdom has no matchMedia, so systemTheme falls back to dark.
    delete document.documentElement.dataset.theme;
    localStorage.clear();
  });

  it("falls back to dark with no attribute, stored value, or matchMedia", () => {
    expect(systemTheme()).toBe("dark");
    expect(initialTheme()).toBe("dark");
  });

  it("prefers the theme already stamped on <html>", () => {
    document.documentElement.dataset.theme = "light";
    localStorage.setItem("dtq-theme", "dark");
    expect(initialTheme()).toBe("light");
  });

  it("reads a stored preference when <html> has none", () => {
    localStorage.setItem("dtq-theme", "light");
    expect(initialTheme()).toBe("light");
  });
});

describe("applyTheme", () => {
  afterEach(() => {
    document.querySelector('meta[name="theme-color"]')?.remove();
    delete document.documentElement.dataset.theme;
  });

  it("stamps the theme attribute and syncs the theme-color meta", () => {
    const meta = document.createElement("meta");
    meta.setAttribute("name", "theme-color");
    document.head.append(meta);

    applyTheme("light");
    expect(document.documentElement.dataset.theme).toBe("light");
    expect(meta.getAttribute("content")).toBe(THEME_COLOR.light);

    applyTheme("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(meta.getAttribute("content")).toBe(THEME_COLOR.dark);
  });

  it("still stamps the attribute when no theme-color meta is present", () => {
    applyTheme("light");
    expect(document.documentElement.dataset.theme).toBe("light");
  });
});
