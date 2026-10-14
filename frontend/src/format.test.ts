import { describe, expect, it } from "vitest";
import { isTerminal, prettyJSON, requeueCurl } from "./format";

describe("isTerminal", () => {
  it("is true only for completed, failed, and dead", () => {
    expect(isTerminal("pending")).toBe(false);
    expect(isTerminal("running")).toBe(false);
    expect(isTerminal("completed")).toBe(true);
    expect(isTerminal("failed")).toBe(true);
    expect(isTerminal("dead")).toBe(true);
  });
});

describe("prettyJSON", () => {
  it("indents objects", () => {
    expect(prettyJSON({ a: 1 })).toBe('{\n  "a": 1\n}');
  });

  it("falls back to String() for values JSON cannot encode", () => {
    const cyclic: Record<string, unknown> = {};
    cyclic.self = cyclic;
    expect(prettyJSON(cyclic)).toBe("[object Object]");
  });
});

describe("requeueCurl", () => {
  it("builds a POST command against the given origin", () => {
    expect(requeueCurl("https://queue.example", "abc-123")).toBe(
      "curl -X POST https://queue.example/v1/jobs/abc-123/requeue",
    );
  });
});
