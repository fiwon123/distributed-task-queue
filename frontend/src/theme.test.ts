import { beforeEach, describe, expect, it } from "vitest";
import { initialTheme, systemTheme } from "./theme";

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
