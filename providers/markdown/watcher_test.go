package markdown

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fsnotify/fsnotify"
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
			name: "success (genuine user edit)",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "gtd.md")

				if err := os.WriteFile(path, []byte("# Inbox"), 0o600); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}

				return path
			},
			wantErr: false,
			verify: func(t *testing.T, client *Client, events <-chan error, cancel context.CancelFunc) {
				if err := os.WriteFile(client.filepath, []byte("valid edit"), 0o600); err != nil {
					t.Fatalf("failed to trigger fsnotify: %v", err)
				}

				select {
				case err, ok := <-events:
					if !ok {
						t.Fatal("expected event, but events channel was unexpectedly closed")
					}

					if err != nil {
						t.Errorf("expected nil error ping, got: %v", err)
					}
				case <-time.After(1 * time.Second):
					t.Fatal("Watch() failed to send an event within 1 second")
				}
			},
		},
		{
			name: "start failure (invalid directory)",
			setup: func(t *testing.T) string {
				return "/does/not/exist/gtd.md"
			},
			wantErr: true,
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

			if cap(events) != 1 {
				t.Errorf("Watch() channel capacity = %d, want 1", cap(events))
			}

			tt.verify(t, client, events, cancel)
		})
	}
}

func TestClient_watchLoop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		verify func(
			t *testing.T,
			client *Client,
			fakeWatcher *fsnotify.Watcher,
			events <-chan error,
			cancel context.CancelFunc,
		)
	}{
		{
			name: "processes valid event",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				client.mu.Lock()
				client.lastModTime = time.Now()
				client.mu.Unlock()

				client.mu.RLock()
				initialModTime := client.lastModTime
				client.mu.RUnlock()

				modTime := client.lastModTime.Add(1)
				if err := os.Chtimes(client.filepath, modTime, modTime); err != nil {
					t.Fatalf("failed to advance file time: %v", err)
				}

				event := fsnotify.Event{
					Name: client.filepath,
					Op:   fsnotify.Write,
				}

				fakeWatcher.Events <- event

				assertEventEmitted(t, events)

				client.mu.RLock()
				lastModTime := client.lastModTime
				client.mu.RUnlock()

				if !lastModTime.After(initialModTime) {
					t.Errorf(
						"expected lastModTime to advance, but it did not. Initial: %v, Final: %v",
						initialModTime,
						lastModTime,
					)
				}
			},
		},
		{
			name: "drops duplicate burst events (backpressure)",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				client.mu.Lock()
				client.lastModTime = time.Now()
				client.mu.Unlock()

				event := fsnotify.Event{
					Name: client.filepath,
					Op:   fsnotify.Write,
				}

				modTime := client.lastModTime.Add(1)
				if err := os.Chtimes(client.filepath, modTime, modTime); err != nil {
					t.Fatalf("failed to advance file time: %v", err)
				}

				fakeWatcher.Events <- event

				time.Sleep(DefaultDebounceInterval)
				synctest.Wait()

				modTime = client.lastModTime.Add(1)
				if err := os.Chtimes(client.filepath, modTime, modTime); err != nil {
					t.Fatalf("failed to advance file time: %v", err)
				}

				fakeWatcher.Events <- event

				time.Sleep(DefaultDebounceInterval)
				synctest.Wait()

				select {
				case err := <-events:
					if err != nil {
						t.Errorf("expected nil error, got: %v", err)
					}
				default:
					t.Fatal("expected at least 1 event in the channel")
				}

				select {
				case <-events:
					t.Fatal("expected second event to be dropped, but it was in the channel")
				default:
					// Success: The second event was dropped.
				}
			},
		},
		{
			name: "resets timer on rapid events",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				event := fsnotify.Event{
					Name: client.filepath,
					Op:   fsnotify.Write,
				}

				fakeWatcher.Events <- event

				event = fsnotify.Event{
					Name: client.filepath,
					Op:   fsnotify.Write,
				}

				fakeWatcher.Events <- event

				assertEventEmitted(t, events)
			},
		},
		{
			name: "ignores events for other files",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				event := fsnotify.Event{
					Name: filepath.Join(filepath.Dir(client.filepath), "some_other_file.md"),
					Op:   fsnotify.Write,
				}

				fakeWatcher.Events <- event

				assertEventIgnored(t, events)
			},
		},
		{
			name: "ignores non-write events (chmod)",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				event := fsnotify.Event{
					Name: client.filepath,
					Op:   fsnotify.Chmod,
				}

				fakeWatcher.Events <- event

				assertEventIgnored(t, events)
			},
		},
		{
			name: "ignores events if os.Stat fails",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				if err := os.Remove(client.filepath); err != nil {
					t.Fatalf("failed to remove file for test setup: %v", err)
				}

				event := fsnotify.Event{
					Name: client.filepath,
					Op:   fsnotify.Write,
				}

				fakeWatcher.Events <- event

				assertEventIgnored(t, events)
			},
		},
		{
			name: "ignores events if mod time has not advanced",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				stat, err := os.Stat(client.filepath)
				if err != nil {
					t.Fatalf("failed to stat test file: %v", err)
				}

				client.mu.Lock()
				client.lastModTime = stat.ModTime().Add(1)
				client.mu.Unlock()

				event := fsnotify.Event{
					Name: client.filepath,
					Op:   fsnotify.Write,
				}

				fakeWatcher.Events <- event

				assertEventIgnored(t, events)
			},
		},
		{
			name: "preserves active timer on ignored events",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				event := fsnotify.Event{
					Name: client.filepath,
					Op:   fsnotify.Write,
				}

				fakeWatcher.Events <- event

				event = fsnotify.Event{
					Name: client.filepath,
					Op:   fsnotify.Chmod,
				}

				fakeWatcher.Events <- event

				assertEventEmitted(t, events)
			},
		},
		{
			name: "exits when events channel is closed",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				close(fakeWatcher.Events)

				assertChannelClosed(t, events)
			},
		},
		{
			name: "processes watcher errors",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				fakeWatcher.Errors <- os.ErrPermission

				select {
				case err := <-events:
					if err == nil {
						t.Fatal("expected an error, got nil")
					}
				case <-time.After(1 * time.Second):
					t.Fatal("timeout waiting for error event")
				}
			},
		},
		{
			name: "aborts sending error on context cancellation",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				fakeWatcher.Errors <- os.ErrPermission

				synctest.Wait()

				go func() {
					fakeWatcher.Errors <- os.ErrPermission
				}()

				synctest.Wait()

				cancel()

				<-events

				assertChannelClosed(t, events)
			},
		},
		{
			name: "exits when errors channel is closed",
			verify: func(
				t *testing.T,
				client *Client,
				fakeWatcher *fsnotify.Watcher,
				events <-chan error,
				cancel context.CancelFunc,
			) {
				close(fakeWatcher.Errors)

				assertChannelClosed(t, events)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "gtd.md")
				if err := os.WriteFile(path, []byte("# Inbox"), 0o600); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}

				logger := slog.New(slog.DiscardHandler)
				client := NewClient(path, logger)

				fakeWatcher := &fsnotify.Watcher{
					Events: make(chan fsnotify.Event),
					Errors: make(chan error),
				}

				events := make(chan error, 1)

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()

				go func() {
					defer close(events)

					client.watchLoop(ctx, fakeWatcher, events)
				}()

				tt.verify(t, client, fakeWatcher, events, cancel)
			})
		})
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func assertEventEmitted(t *testing.T, events <-chan error) {
	t.Helper()

	err := <-events
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func assertEventIgnored(t *testing.T, events <-chan error) {
	t.Helper()

	time.Sleep(DefaultDebounceInterval)
	synctest.Wait()

	select {
	case <-events:
		t.Fatal("expected event to be ignored, but it was emitted")
	default:
		// Success: The event was successfully ignored.
	}
}

func assertChannelClosed(t *testing.T, events <-chan error) {
	t.Helper()
	synctest.Wait()

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("expected events channel to be closed, but it was open")
		}
	default:
		t.Fatal("expected events channel to be closed, but it was empty and open")
	}
}
