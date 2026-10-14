package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/fiwon123/distributed-task-queue/backend/internal/queue"
)

const testJobID = "11111111-1111-1111-1111-111111111111"

// fakeStore is an in-memory JobStore for handler tests. No database required.
type fakeStore struct {
	enqueueParams []queue.EnqueueParams
	enqueueJob    queue.Job
	enqueueErr    error

	getID  string
	getJob queue.Job
	getErr error

	requeueID  string
	requeueJob queue.Job
	requeueErr error

	requeueDeadQueue string
	requeueDeadCount int
	requeueDeadErr   error
	requeueDeadCalls int

	statsQueue string
	stats      queue.Stats
	statsErr   error

	listJobsParams  queue.ListParams
	listJobsResult  []queue.Job
	listJobsErr     error
	listQueues      []queue.QueueInfo
	listQueuesErr   error
	listQueuesCalls int
}

func (f *fakeStore) Enqueue(_ context.Context, p queue.EnqueueParams) (queue.Job, error) {
	f.enqueueParams = append(f.enqueueParams, p)
	if f.enqueueErr != nil {
		return queue.Job{}, f.enqueueErr
	}
	if f.enqueueJob.ID != "" {
		return f.enqueueJob, nil
	}
	return queue.Job{
		ID:          testJobID,
		Type:        p.Type,
		Payload:     p.Payload,
		Queue:       queue.DefaultQueue,
		Status:      queue.StatusPending,
		MaxAttempts: queue.DefaultMaxAttempts,
	}, nil
}

func (f *fakeStore) Get(_ context.Context, id string) (queue.Job, error) {
	f.getID = id
	if f.getErr != nil {
		return queue.Job{}, f.getErr
	}
	return f.getJob, nil
}

func (f *fakeStore) Requeue(_ context.Context, id string) (queue.Job, error) {
	f.requeueID = id
	if f.requeueErr != nil {
		return queue.Job{}, f.requeueErr
	}
	return f.requeueJob, nil
}

func (f *fakeStore) RequeueDead(_ context.Context, queue string) (int, error) {
	f.requeueDeadQueue = queue
	f.requeueDeadCalls++
	if f.requeueDeadErr != nil {
		return 0, f.requeueDeadErr
	}
	return f.requeueDeadCount, nil
}

func (f *fakeStore) QueueStats(_ context.Context, name string) (queue.Stats, error) {
	f.statsQueue = name
	if f.statsErr != nil {
		return queue.Stats{}, f.statsErr
	}
	return f.stats, nil
}

func (f *fakeStore) ListJobs(_ context.Context, p queue.ListParams) ([]queue.Job, error) {
	f.listJobsParams = p
	if f.listJobsErr != nil {
		return nil, f.listJobsErr
	}
	return f.listJobsResult, nil
}

