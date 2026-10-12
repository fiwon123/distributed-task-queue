package httpapi

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/fiwon123/distributed-task-queue/backend/internal/queue"
	"github.com/fiwon123/distributed-task-queue/docs"
)

// loadSpec parses and validates the embedded OpenAPI document. A failure here
// means docs/openapi.yaml is malformed, independent of any handler.
func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()

	data, err := fs.ReadFile(docs.FS, "openapi.yaml")
	if err != nil {
		t.Fatalf("read embedded openapi.yaml: %v", err)
	}
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("openapi.yaml is invalid: %v", err)
	}
	return doc
}

// TestOpenAPIRoutesMatchSpec is the drift test: the routes Server actually
// registers and the paths docs/openapi.yaml documents must be the same set. It
// is what keeps the spec from rotting as endpoints are added or removed.
func TestOpenAPIRoutesMatchSpec(t *testing.T) {
	doc := loadSpec(t)

	documented := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			documented[method+" "+path] = true
		}
	}

	registered := map[string]bool{}
	for _, rt := range New(&fakeStore{}, discardLogger()).routes() {
		registered[rt.method+" "+rt.pattern] = true
	}

	for route := range registered {
		if !documented[route] {
			t.Errorf("%s is registered in Server.routes but missing from docs/openapi.yaml", route)
		}
	}
	for route := range documented {
		if !registered[route] {
			t.Errorf("%s is documented in docs/openapi.yaml but not registered in Server.routes", route)
		}
	}
}

// validateResponse asserts body is JSON matching the schema for
// method+pathPattern+status in doc.
func validateResponse(t *testing.T, doc *openapi3.T, method, pathPattern string, status int, body []byte) {
	t.Helper()

	item := doc.Paths.Find(pathPattern)
	if item == nil {
		t.Fatalf("openapi.yaml has no path %q", pathPattern)
	}
	op := item.GetOperation(method)
	if op == nil {
		t.Fatalf("openapi.yaml has no %s %s", method, pathPattern)
	}
	respRef := op.Responses.Status(status)
	if respRef == nil || respRef.Value == nil {
		t.Fatalf("openapi.yaml has no %d response for %s %s", status, method, pathPattern)
	}
	media := respRef.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatalf("openapi.yaml %d response for %s %s has no JSON schema", status, method, pathPattern)
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("response body is not valid JSON: %v\nbody: %s", err, body)
	}
	if err := media.Schema.Value.VisitJSON(decoded); err != nil {
		t.Errorf("%s %s -> %d body violates its OpenAPI schema: %v\nbody: %s",
			method, pathPattern, status, err, body)
	}
}

// sampleJob is a fully-populated, schema-valid job for response validation.
func sampleJob(status queue.Status) queue.Job {
	now := time.Now().UTC().Truncate(time.Second)
	job := queue.Job{
		ID:          testJobID,
		Type:        "echo",
		Payload:     json.RawMessage(`{"msg":"hi"}`),
		Queue:       queue.DefaultQueue,
		Status:      status,
		Attempts:    1,
		MaxAttempts: queue.DefaultMaxAttempts,
		RunAt:       now,
		CreatedAt:   now,
	}
	if status == queue.StatusCompleted {
		completedAt := now
		job.CompletedAt = &completedAt
	}
	return job
}

// sampleStats is a schema-valid queue.Stats with every state present, matching
// what Store.QueueStats returns for a real queue.
func sampleStats(name string) queue.Stats {
	counts := make(map[queue.Status]int, len(queue.AllStatuses))
	for _, st := range queue.AllStatuses {
		counts[st] = 0
	}
	counts[queue.StatusCompleted] = 2
	return queue.Stats{Queue: name, Counts: counts, Total: 2}
}

