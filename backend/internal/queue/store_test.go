package queue

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func intPtr(n int) *int { return &n }

func TestNormalize(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	five := 5

	tests := []struct {
		name    string
		in      EnqueueParams
		want    EnqueueParams
		wantErr string
	}{
		{
			name: "applies defaults",
			in:   EnqueueParams{Type: "email"},
			want: EnqueueParams{
				Type:        "email",
				Payload:     json.RawMessage(`{}`),
				Queue:       DefaultQueue,
				MaxAttempts: intPtr(DefaultMaxAttempts),
			},
		},
		{
			name: "preserves and trims provided fields",
			in: EnqueueParams{
				Type:        "  email  ",
				Payload:     json.RawMessage(`{"to":"a@b.c"}`),
				Queue:       "  critical ",
				RunAt:       &when,
				MaxAttempts: &five,
			},
			want: EnqueueParams{
				Type:        "email",
				Payload:     json.RawMessage(`{"to":"a@b.c"}`),
				Queue:       "critical",
				RunAt:       &when,
				MaxAttempts: &five,
			},
		},
		{
			name:    "rejects empty type",
			in:      EnqueueParams{Type: "   "},
			wantErr: "type is required",
		},
		{
			name:    "rejects invalid payload",
			in:      EnqueueParams{Type: "email", Payload: json.RawMessage(`{`)},
			wantErr: "payload must be valid JSON",
		},
		{
			name:    "rejects zero max_attempts",
			in:      EnqueueParams{Type: "email", MaxAttempts: intPtr(0)},
			wantErr: "max_attempts must be at least 1",
		},
		{
			name:    "rejects negative max_attempts",
			in:      EnqueueParams{Type: "email", MaxAttempts: intPtr(-3)},
			wantErr: "max_attempts must be at least 1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalize(tc.in)

			if tc.wantErr != "" {
				var invalid *ValidationError
				if !errors.As(err, &invalid) {
					t.Fatalf("error = %v, want *ValidationError", err)
				}
				if invalid.Message != tc.wantErr {
					t.Fatalf("message = %q, want %q", invalid.Message, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Type != tc.want.Type || got.Queue != tc.want.Queue {
				t.Errorf("type/queue = %q/%q, want %q/%q", got.Type, got.Queue, tc.want.Type, tc.want.Queue)
			}
			if string(got.Payload) != string(tc.want.Payload) {
				t.Errorf("payload = %s, want %s", got.Payload, tc.want.Payload)
			}
			if (got.MaxAttempts == nil) != (tc.want.MaxAttempts == nil) {
				t.Fatalf("max_attempts = %v, want %v", got.MaxAttempts, tc.want.MaxAttempts)
			}
			if got.MaxAttempts != nil && *got.MaxAttempts != *tc.want.MaxAttempts {
				t.Errorf("max_attempts = %d, want %d", *got.MaxAttempts, *tc.want.MaxAttempts)
			}
			if (got.RunAt == nil) != (tc.want.RunAt == nil) {
				t.Fatalf("run_at = %v, want %v", got.RunAt, tc.want.RunAt)
			}
			if got.RunAt != nil && !got.RunAt.Equal(*tc.want.RunAt) {
				t.Errorf("run_at = %v, want %v", got.RunAt, *tc.want.RunAt)
			}
		})
	}
}
