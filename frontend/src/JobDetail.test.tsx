import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import JobDetail from "./JobDetail";
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

const base: Job = {
  id: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  type: "echo",
  payload: { hello: "world" },
  queue: "default",
  priority: 0,
  status: "running",
  attempts: 1,
  max_attempts: 3,
  run_at: "2026-01-01T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
};

function job(over: Partial<Job>): Job {
  return { ...base, ...over };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("JobDetail", () => {
  it("shows the full record with a pretty-printed payload", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(
          job({ status: "dead", last_error: "kaboom", timeout_seconds: 30, priority: 5 }),
        ),
      ),
    );
    render(<JobDetail id={base.id} pollIntervalMs={1000} />);

    const region = await screen.findByRole("region", { name: "job detail" });
    expect(within(region).getByText("kaboom")).toBeInTheDocument();
    expect(within(region).getByText(/1 \/ 3/)).toBeInTheDocument();
    expect(within(region).getByText("30s")).toBeInTheDocument();
    expect(within(region).getByText("5")).toBeInTheDocument();
    expect(within(region).getByText(/"hello": "world"/)).toBeInTheDocument();
  });

  it("shows no timeout when the job has none", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(job({}))),
    );
    render(<JobDetail id={base.id} pollIntervalMs={1000} />);

    const region = await screen.findByRole("region", { name: "job detail" });
    const term = within(region).getByText("Timeout");
    expect(term.nextElementSibling).toHaveTextContent("—");
  });

  it("polls a running job until it is terminal, then stops", async () => {
    let calls = 0;
    const fetchMock = vi.fn(async () => {
      calls += 1;
      return jsonResponse(
        calls === 1
          ? job({ status: "running" })
          : job({ status: "completed", completed_at: "2026-01-01T00:05:00Z" }),
      );
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<JobDetail id={base.id} pollIntervalMs={20} />);

    expect(await screen.findByText("running")).toBeInTheDocument();
    expect(await screen.findByText("completed")).toBeInTheDocument();

    const settled = fetchMock.mock.calls.length;
    await new Promise((resolve) => setTimeout(resolve, 80));
    expect(fetchMock.mock.calls).toHaveLength(settled);
  });

  it("requeues a dead job and refreshes it", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).includes("/requeue")) return jsonResponse(job({ status: "pending" }));
      return jsonResponse(job({ status: "dead", last_error: "kaboom" }));
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<JobDetail id={base.id} pollIntervalMs={1000} />);

    await screen.findByText("kaboom");
    await user.click(screen.getByRole("button", { name: "Requeue" }));

    await waitFor(() =>
      expect(fetchMock.mock.calls.some(([url]) => String(url).includes("/requeue"))).toBe(true),
    );
  });

  it("offers a copyable curl command to requeue a dead job", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(job({ status: "dead", last_error: "kaboom" }))),
    );
    render(<JobDetail id={base.id} pollIntervalMs={1000} />);

    const region = await screen.findByRole("region", { name: "job detail" });
    const command = `curl -X POST ${window.location.origin}/v1/jobs/${base.id}/requeue`;
    expect(within(region).getByText(command)).toBeInTheDocument();
    expect(within(region).getByRole("button", { name: "Copy curl" })).toBeInTheDocument();
  });

  it("omits the requeue command while a job cannot be requeued", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(job({ status: "running" }))),
    );
    render(<JobDetail id={base.id} pollIntervalMs={1000} />);

    const region = await screen.findByRole("region", { name: "job detail" });
    await within(region).findByText("running");
    expect(within(region).queryByText(/curl -X POST/)).not.toBeInTheDocument();
  });
});
