// A deliberately small OpenAPI reader: enough to list an operation and seed a
// request from the spec. It follows local `$ref`s (`#/…`) and walks simple
// types, but it is not a JSON-Schema implementation.

export type Spec = Record<string, unknown>;

export interface Param {
  name: string;
  in: string; // "path" | "query"
  required: boolean;
  description: string;
  example: string; // seed text for the input
}

export interface Operation {
  key: string; // unique, e.g. "POST /v1/jobs"
  operationId: string;
  method: string; // GET, POST, …
  path: string;
  summary: string;
  tags: string[];
  params: Param[];
  hasBody: boolean;
  bodyExample?: unknown; // seed for the body textarea
}

const METHODS = ["get", "put", "post", "delete", "options", "head", "patch", "trace"] as const;
const MAX_DEPTH = 4;

function asObject(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

// resolve follows a single local $ref (or returns the node as-is). A remote or
// missing reference yields undefined so callers degrade gracefully.
function resolve(spec: Spec, node: unknown): Record<string, unknown> | undefined {
  const obj = asObject(node);
  if (!obj) return undefined;
  const ref = obj.$ref;
  if (typeof ref !== "string") return obj;
  if (!ref.startsWith("#/")) return undefined;
  let current: unknown = spec;
  for (const part of ref.slice(2).split("/")) {
    current = asObject(current)?.[part];
  }
  return asObject(current);
}

// schemaExample returns a placeholder value for a schema: its `example`/
// `default`/first `enum` when present, otherwise a type-shaped zero value. It is
// shallow on purpose and bounded by depth.
export function schemaExample(spec: Spec, schema: unknown, depth = 0): unknown {
  const node = resolve(spec, schema);
  if (!node) return null;
  if (node.example !== undefined) return node.example;
  if (node.default !== undefined) return node.default;
  if (Array.isArray(node.enum) && node.enum.length > 0) return node.enum[0];
  if (depth >= MAX_DEPTH) return null;

  switch (node.type) {
    case "object":
      return objectExample(spec, node, depth);
    case "array":
      return [];
    case "integer":
    case "number":
      return 0;
    case "boolean":
      return false;
    case "string":
      return "";
    default:
      break;
  }
  // Untyped but shaped like an object: still walk its properties.
  return asObject(node.properties) ? objectExample(spec, node, depth) : null;
}

function objectExample(
  spec: Spec,
  node: Record<string, unknown>,
  depth: number,
): Record<string, unknown> {
  const props = asObject(node.properties) ?? {};
  const out: Record<string, unknown> = {};
  for (const [key, prop] of Object.entries(props)) {
    out[key] = schemaExample(spec, prop, depth + 1);
  }
  return out;
}

export function parseOperations(spec: Spec): Operation[] {
  const paths = asObject(spec.paths) ?? {};
  const operations: Operation[] = [];
  for (const [path, rawItem] of Object.entries(paths)) {
    const item = asObject(rawItem);
    if (!item) continue;
    for (const method of METHODS) {
      const raw = asObject(item[method]);
      if (!raw) continue;
      operations.push({
        key: `${method.toUpperCase()} ${path}`,
        operationId: typeof raw.operationId === "string" ? raw.operationId : `${method} ${path}`,
        method: method.toUpperCase(),
        path,
        summary: typeof raw.summary === "string" ? raw.summary : "",
        tags: stringArray(raw.tags),
        params: collectParams(spec, item, raw),
        ...bodySpec(spec, raw),
      });
    }
  }
  return operations;
}

function stringArray(value: unknown): string[] {
  const tags = Array.isArray(value) ? value.filter((t): t is string => typeof t === "string") : [];
  return tags.length > 0 ? tags : ["default"];
}

function collectParams(
  spec: Spec,
  item: Record<string, unknown>,
  op: Record<string, unknown>,
): Param[] {
  const out: Param[] = [];
  for (const source of [item.parameters, op.parameters]) {
    if (!Array.isArray(source)) continue;
    for (const raw of source) {
      const resolved = resolve(spec, raw);
      if (!resolved) continue;
      const { name, in: where } = resolved;
      if (typeof name !== "string" || (where !== "path" && where !== "query")) continue;
      out.push({
        name,
        in: where,
        required: resolved.required === true,
        description: typeof resolved.description === "string" ? resolved.description : "",
        example: seedValue(spec, resolved.schema),
      });
    }
  }
  return out;
}

function seedValue(spec: Spec, schema: unknown): string {
  const value = schemaExample(spec, schema);
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return "";
}

function bodySpec(
  spec: Spec,
  op: Record<string, unknown>,
): { hasBody: boolean; bodyExample?: unknown } {
  const requestBody = asObject(op.requestBody);
  if (!requestBody) return { hasBody: false };
  const schema = mediaSchema(requestBody);
  return schema === undefined
    ? { hasBody: true }
    : { hasBody: true, bodyExample: schemaExample(spec, schema) };
}

function mediaSchema(requestBody: Record<string, unknown>): unknown {
  const content = asObject(requestBody.content);
  if (!content) return undefined;
  const media = content["application/json"] ?? Object.values(content)[0];
  return asObject(media)?.schema;
}
