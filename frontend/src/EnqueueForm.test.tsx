import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import EnqueueForm from "./EnqueueForm";
import type { Job } from "./types";

// jsonResponse returns a fetch-shaped stub, avoiding any dependency on a global
// Response implementation in the test environment.
function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status >= 200 && status < 300 ? "OK" : "Error",
    json: async () => body,
  } as unknown as Response;
}

const created: Job = {
  id: "33333333-3333-3333-3333-333333333333",
  type: "echo",
  payload: { n: 1 },
  queue: "critical",
  status: "pending",
  attempts: 0,
  max_attempts: 5,
  run_at: "2026-01-01T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
};

interface Call {
  url: string;
  method: string;
  body: unknown;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("EnqueueForm", () => {
  it("POSTs the assembled request and reports the created job", async () => {
    const calls: Call[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({
          url: String(input),
          method: init?.method ?? "GET",
          body: init?.body ? JSON.parse(String(init.body)) : undefined,
        });
        return jsonResponse(created, 201);
      }),
    );
    const onEnqueued = vi.fn();
    const user = userEvent.setup();
    render(<EnqueueForm onEnqueued={onEnqueued} onError={vi.fn()} />);

    fireEvent.change(screen.getByLabelText("Queue"), { target: { value: "critical" } });
    fireEvent.change(screen.getByLabelText("Payload (JSON)"), { target: { value: '{"n":1}' } });
    fireEvent.change(screen.getByLabelText("Max attempts"), { target: { value: "5" } });
    fireEvent.change(screen.getByLabelText("Timeout (s)"), { target: { value: "30" } });
    await user.click(screen.getByRole("button", { name: "Enqueue job" }));

    await waitFor(() => expect(onEnqueued).toHaveBeenCalledWith(created));
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("/v1/jobs");
    expect(calls[0].method).toBe("POST");
    expect(calls[0].body).toEqual({
      type: "echo",
      payload: { n: 1 },
      queue: "critical",
      max_attempts: 5,
      timeout_seconds: 30,
    });
  });

  it("omits timeout_seconds when left blank", async () => {
    const calls: Call[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({
          url: String(input),
          method: init?.method ?? "GET",
          body: init?.body ? JSON.parse(String(init.body)) : undefined,
        });
        return jsonResponse(created, 201);
      }),
    );
    const user = userEvent.setup();
    render(<EnqueueForm onEnqueued={vi.fn()} onError={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Enqueue job" }));

    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toEqual({ type: "echo", payload: {}, max_attempts: 3 });
  });

  it("sends timeout_seconds: 0, meaning no timeout", async () => {
    const calls: Call[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({
          url: String(input),
          method: init?.method ?? "GET",
          body: init?.body ? JSON.parse(String(init.body)) : undefined,
        });
        return jsonResponse(created, 201);
      }),
    );
    const user = userEvent.setup();
    render(<EnqueueForm onEnqueued={vi.fn()} onError={vi.fn()} />);

    fireEvent.change(screen.getByLabelText("Timeout (s)"), { target: { value: "0" } });
    await user.click(screen.getByRole("button", { name: "Enqueue job" }));

    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].body).toEqual({
      type: "echo",
      payload: {},
      max_attempts: 3,
      timeout_seconds: 0,
    });
  });

  it("rejects invalid JSON without calling the API", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<EnqueueForm onEnqueued={vi.fn()} onError={vi.fn()} />);

    fireEvent.change(screen.getByLabelText("Payload (JSON)"), { target: { value: "{not json" } });
    await user.click(screen.getByRole("button", { name: "Enqueue job" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/valid JSON/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("surfaces an API error through onError", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ error: "boom" }, 400)),
    );
    const onError = vi.fn();
    const user = userEvent.setup();
    render(<EnqueueForm onEnqueued={vi.fn()} onError={onError} />);

    await user.click(screen.getByRole("button", { name: "Enqueue job" }));

    await waitFor(() => expect(onError).toHaveBeenCalledWith("boom"));
  });
});