// TestOpenAPIResponsesValidateSpec drives each handler and checks the real
// response body against the schema the spec documents for that status — so the
// spec cannot silently drift from what the API returns.
func TestOpenAPIResponsesValidateSpec(t *testing.T) {
	doc := loadSpec(t)

	cases := []struct {
		name    string
		method  string
		target  string
		body    string
		store   *fakeStore
		want    int
		pathKey string
	}{
		{
			name: "enqueue created", method: http.MethodPost, target: "/v1/jobs",
			body:  `{"type":"echo","payload":{"msg":"hi"}}`,
			store: &fakeStore{}, want: http.StatusCreated, pathKey: "/v1/jobs",
		},
		{
			name: "enqueue bad json", method: http.MethodPost, target: "/v1/jobs",
			body: `{`, store: &fakeStore{}, want: http.StatusBadRequest, pathKey: "/v1/jobs",
		},
		{
			name: "get job", method: http.MethodGet, target: "/v1/jobs/" + testJobID,
			store: &fakeStore{getJob: sampleJob(queue.StatusCompleted)},
			want:  http.StatusOK, pathKey: "/v1/jobs/{id}",
		},
		{
			name: "get job not found", method: http.MethodGet, target: "/v1/jobs/" + testJobID,
			store: &fakeStore{getErr: queue.ErrNotFound},
			want:  http.StatusNotFound, pathKey: "/v1/jobs/{id}",
		},
		{
			name: "list jobs", method: http.MethodGet, target: "/v1/jobs?queue=default",
			store: &fakeStore{listJobsResult: []queue.Job{sampleJob(queue.StatusPending)}},
			want:  http.StatusOK, pathKey: "/v1/jobs",
		},
		{
			name: "list jobs bad filter", method: http.MethodGet, target: "/v1/jobs?status=bogus",
			store: &fakeStore{listJobsErr: &queue.ValidationError{Message: "invalid status"}},
			want:  http.StatusBadRequest, pathKey: "/v1/jobs",
		},
		{
			name: "requeue", method: http.MethodPost, target: "/v1/jobs/" + testJobID + "/requeue",
			store: &fakeStore{requeueJob: sampleJob(queue.StatusPending)},
			want:  http.StatusOK, pathKey: "/v1/jobs/{id}/requeue",
		},
		{
			name: "requeue conflict", method: http.MethodPost, target: "/v1/jobs/" + testJobID + "/requeue",
			store: &fakeStore{requeueErr: queue.ErrNotRequeueable},
			want:  http.StatusConflict, pathKey: "/v1/jobs/{id}/requeue",
		},
		{
			name: "list queues", method: http.MethodGet, target: "/v1/queues",
			store: &fakeStore{listQueues: []queue.QueueInfo{{Name: "default", Total: 2}}},
			want:  http.StatusOK, pathKey: "/v1/queues",
		},
		{
			name: "queue stats", method: http.MethodGet, target: "/v1/queues/default/stats",
			store: &fakeStore{stats: sampleStats("default")},
			want:  http.StatusOK, pathKey: "/v1/queues/{name}/stats",
		},
		{
			name: "queue stats empty name", method: http.MethodGet, target: "/v1/queues/%20/stats",
			store: &fakeStore{}, want: http.StatusBadRequest, pathKey: "/v1/queues/{name}/stats",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, tc.store, tc.method, tc.target, tc.body)
			if w.Code != tc.want {
				t.Fatalf("%s %s = %d, want %d; body: %s", tc.method, tc.target, w.Code, tc.want, w.Body.String())
			}
			validateResponse(t, doc, tc.method, tc.pathKey, w.Code, w.Body.Bytes())
		})
	}
}

func get(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func TestOpenAPIDocsServed(t *testing.T) {
	handler := New(&fakeStore{}, discardLogger()).WithOpenAPI(docs.FS).Handler()

	spec := get(t, handler, "/openapi.yaml")
	if spec.Code != http.StatusOK {
		t.Fatalf("GET /openapi.yaml = %d, want 200", spec.Code)
	}
	if ct := spec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("GET /openapi.yaml Content-Type = %q, want application/yaml", ct)
	}
	if !strings.Contains(spec.Body.String(), "openapi: 3.0.3") {
		t.Errorf("GET /openapi.yaml body does not look like the spec:\n%s", spec.Body.String())
	}

	page := get(t, handler, "/docs")
	if page.Code != http.StatusOK {
		t.Fatalf("GET /docs = %d, want 200", page.Code)
	}
	if ct := page.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET /docs Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(page.Body.String(), "SwaggerUIBundle") {
		t.Errorf("GET /docs body does not reference Swagger UI:\n%s", page.Body.String())
	}
}

// TestOpenAPIDisabledByDefault pins that the infrastructure routes only exist
// when WithOpenAPI is called.
func TestOpenAPIDisabledByDefault(t *testing.T) {
	handler := New(&fakeStore{}, discardLogger()).Handler()
	for _, target := range []string{"/openapi.yaml", "/docs"} {
		if w := get(t, handler, target); w.Code != http.StatusNotFound {
			t.Errorf("GET %s without WithOpenAPI = %d, want 404", target, w.Code)
		}
	}
}

// TestOpenAPIDoesNotShadowDashboard proves /docs and /openapi.yaml win over the
// SPA catch-all when a dashboard is mounted, so the docs are reachable in the
// full stack.
func TestOpenAPIDoesNotShadowDashboard(t *testing.T) {
	dashboard := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<!doctype html><div id=root></div>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')")},
	}
	handler := New(&fakeStore{}, discardLogger()).
		WithDashboard(dashboard).
		WithOpenAPI(docs.FS).
		Handler()

	if w := get(t, handler, "/docs"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "SwaggerUIBundle") {
		t.Errorf("GET /docs with dashboard mounted = %d; want the docs page, not the SPA", w.Code)
	}
	if w := get(t, handler, "/openapi.yaml"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "openapi:") {
		t.Errorf("GET /openapi.yaml with dashboard mounted = %d; want the spec, not the SPA", w.Code)
	}
	// A client-side route still falls back to the SPA.
	if w := get(t, handler, "/queues/default"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "id=root") {
		t.Errorf("GET /queues/default = %d; want the SPA index.html fallback", w.Code)
	}
}