func (f *fakeStore) ListQueues(_ context.Context) ([]queue.QueueInfo, error) {
	f.listQueuesCalls++
	if f.listQueuesErr != nil {
		return nil, f.listQueuesErr
	}
	return f.listQueues, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// do sends a request through the routed handler and returns the recorder.
func do(t *testing.T, store JobStore, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	w := httptest.NewRecorder()
	New(store, discardLogger()).Handler().ServeHTTP(w, r)
	return w
}

func TestHandleEnqueueCreated(t *testing.T) {
	store := &fakeStore{}
	body := `{"type":"email","payload":{"to":"a@b.c"},"queue":"critical","priority":9,"max_attempts":5,"timeout_seconds":30}`

	w := do(t, store, http.MethodPost, "/v1/jobs", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusCreated, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got queue.Job
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != testJobID {
		t.Errorf("id = %q, want %q", got.ID, testJobID)
	}
	if got.Status != queue.StatusPending {
		t.Errorf("status = %q, want %q", got.Status, queue.StatusPending)
	}

	if len(store.enqueueParams) != 1 {
		t.Fatalf("store Enqueue calls = %d, want 1", len(store.enqueueParams))
	}
	p := store.enqueueParams[0]
	if p.Type != "email" || p.Queue != "critical" {
		t.Errorf("params type/queue = %q/%q", p.Type, p.Queue)
	}
	if p.Priority == nil || *p.Priority != 9 {
		t.Errorf("priority = %v, want 9", p.Priority)
	}
	if p.MaxAttempts == nil || *p.MaxAttempts != 5 {
		t.Errorf("max_attempts = %v, want 5", p.MaxAttempts)
	}
	if p.TimeoutSeconds == nil || *p.TimeoutSeconds != 30 {
		t.Errorf("timeout_seconds = %v, want 30", p.TimeoutSeconds)
	}
	if string(p.Payload) != `{"to":"a@b.c"}` {
		t.Errorf("payload = %s", p.Payload)
	}
}

func TestHandleEnqueueBadRequest(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty body", ""},
		{"invalid json", "{"},
		{"unknown field", `{"type":"email","nope":1}`},
		{"wrong field type", `{"type":123}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{}
			w := do(t, store, http.MethodPost, "/v1/jobs", tc.body)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusBadRequest, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), `"error"`) {
				t.Errorf("expected error body, got %s", w.Body.String())
			}
			if len(store.enqueueParams) != 0 {
				t.Errorf("store should not be called on a bad request")
			}
		})
	}
}

func TestHandleEnqueueStoreValidationError(t *testing.T) {
	store := &fakeStore{enqueueErr: &queue.ValidationError{Message: "type is required"}}

	w := do(t, store, http.MethodPost, "/v1/jobs", `{"type":""}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "type is required") {
		t.Errorf("body = %s, want message", w.Body.String())
	}
}

func TestHandleEnqueueInternalError(t *testing.T) {
	store := &fakeStore{enqueueErr: errors.New("boom: connection reset")}

	w := do(t, store, http.MethodPost, "/v1/jobs", `{"type":"email"}`)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if strings.Contains(w.Body.String(), "boom") {
		t.Errorf("internal error leaked to client: %s", w.Body.String())
	}
}

func TestHandleGetJobFound(t *testing.T) {
	store := &fakeStore{getJob: queue.Job{
		ID:      testJobID,
		Type:    "email",
		Payload: json.RawMessage(`{"to":"a@b.c"}`),
		Queue:   queue.DefaultQueue,
		Status:  queue.StatusCompleted,
	}}

	w := do(t, store, http.MethodGet, "/v1/jobs/"+testJobID, "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if store.getID != testJobID {
		t.Errorf("store Get id = %q, want %q", store.getID, testJobID)
	}
	var got queue.Job
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != queue.StatusCompleted {
		t.Errorf("status = %q, want %q", got.Status, queue.StatusCompleted)
	}
}

func TestHandleGetJobNotFound(t *testing.T) {
	store := &fakeStore{getErr: queue.ErrNotFound}

	w := do(t, store, http.MethodGet, "/v1/jobs/"+testJobID, "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetJobMalformedID(t *testing.T) {
	store := &fakeStore{}

	w := do(t, store, http.MethodGet, "/v1/jobs/not-a-uuid", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if store.getID != "" {
		t.Errorf("store should not be called for a malformed id (got %q)", store.getID)
	}
}

func TestHandleGetJobInternalError(t *testing.T) {
	store := &fakeStore{getErr: errors.New("boom")}

	w := do(t, store, http.MethodGet, "/v1/jobs/"+testJobID, "")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "boom") {
		t.Errorf("internal error leaked to client: %s", w.Body.String())
	}
}

func TestIsUUID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{testJobID, true},
		{"11111111-1111-1111-1111-11111111111Z", false},
		{"111111111111111111111111111111111111", false},
		{"", false},
		{"not-a-uuid", false},
	}
	for _, tc := range tests {
		if got := isUUID(tc.in); got != tc.want {
			t.Errorf("isUUID(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestHandleRequeueOK(t *testing.T) {
	store := &fakeStore{requeueJob: queue.Job{
		ID:       testJobID,
		Type:     "email",
		Queue:    queue.DefaultQueue,
		Status:   queue.StatusPending,
		Attempts: 0,
	}}

	w := do(t, store, http.MethodPost, "/v1/jobs/"+testJobID+"/requeue", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if store.requeueID != testJobID {
		t.Errorf("store Requeue id = %q, want %q", store.requeueID, testJobID)
	}
	var got queue.Job
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != queue.StatusPending {
		t.Errorf("status = %q, want %q", got.Status, queue.StatusPending)
	}
}

func TestHandleRequeueErrors(t *testing.T) {
	cases := []struct {
		name       string
		id         string
		storeErr   error
		wantStatus int
		wantCalled bool
	}{
		{"not found", testJobID, queue.ErrNotFound, http.StatusNotFound, true},
		{"not requeueable", testJobID, queue.ErrNotRequeueable, http.StatusConflict, true},
		{"internal error", testJobID, errors.New("boom"), http.StatusInternalServerError, true},
		{"malformed id", "not-a-uuid", nil, http.StatusNotFound, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{requeueErr: tc.storeErr}
			w := do(t, store, http.MethodPost, "/v1/jobs/"+tc.id+"/requeue", "")

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.storeErr != nil && !errors.Is(tc.storeErr, queue.ErrNotRequeueable) {
				if strings.Contains(w.Body.String(), "boom") {
					t.Errorf("internal error leaked to client: %s", w.Body.String())
				}
			}
			if got := store.requeueID != ""; got != tc.wantCalled {
				t.Errorf("store Requeue called = %v, want %v", got, tc.wantCalled)
			}
		})
	}
}

func TestHandleRequeueDeadOK(t *testing.T) {
	store := &fakeStore{requeueDeadCount: 3}

	w := do(t, store, http.MethodPost, "/v1/queues/critical/requeue-dead", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if store.requeueDeadQueue != "critical" {
		t.Errorf("store RequeueDead queue = %q, want %q", store.requeueDeadQueue, "critical")
	}
	var got struct {
		Queue    string `json:"queue"`
		Requeued int    `json:"requeued"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Queue != "critical" || got.Requeued != 3 {
		t.Errorf("response = %+v, want queue=critical requeued=3", got)
	}
}

func TestHandleRequeueDeadNoJobs(t *testing.T) {
	store := &fakeStore{requeueDeadCount: 0}

	w := do(t, store, http.MethodPost, "/v1/queues/empty/requeue-dead", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"requeued":0`) {
		t.Errorf("body = %s, want requeued 0", w.Body.String())
	}
}

func TestHandleRequeueDeadErrors(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		storeErr   error
		wantStatus int
		wantCalled bool
	}{
		{"empty name", "/v1/queues/%20/requeue-dead", nil, http.StatusBadRequest, false},
		{"internal error", "/v1/queues/critical/requeue-dead", errors.New("boom"), http.StatusInternalServerError, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{requeueDeadErr: tc.storeErr}
			w := do(t, store, http.MethodPost, tc.target, "")

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "boom") {
				t.Errorf("internal error leaked to client: %s", w.Body.String())
			}
			if got := store.requeueDeadCalls > 0; got != tc.wantCalled {
				t.Errorf("store RequeueDead called = %v, want %v", got, tc.wantCalled)
			}
		})
	}
}

func TestHandleQueueStatsOK(t *testing.T) {
	want := queue.Stats{
		Queue: "critical",
		Counts: map[queue.Status]int{
			queue.StatusPending:   2,
			queue.StatusRunning:   1,
			queue.StatusCompleted: 5,
			queue.StatusFailed:    0,
			queue.StatusDead:      3,
		},
		Total: 11,
	}
	store := &fakeStore{stats: want}

	w := do(t, store, http.MethodGet, "/v1/queues/critical/stats", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if store.statsQueue != "critical" {
		t.Errorf("store QueueStats queue = %q, want %q", store.statsQueue, "critical")
	}
	var got queue.Stats
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Total != 11 || got.Counts[queue.StatusDead] != 3 {
		t.Errorf("stats = %+v, want %+v", got, want)
	}
}

func TestHandleQueueStatsInternalError(t *testing.T) {
	store := &fakeStore{statsErr: errors.New("boom")}

	w := do(t, store, http.MethodGet, "/v1/queues/default/stats", "")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "boom") {
		t.Errorf("internal error leaked to client: %s", w.Body.String())
	}
}

func TestHandleListJobsOK(t *testing.T) {
	store := &fakeStore{listJobsResult: []queue.Job{
		{ID: testJobID, Type: "email", Queue: "critical", Status: queue.StatusDead},
	}}

	w := do(t, store, http.MethodGet, "/v1/jobs?queue=critical&status=dead&limit=10", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if store.listJobsParams.Queue != "critical" ||
		store.listJobsParams.Status != queue.StatusDead ||
		store.listJobsParams.Limit != 10 {
		t.Errorf("params = %+v, want queue=critical status=dead limit=10", store.listJobsParams)
	}

	var got struct {
		Jobs  []queue.Job `json:"jobs"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Count != 1 || len(got.Jobs) != 1 || got.Jobs[0].ID != testJobID {
		t.Errorf("response = %+v, want one job %s", got, testJobID)
	}
}

func TestHandleListJobsErrors(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		storeErr   error
		wantStatus int
	}{
		{"bad limit", "/v1/jobs?limit=abc", nil, http.StatusBadRequest},
		{"validation error", "/v1/jobs?status=bogus", &queue.ValidationError{Message: "unknown status \"bogus\""}, http.StatusBadRequest},
		{"internal error", "/v1/jobs", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{listJobsErr: tc.storeErr}
			w := do(t, store, http.MethodGet, tc.target, "")

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantStatus == http.StatusBadRequest && tc.storeErr == nil && store.listJobsParams.Limit != 0 {
				t.Errorf("bad limit should not reach the store, got params %+v", store.listJobsParams)
			}
			if strings.Contains(w.Body.String(), "boom") {
				t.Errorf("internal error leaked to client: %s", w.Body.String())
			}
		})
	}
}

func TestHandleListQueuesOK(t *testing.T) {
	store := &fakeStore{listQueues: []queue.QueueInfo{
		{Name: "critical", Total: 3},
		{Name: "default", Total: 7},
	}}

	w := do(t, store, http.MethodGet, "/v1/queues", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var got struct {
		Queues []queue.QueueInfo `json:"queues"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Queues) != 2 || got.Queues[0].Name != "critical" || got.Queues[1].Total != 7 {
		t.Errorf("queues = %+v", got.Queues)
	}
}

func TestHandleListQueuesInternalError(t *testing.T) {
	store := &fakeStore{listQueuesErr: errors.New("boom")}

	w := do(t, store, http.MethodGet, "/v1/queues", "")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "boom") {
		t.Errorf("internal error leaked to client: %s", w.Body.String())
	}
}

// TestDashboardServing covers the embedded-SPA handler: real assets are served,
// unknown client-side routes fall back to index.html, and non-GET is rejected.
func TestDashboardServing(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":    {Data: []byte("<html>dashboard</html>")},
		"assets/app.js": {Data: []byte("console.log('hi')")},
	}
	srv := New(&fakeStore{}, discardLogger()).WithDashboard(assets)
	handler := srv.Handler()

	tests := []struct {
		name        string
		method      string
		target      string
		wantStatus  int
		wantContain string
	}{
		{"root serves index", http.MethodGet, "/", http.StatusOK, "dashboard"},
		{"asset served", http.MethodGet, "/assets/app.js", http.StatusOK, "console.log"},
		{"deep link falls back to index", http.MethodGet, "/jobs/some-id", http.StatusOK, "dashboard"},
		{"method not allowed", http.MethodPost, "/", http.StatusMethodNotAllowed, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.target, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantContain != "" && !strings.Contains(w.Body.String(), tc.wantContain) {
				t.Errorf("body %q does not contain %q", w.Body.String(), tc.wantContain)
			}
		})
	}
}

// TestDashboardDoesNotShadowAPIRoutes verifies /v1 still routes to the API when
// a dashboard is mounted.
func TestDashboardDoesNotShadowAPIRoutes(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html>dashboard</html>")}}
	store := &fakeStore{stats: queue.Stats{Queue: "default", Counts: map[queue.Status]int{}, Total: 0}}
	srv := New(store, discardLogger()).WithDashboard(assets)
	handler := srv.Handler()

	r := httptest.NewRequest(http.MethodGet, "/v1/queues/default/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"queue":"default"`) {
		t.Errorf("API response expected, got %s", w.Body.String())
	}
}

func (f *fakeStore) Ping(ctx context.Context) error {
	return nil
}
