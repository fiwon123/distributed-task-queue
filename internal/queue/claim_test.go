package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClaimValidatesParams(t *testing.T) {
	// A zero Store has a nil pool; Claim must reject bad params before touching it.
	s := &Store{}

	tests := []struct {
		name string
		p    ClaimParams
	}{
		{"no queues", ClaimParams{Lease: time.Second, Limit: 1}},
		{"zero limit", ClaimParams{Queues: []string{"default"}, Lease: time.Second}},
		{"zero lease", ClaimParams{Queues: []string{"default"}, Limit: 1}},
		{"negative lease", ClaimParams{Queues: []string{"default"}, Lease: -time.Second, Limit: 1}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Claim(context.Background(), tc.p)
			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("error = %v, want *ValidationError", err)
			}
		})
	}
}
