import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "./App";
import type { RequeueDeadResult } from "./api";
import { DOCS_PATH, REPO_URL } from "./links";
import type { Job, QueueInfo, Stats } from "./types";

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

const queues: QueueInfo[] = [{ name: "critical", total: 2 }];
const stats: Stats = {
  queue: "critical",
  counts: { pending: 1, running: 0, completed: 1, failed: 0, dead: 1 },
  total: 3,
};
const deadJob: Job = {
  id: "11111111-1111-1111-1111-111111111111",
  type: "email",
  payload: { to: "a@b.c" },
  queue: "critical",
  priority: 0,
  status: "dead",
  attempts: 3,
  max_attempts: 3,
  run_at: "2026-01-01T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
  last_error: "kaboom",
};
const completedJob: Job = {
  ...deadJob,
  id: "22222222-2222-2222-2222-222222222222",
  type: "report",
  status: "completed",
  attempts: 1,
  last_error: undefined,
};

interface Call {
  url: string;
  method: string;
}

// installFetch routes the three endpoints the app calls and records requests.
function installFetch(): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = init?.method ?? "GET";
      calls.push({ url, method });
      if (url.endsWith("/requeue-dead"))
        return jsonResponse({ queue: "critical", requeued: 1 } satisfies RequeueDeadResult);
      if (url.includes("/requeue")) return jsonResponse({ ...deadJob, status: "pending" });
      if (url.endsWith("/v1/queues")) return jsonResponse({ queues });
      if (url.endsWith("/stats")) return jsonResponse(stats);
      if (/^\/v1\/jobs\/[^/?]+$/.test(url)) return jsonResponse(deadJob);
      if (url.startsWith("/v1/jobs"))
        return jsonResponse({ jobs: [deadJob, completedJob], count: 2 });
      return jsonResponse({ error: "not found" }, 404);
    }),
  );
  return calls;
}

beforeEach(() => {
  // Each test starts at the console route with a clean theme.
  window.history.pushState({}, "", "/");
  document.documentElement.dataset.theme = "dark";
  localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("App", () => {
  it("loads queue stats and jobs, and only offers requeue for requeueable jobs", async () => {
    installFetch();
    render(<App />);

    const table = await screen.findByRole("table");
    expect(within(table).getByText("email")).toBeInTheDocument();
    expect(within(table).getByText("report")).toBeInTheDocument();
    expect(within(table).getByText("kaboom")).toBeInTheDocument();

    // Only the dead job has a Requeue button.
    const requeueButtons = within(table).getAllByRole("button", { name: "Requeue" });
    expect(requeueButtons).toHaveLength(1);

    const statsRegion = screen.getByRole("region", { name: "queue statistics" });
    expect(statsRegion).toHaveTextContent("dead");
    expect(statsRegion).toHaveTextContent("completed");

    expect(await screen.findByRole("option", { name: /critical/ })).toBeInTheDocument();
  });

  it("POSTs to the requeue endpoint when Requeue is clicked", async () => {
    const calls = installFetch();
    const user = userEvent.setup();
    render(<App />);

    const table = await screen.findByRole("table");
    await user.click(within(table).getByRole("button", { name: "Requeue" }));

    await waitFor(() => {
      expect(calls.some((c) => c.method === "POST" && c.url.includes("/requeue"))).toBe(true);
    });
    expect(await screen.findByRole("status")).toHaveTextContent(/Requeued/);
  });

  it("bulk-requeues every dead job for the selected queue", async () => {
    const calls = installFetch();
    const user = userEvent.setup();
    render(<App />);

    await screen.findByRole("table");
    await user.click(await screen.findByRole("button", { name: "Requeue all dead" }));

    await waitFor(() => {
      expect(calls.some((c) => c.method === "POST" && c.url.endsWith("/requeue-dead"))).toBe(true);
    });
    expect(await screen.findByRole("status")).toHaveTextContent("Requeued 1 dead job");
  });

  it("offers no bulk requeue when the queue has no dead jobs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = typeof input === "string" ? input : input.toString();
        if (url.endsWith("/v1/queues")) return jsonResponse({ queues });
        if (url.endsWith("/stats"))
          return jsonResponse({ ...stats, counts: { ...stats.counts, dead: 0 } });
        if (url.startsWith("/v1/jobs")) return jsonResponse({ jobs: [completedJob], count: 1 });
        return jsonResponse({ error: "not found" }, 404);
      }),
    );
    render(<App />);

    await screen.findByRole("table");
    expect(screen.queryByRole("button", { name: "Requeue all dead" })).not.toBeInTheDocument();
  });

  it("shows an error banner when the API fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ error: "boom" }, 500)),
    );
    render(<App />);

    expect(await screen.findByRole("alert")).toHaveTextContent("boom");
  });

  it("renders the job detail for a /jobs/{id} deep link", async () => {
    window.history.pushState({}, "", `/jobs/${deadJob.id}`);
    installFetch();
    render(<App />);

    const detail = await screen.findByRole("region", { name: "job detail" });
    expect(within(detail).getByText("email")).toBeInTheDocument();
    expect(within(detail).getByText("kaboom")).toBeInTheDocument();
  });
});

describe("console chrome", () => {
  it("renders the brand as the page heading", async () => {
    installFetch();
    render(<App />);

    const heading = screen.getByRole("heading", { level: 1 });
    expect(heading).toHaveTextContent(/Distributed Task Queue/);
  });

  it("links to GitHub from the header and footer", async () => {
    installFetch();
    render(<App />);

    const links = screen.getAllByRole("link", { name: /GitHub/ });
    expect(links.length).toBeGreaterThanOrEqual(2);
    for (const link of links) {
      expect(link).toHaveAttribute("href", REPO_URL);
      expect(link).toHaveAttribute("target", "_blank");
      expect(link).toHaveAttribute("rel", "noreferrer");
    }
  });

  it("links to the API docs from the footer", async () => {
    installFetch();
    render(<App />);

    const contentinfo = screen.getByRole("contentinfo");
    expect(within(contentinfo).getByRole("link", { name: "API docs" })).toHaveAttribute(
      "href",
      DOCS_PATH,
    );
  });

  it("toggles the theme and persists the choice", async () => {
    installFetch();
    const user = userEvent.setup();
    render(<App />);

    expect(document.documentElement.dataset.theme).toBe("dark");
    const toggle = screen.getByTestId("theme-toggle");
    expect(toggle).toHaveAccessibleName("Switch to light theme");

    await user.click(toggle);

    expect(document.documentElement.dataset.theme).toBe("light");
    expect(localStorage.getItem("dtq-theme")).toBe("light");
    expect(toggle).toHaveAccessibleName("Switch to dark theme");
  });

  it("renders the about page and marks its nav item current", async () => {
    window.history.pushState({}, "", "/about");
    installFetch();
    render(<App />);

    expect(await screen.findByRole("region", { name: "about" })).toBeInTheDocument();
    expect(screen.getByText("How it works")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "About" })).toHaveAttribute("aria-current", "page");
  });
});
