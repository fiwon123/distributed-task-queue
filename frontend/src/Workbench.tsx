import { useEffect, useMemo, useState } from "react";
import { getOpenAPI } from "./api";
import { errorMessage, prettyJSON } from "./format";
import { parseOperations, type Operation } from "./openapi";
import Skeleton from "./Skeleton";

interface ResponseView {
  status: number;
  statusText: string;
  ok: boolean;
  ms: number;
  body: unknown;
}

function seedValues(op: Operation): Record<string, string> {
  const values: Record<string, string> = {};
  for (const p of op.params) values[p.name] = p.example;
  return values;
}

function buildURL(op: Operation, values: Record<string, string>): string {
  let path = op.path;
  const query = new URLSearchParams();
  for (const p of op.params) {
    const value = values[p.name] ?? "";
    if (p.in === "path") path = path.replace(`{${p.name}}`, encodeURIComponent(value));
    else if (value !== "") query.append(p.name, value);
  }
  const qs = query.toString();
  return qs ? `${path}?${qs}` : path;
}

// Workbench renders the API from the served OpenAPI document and sends the
// selected operation. It is deliberately shallow: path/query params seeds and a
// body, nothing more.
export default function Workbench() {
  const [operations, setOperations] = useState<Operation[] | null>(null);
  const [specError, setSpecError] = useState<string | null>(null);
  const [selectedKey, setSelectedKey] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [body, setBody] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const [response, setResponse] = useState<ResponseView | null>(null);
  const [sending, setSending] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const spec = await getOpenAPI();
        if (cancelled) return;
        const list = parseOperations(spec);
        setOperations(list);
        setSelectedKey((current) => current || list[0]?.key || "");
      } catch (err) {
        if (!cancelled) setSpecError(errorMessage(err));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const selected = useMemo(
    () => operations?.find((op) => op.key === selectedKey) ?? null,
    [operations, selectedKey],
  );

  // Re-seed the request whenever a different operation is selected.
  useEffect(() => {
    if (!selected) return;
    setValues(seedValues(selected));
    setBody(selected.hasBody ? prettyJSON(selected.bodyExample ?? {}) : "");
    setResponse(null);
    setValidationError(null);
  }, [selected]);

  const groups = useMemo(() => {
    const map = new Map<string, Operation[]>();
    for (const op of operations ?? []) {
      const tag = op.tags[0] ?? "default";
      const list = map.get(tag);
      if (list) list.push(op);
      else map.set(tag, [op]);
    }
    return [...map.entries()];
  }, [operations]);

  const requestURL = selected ? buildURL(selected, values) : "";

  async function send(op: Operation) {
    setValidationError(null);

    for (const p of op.params) {
      if (p.in === "path" && (values[p.name] ?? "").trim() === "") {
        setValidationError(`Path parameter "${p.name}" is required.`);
        return;
      }
    }

    let payload: string | undefined;
    if (op.hasBody && body.trim() !== "") {
      try {
        JSON.parse(body);
      } catch {
        setValidationError("Request body must be valid JSON.");
        return;
      }
      payload = body;
    }

    const headers = payload === undefined ? undefined : { "Content-Type": "application/json" };
    const started = Date.now();
    setSending(true);
    try {
      const res = await fetch(buildURL(op, values), { method: op.method, headers, body: payload });
      const text = await res.text();
      let parsed: unknown = text;
      try {
        parsed = text === "" ? null : JSON.parse(text);
      } catch {
        // Not JSON: show the raw text.
      }
      setResponse({
        status: res.status,
        statusText: res.statusText,
        ok: res.ok,
        ms: Date.now() - started,
        body: parsed,
      });
    } catch (err) {
      setResponse({
        status: 0,
        statusText: errorMessage(err),
        ok: false,
        ms: Date.now() - started,
        body: null,
      });
    } finally {
      setSending(false);
    }
  }

  return (
    <section aria-label="api workbench" className="workbench">
      <div className="operation-list">
        <h2>API</h2>
        {specError && (
          <p className="banner error" role="alert">
            {specError}
          </p>
        )}
        {operations === null && !specError && <Skeleton lines={4} />}
        {operations?.length === 0 && <p className="muted">The spec has no operations.</p>}
        {groups.map(([tag, ops]) => (
          <div key={tag}>
            <h3 className="muted">{tag}</h3>
            <ul>
              {ops.map((op) => (
                <li key={op.key}>
                  <button
                    type="button"
                    className={`operation method-${op.method}`}
                    aria-pressed={op.key === selectedKey}
                    onClick={() => setSelectedKey(op.key)}
                  >
                    <span className="method">{op.method}</span>{" "}
                    <span className="mono">{op.path}</span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>

      <div className="request">
        {selected ? (
          <>
            <h2>{selected.summary || selected.operationId}</h2>
            <p className="mono muted">{requestURL}</p>

            {selected.params.length > 0 && (
              <fieldset className="parameters">
                <legend>Parameters</legend>
                {selected.params.map((p) => (
                  <label key={p.name}>
                    {p.name} <span className="muted">({p.in})</span>
                    <input
                      value={values[p.name] ?? ""}
                      placeholder={p.description}
                      onChange={(e) =>
                        setValues((current) => ({ ...current, [p.name]: e.target.value }))
                      }
                    />
                  </label>
                ))}
              </fieldset>
            )}

            {selected.hasBody && (
              <label className="body-field">
                Request body (JSON)
                <textarea
                  rows={8}
                  spellCheck={false}
                  value={body}
                  onChange={(e) => setBody(e.target.value)}
                />
              </label>
            )}

            {validationError && (
              <p className="banner error" role="alert">
                {validationError}
              </p>
            )}

            <button type="button" onClick={() => void send(selected)} disabled={sending}>
              {sending ? "Sending…" : "Send"}
            </button>

            {response && (
              <div className="response" role="status">
                <p className={response.ok ? "muted" : "banner error"}>
                  {response.status === 0
                    ? response.statusText
                    : `${response.status} ${response.statusText}`}{" "}
                  · {response.ms} ms
                </p>
                <pre className="mono">{prettyJSON(response.body)}</pre>
              </div>
            )}
          </>
        ) : (
          operations !== null && <p className="muted">Select an operation.</p>
        )}
      </div>
    </section>
  );
}
