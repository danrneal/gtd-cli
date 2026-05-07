package googletasks

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestClient_Watch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		verify   func(t *testing.T, events <-chan error, cancel context.CancelFunc)
	}{
		{
			name:     "success",
			interval: 5 * time.Millisecond,
			verify: func(t *testing.T, events <-chan error, cancel context.CancelFunc) {
				select {
				case err := <-events:
					if err != nil {
						t.Errorf("expected nil error ping, got: %v", err)
					}
				case <-time.After(1 * time.Second):
					t.Fatal("Watch() failed to send an event within 1 second")
				}
			},
		},
		{
			name:     "graceful shutdown",
			interval: 5 * time.Millisecond,
			verify: func(t *testing.T, events <-chan error, cancel context.CancelFunc) {
				cancel()

				done := make(chan struct{})
				go func() {
					for range events {
						// Drain any events that snuck in before cancel
					}

					close(done)
				}()

				select {
				case <-done:
					// Success! The channel was closed.
				case <-time.After(1 * time.Second):
					t.Fatal("Watch() goroutine failed to shut down on context cancel")
				}
			},
		},
		{
			name:     "backpressure",
			interval: 5 * time.Millisecond,
			verify: func(t *testing.T, events <-chan error, cancel context.CancelFunc) {
				select {
				case <-events:
					// Success! It read one event
				case <-time.After(1 * time.Second):
					t.Fatal("Watch() deadlocked on full channel")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewClient(nil, tt.interval, slog.Default())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			events, err := client.Watch(ctx)
			if err != nil {
				t.Fatalf("Watch() returned an unexpected error: %v", err)
			}

			tt.verify(t, events, cancel)
		})
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
