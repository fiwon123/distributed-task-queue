import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import Workbench from "./Workbench";

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status >= 200 && status < 300 ? "OK" : "Error",
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as unknown as Response;
}

const spec = {
  paths: {
    "/v1/jobs": {
      get: { operationId: "listJobs", summary: "List jobs", tags: ["jobs"] },
      post: {
        operationId: "enqueueJob",
        summary: "Enqueue a job",
        tags: ["jobs"],
        requestBody: {
          content: {
            "application/json": {
              schema: { type: "object", properties: { type: { type: "string" } } },
            },
          },
        },
      },
    },
  },
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("Workbench", () => {
  it("builds the operation list from the served spec", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(spec)),
    );
    render(<Workbench />);

    expect(await screen.findByRole("button", { name: /GET \/v1\/jobs/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /POST \/v1\/jobs/ })).toBeInTheDocument();
  });

  it("sends the selected operation and shows the response", async () => {
    const calls: { url: string; method: string; body?: string }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith("/openapi.json")) return jsonResponse(spec);
        calls.push({ url, method: init?.method ?? "GET", body: init?.body as string | undefined });
        return jsonResponse({ jobs: [], count: 0 });
      }),
    );
    const user = userEvent.setup();
    render(<Workbench />);

    await user.click(await screen.findByRole("button", { name: /GET \/v1\/jobs/ }));
    await user.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(calls).toEqual([{ url: "/v1/jobs", method: "GET", body: undefined }]),
    );
    expect(await screen.findByText(/200 OK/)).toBeInTheDocument();
  });

  it("rejects an invalid JSON body without sending", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input).endsWith("/openapi.json")) return jsonResponse(spec);
      return jsonResponse({});
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<Workbench />);

    await user.click(await screen.findByRole("button", { name: /POST \/v1\/jobs/ }));
    fireEvent.change(screen.getByLabelText("Request body (JSON)"), { target: { value: "{nope" } });
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/valid JSON/);
    expect(fetchMock.mock.calls.filter(([u]) => !String(u).endsWith("/openapi.json"))).toHaveLength(
      0,
    );
  });
});
