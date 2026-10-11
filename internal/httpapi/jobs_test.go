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

	"github.com/fiwon123/distributed-task-queue/internal/queue"
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
	body := `{"type":"email","payload":{"to":"a@b.c"},"queue":"critical","max_attempts":5}`

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
	if p.MaxAttempts == nil || *p.MaxAttempts != 5 {
		t.Errorf("max_attempts = %v, want 5", p.MaxAttempts)
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
