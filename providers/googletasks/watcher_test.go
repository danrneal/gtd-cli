package googletasks

import (
	"context"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"
)

func TestClient_Watch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		verify func(t *testing.T, events <-chan error, cancel context.CancelFunc)
	}{
		{
			name: "success",
			verify: func(t *testing.T, events <-chan error, cancel context.CancelFunc) {
				err := <-events
				if err != nil {
					t.Errorf("expected nil error ping, got: %v", err)
				}
			},
		},
		{
			name: "graceful shutdown",
			verify: func(t *testing.T, events <-chan error, cancel context.CancelFunc) {
				cancel()

				synctest.Wait()

				select {
				case err, ok := <-events:
					if ok {
						t.Fatalf("expected events channel to be closed, got %v", err)
					}
				default:
					t.Fatal("expected events channel to be closed, but it was open and empty")
				}
			},
		},
		{
			name: "backpressure",
			verify: func(t *testing.T, events <-chan error, cancel context.CancelFunc) {
				time.Sleep(30 * time.Second)
				synctest.Wait()
				time.Sleep(30 * time.Second)
				synctest.Wait()

				select {
				case err := <-events:
					if err != nil {
						t.Errorf("expected nil error, got: %v", err)
					}
				default:
					t.Fatal("expected 1 event in the channel")
				}

				select {
				case err := <-events:
					t.Fatalf("expected second event to be dropped, but got: %v", err)
				default:
					// Success! The channel is empty.
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				pollInterval := 30 * time.Second
				logger := slog.New(slog.DiscardHandler)
				client := NewClient(nil, pollInterval, logger)

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()

				events, err := client.Watch(ctx)
				if err != nil {
					t.Fatalf("Watch() returned an unexpected error: %v", err)
				}

				if cap(events) != 1 {
					t.Errorf("Watch() channel capacity = %d, want 1", cap(events))
				}

				tt.verify(t, events, cancel)
			})
		})
	}
}
