package markdown

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestClient_Watch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
		verify  func(t *testing.T, client *Client, events <-chan error, cancel context.CancelFunc)
	}{
		{
			name:    "success (genuine user edit)",
			setup:   setupValidMarkdown,
			wantErr: false,
			verify: func(t *testing.T, client *Client, events <-chan error, cancel context.CancelFunc) {
				errChan := make(chan error)
				go func() {
					errChan <- <-events
				}()

				if err := os.WriteFile(client.filepath, []byte("user edit"), 0o600); err != nil {
					t.Fatalf("failed to trigger fsnotify: %v", err)
				}

				select {
				case err := <-errChan:
					if err != nil {
						t.Errorf("expected nil error ping, got: %v", err)
					}
				case <-time.After(1 * time.Second):
					t.Fatal("Watch() failed to send an event within 1 second")
				}
			},
		},
		{
			name:    "echo deduplication (app edit)",
			setup:   setupValidMarkdown,
			wantErr: false,
			verify: func(t *testing.T, client *Client, events <-chan error, cancel context.CancelFunc) {
				trigger := func() error {
					client.mu.Lock()
					client.lastModTime = time.Now().Add(1 * time.Hour)
					client.mu.Unlock()

					err := os.WriteFile(client.filepath, []byte("app edit"), 0o600)

					return err
				}

				assertIgnoredEvent(t, 1*time.Second, client, events, trigger)
			},
		},
		{
			name: "start failure (invalid directory)",
			setup: func(t *testing.T) string {
				return "/does/not/exist/gtd.md"
			},
			wantErr: true,
		},
		{
			name:    "graceful shutdown",
			setup:   setupValidMarkdown,
			wantErr: false,
			verify: func(t *testing.T, client *Client, events <-chan error, cancel context.CancelFunc) {
				cancel()

				select {
				case _, ok := <-events:
					if ok {
						t.Fatal("expected events channel to be closed")
					}
				case <-time.After(1 * time.Second):
					t.Fatal("Watch() goroutine failed to shut down on context cancel")
				}
			},
		},
		{
			name:    "backpressure",
			setup:   setupValidMarkdown,
			wantErr: false,
			verify: func(t *testing.T, client *Client, events <-chan error, cancel context.CancelFunc) {
				for i := range 5 {
					if err := os.WriteFile(client.filepath, fmt.Appendf(nil, "burst %d", i), 0o600); err != nil {
						t.Fatalf("failed to trigger fsnotify: %v", err)
					}
				}

				select {
				case <-events:
					// Success! It read one event and safely dropped the rest.
				case <-time.After(1 * time.Second):
					t.Fatal("Watch() deadlocked or failed to send burst event")
				}
			},
		},
		{
			name:    "ignored directory event (different file)",
			setup:   setupValidMarkdown,
			wantErr: false,
			verify: func(t *testing.T, client *Client, events <-chan error, cancel context.CancelFunc) {
				trigger := func() error {
					dummyPath := filepath.Join(filepath.Dir(client.filepath), "dummy.md")
					err := os.WriteFile(dummyPath, []byte("noise"), 0o600)

					return err
				}

				assertIgnoredEvent(t, 1*time.Second, client, events, trigger)
			},
		},
		{
			name:    "ignored non-write event (chmod)",
			setup:   setupValidMarkdown,
			wantErr: false,
			verify: func(t *testing.T, client *Client, events <-chan error, cancel context.CancelFunc) {
				trigger := func() error {
					err := os.Chmod(client.filepath, 0o777)
					return err
				}

				assertIgnoredEvent(t, 1*time.Second, client, events, trigger)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			events, err := client.Watch(ctx)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Watch() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			tt.verify(t, client, events, cancel)
		})
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func setupValidMarkdown(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "watch_test.md")
	if err := os.WriteFile(path, []byte("# Inbox"), 0o600); err != nil {
		t.Fatalf("failed to setup file: %v", err)
	}

	return path
}

func assertIgnoredEvent(t *testing.T, timeout time.Duration, c *Client, events <-chan error, trigger func() error) {
	t.Helper()

	collected := make(chan error, 2)
	go func() {
		for err := range events {
			collected <- err
		}
	}()

	if err := trigger(); err != nil {
		t.Fatalf("failed to trigger ignored event: %v", err)
	}

	c.mu.RLock()
	modified := c.lastModTime.Add(1 * time.Hour)
	c.mu.RUnlock()

	file, err := os.OpenFile(c.filepath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("failed to open sentinel file: %v", err)
	}

	if _, err := file.WriteString("\n"); err != nil {
		t.Fatalf("failed to write sentinel byte: %v", err)
	}

	if err := os.Chtimes(c.filepath, modified, modified); err != nil {
		t.Fatalf("failed to advance sentinel timestamp: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("failed to close sentinel file: %v", err)
	}

	select {
	case <-collected:
		// Success! We received the valid sentinel event.
	case <-time.After(timeout):
		t.Fatal("Watch() failed to send an event")
	}

	select {
	case <-collected:
		t.Fatal("Watch() incorrectly processed the ignored event")
	default:
		// Success! The ignored event was dropped.
	}
}
