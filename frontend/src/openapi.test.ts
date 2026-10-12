import { describe, expect, it } from "vitest";
import { parseOperations, schemaExample, type Spec } from "./openapi";

const spec: Spec = {
  paths: {
    "/v1/jobs": {
      get: { operationId: "listJobs", tags: ["jobs"], summary: "List jobs" },
      post: {
        operationId: "enqueueJob",
        tags: ["jobs"],
        summary: "Enqueue a job",
        requestBody: {
          content: {
            "application/json": { schema: { $ref: "#/components/schemas/EnqueueRequest" } },
          },
        },
      },
    },
    "/v1/jobs/{id}": {
      get: {
        operationId: "getJob",
        parameters: [{ $ref: "#/components/parameters/JobId" }],
      },
    },
  },
  components: {
    parameters: {
      JobId: { name: "id", in: "path", required: true, schema: { type: "string" } },
    },
    schemas: {
      EnqueueRequest: {
        type: "object",
        properties: {
          type: { type: "string" },
          queue: { type: "string", default: "default" },
          max_attempts: { type: "integer", default: 3 },
        },
      },
    },
  },
};

describe("parseOperations", () => {
  it("lists every method/path with tags and params", () => {
    const keys = parseOperations(spec).map((op) => op.key);
    expect(keys).toEqual(["GET /v1/jobs", "POST /v1/jobs", "GET /v1/jobs/{id}"]);

    const getJob = parseOperations(spec).find((op) => op.key === "GET /v1/jobs/{id}");
    expect(getJob?.params).toEqual([
      { name: "id", in: "path", required: true, description: "", example: "" },
    ]);
  });

  it("seeds a request body from the schema, resolving $ref and defaults", () => {
    const post = parseOperations(spec).find((op) => op.key === "POST /v1/jobs");
    expect(post?.hasBody).toBe(true);
    expect(post?.bodyExample).toEqual({ type: "", queue: "default", max_attempts: 3 });
  });

  it("marks bodyless operations", () => {
    const get = parseOperations(spec).find((op) => op.key === "GET /v1/jobs");
    expect(get?.hasBody).toBe(false);
    expect(get?.params).toEqual([]);
  });
});

describe("schemaExample", () => {
  it("resolves a $ref and walks properties", () => {
    expect(schemaExample(spec, { $ref: "#/components/schemas/EnqueueRequest" })).toEqual({
      type: "",
      queue: "default",
      max_attempts: 3,
    });
  });

  it("prefers example over default, and null for the unknown", () => {
    expect(schemaExample(spec, { example: 7, default: 3 })).toBe(7);
    expect(schemaExample(spec, {})).toBeNull();
  });
});
