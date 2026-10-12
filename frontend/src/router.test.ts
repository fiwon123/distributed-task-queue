import { describe, expect, it } from "vitest";
import { matchJobPath, navigate } from "./router";

describe("matchJobPath", () => {
  it("extracts and decodes the id from a job route", () => {
    expect(matchJobPath("/jobs/abc")).toBe("abc");
    expect(matchJobPath("/jobs/abc/")).toBe("abc");
    expect(matchJobPath("/jobs/a%2Fb")).toBe("a/b");
  });

  it("returns null for any other route", () => {
    expect(matchJobPath("/")).toBeNull();
    expect(matchJobPath("/jobs")).toBeNull();
    expect(matchJobPath("/jobs/a/b")).toBeNull();
  });
});

describe("navigate", () => {
  it("pushes a new entry and is a no-op for the current path", () => {
    window.history.pushState({}, "", "/");
    const visited: string[] = [];
    const onPop = () => visited.push(window.location.pathname);
    window.addEventListener("popstate", onPop);

    navigate("/jobs/x");
    navigate("/jobs/x"); // already there: no second event

    window.removeEventListener("popstate", onPop);
    expect(window.location.pathname).toBe("/jobs/x");
    expect(visited).toEqual(["/jobs/x"]);
  });
});
