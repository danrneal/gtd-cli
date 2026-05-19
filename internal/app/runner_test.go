package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/goleak"

	"github.com/danrneal/gtd.nvim/internal/model"
)

// FakeWatcher is a mock implementation of the Watcher interface for testing purposes.
type FakeWatcher struct {
	events   chan error
	watchErr error
}

func NewFakeWatcher() *FakeWatcher {
	watcher := &FakeWatcher{
		events: make(chan error, 1),
	}

	return watcher
}

func (f *FakeWatcher) Watch(_ context.Context) (<-chan error, error) {
	if f.watchErr != nil {
		return nil, f.watchErr
	}

	return f.events, nil
}

func (f *FakeWatcher) Trigger(err error) {
	go func() {
		select {
		case f.events <- err:
			// Successfully sent the event
		default:
			// Non-blocking send: drop duplicate burst events if the channel is unread
		}
	}()
}

func TestRun(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		setup        func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store Provider, md, tasks RemoteProvider)
		triggerEvent func(mdWatcher, tasksWatcher *FakeWatcher)
		wantStore    []model.List
		wantMd       []model.List
		wantTasks    []model.List
		wantErr      string
	}{
		{
			name: "bootstrap sync processes initial state without events",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, []model.List{})

				md := &FakeRemoteProvider{
					RemoteProvider: setupTestMarkdown(t, []model.List{
						{
							Name:     "New Offline List",
							Modified: modified,
							Items:    []*model.Item{},
						},
					}),
				}

				tasks := &FakeRemoteProvider{
					RemoteProvider: setupTestGoogleTasks(t, []model.List{}),
				}

				return store, md, tasks
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New Offline List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:     "store-list-1",
					Name:   "New Offline List",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "New Offline List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "single event triggers full reconciliation and ID backfill",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, []model.List{})

				md := &FakeRemoteProvider{
					RemoteProvider: setupTestMarkdown(t, []model.List{
						{
							Name:     "New List",
							Modified: modified,
							Items:    []*model.Item{},
						},
					}),
				}

				tasks := &FakeRemoteProvider{
					RemoteProvider: setupTestGoogleTasks(t, []model.List{}),
				}

				return store, md, tasks
			},
			triggerEvent: func(mdWatcher, _ *FakeWatcher) {
				mdWatcher.Trigger(nil)
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:     "store-list-1",
					Name:   "New List",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "remote event pulls into local",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, []model.List{})

				md := &FakeRemoteProvider{
					RemoteProvider: setupTestMarkdown(t, []model.List{}),
				}

				tasks := &FakeRemoteProvider{
					RemoteProvider: setupTestGoogleTasks(t, []model.List{
						{
							Name:     "Remote List",
							Modified: modified,
							Items:    []*model.Item{},
						},
					}),
				}

				return store, md, tasks
			},
			triggerEvent: func(_, tasksWatcher *FakeWatcher) {
				tasksWatcher.Trigger(nil)
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "Remote List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:     "store-list-1",
					Name:   "Remote List",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "Remote List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "watcher startup failure",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				mdWatcher.watchErr = errors.New("simulated watcher error")

				store := setupTestSQLite(t, nil)

				md := &FakeRemoteProvider{
					RemoteProvider: setupTestMarkdown(t, nil),
				}

				tasks := &FakeRemoteProvider{
					RemoteProvider: setupTestGoogleTasks(t, nil),
				}

				return store, md, tasks
			},
			triggerEvent: func(_, _ *FakeWatcher) {},
			wantStore:    []model.List{},
			wantMd:       []model.List{},
			wantTasks:    []model.List{},
			wantErr:      "failed to start watcher for markdown: simulated watcher error",
		},
		{
			name: "fatal watcher error aborts sync loop",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, nil)

				md := &FakeRemoteProvider{
					RemoteProvider: setupTestMarkdown(t, nil),
				}

				tasks := &FakeRemoteProvider{
					RemoteProvider: setupTestGoogleTasks(t, nil),
				}

				return store, md, tasks
			},
			triggerEvent: func(mdWatcher, _ *FakeWatcher) {
				close(mdWatcher.events)
			},
			wantStore: []model.List{},
			wantMd:    []model.List{},
			wantTasks: []model.List{},
			wantErr:   "fatal error in markdown watcher: watcher channel closed unexpectedly",
		},
		{
			name: "pull failure sets retry flag and recovers on next event",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, []model.List{})

				md := &FakeRemoteProvider{
					RemoteProvider: setupTestMarkdown(t, []model.List{
						{
							Name:     "New List",
							Modified: modified,
							Items:    []*model.Item{},
						},
					}),
					ErrNextRead: errors.New("transient i/o error"),
				}

				tasks := &FakeRemoteProvider{
					RemoteProvider: setupTestGoogleTasks(t, []model.List{}),
				}

				return store, md, tasks
			},
			triggerEvent: func(mdWatcher, tasksWatcher *FakeWatcher) {
				mdWatcher.Trigger(nil)
				time.Sleep(5 * time.Millisecond)
				tasksWatcher.Trigger(nil)
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:     "store-list-1",
					Name:   "New List",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "missing provider aborts pull and schedules recreation (push)",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, []model.List{
					{
						ID:         "store-list-1",
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: stringPtr("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
				})

				tasks := &FakeRemoteProvider{
					RemoteProvider: setupTestGoogleTasks(t, []model.List{
						{
							Name:       "Inbox",
							Status:     model.StatusOpen,
							ExternalID: stringPtr("external-list-1"),
							Modified:   modified,
							Items:      []*model.Item{},
						},
					}),
				}

				md := &FakeRemoteProvider{
					RemoteProvider: setupTestMarkdown(t, []model.List{}),
					ErrNextRead:    fs.ErrNotExist,
				}

				return store, md, tasks
			},
			triggerEvent: func(mdWatcher, _ *FakeWatcher) {
				mdWatcher.Trigger(nil)
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:       "store-list-1",
					Name:     "Inbox",
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
		},
		{
			name: "pull failure blocks subsequent push and recovers on next event",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, []model.List{})

				md := &FakeRemoteProvider{
					RemoteProvider: setupTestMarkdown(t, []model.List{
						{
							Name:     "New List",
							Modified: modified,
							Items:    []*model.Item{},
						},
					}),
				}

				tasks := &FakeRemoteProvider{
					RemoteProvider: setupTestGoogleTasks(t, []model.List{}),
					ErrNextRead:    errors.New("transient network error"),
				}

				return store, md, tasks
			},
			triggerEvent: func(mdWatcher, _ *FakeWatcher) {
				mdWatcher.Trigger(nil)
				time.Sleep(5 * time.Millisecond)
				mdWatcher.Trigger(nil)
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:     "store-list-1",
					Name:   "New List",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "push mutation failure sets retry flag and recovers on next event",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, []model.List{})

				md := &FakeRemoteProvider{
					RemoteProvider: setupTestMarkdown(t, []model.List{
						{
							Name:     "New List",
							Modified: modified,
							Items:    []*model.Item{},
						},
					}),
				}

				tasks := &FakeRemoteProvider{
					RemoteProvider: setupTestGoogleTasks(t, []model.List{}),
					ErrNextWrite:   errors.New("transient api error"),
				}

				return store, md, tasks
			},
			triggerEvent: func(_, tasksWatcher *FakeWatcher) {
				tasksWatcher.Trigger(nil)
				time.Sleep(5 * time.Millisecond)
				tasksWatcher.Trigger(nil)
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:     "store-list-1",
					Name:   "New List",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mdWatcher := NewFakeWatcher()
			tasksWatcher := NewFakeWatcher()

			store, md, tasks := tt.setup(t, mdWatcher, tasksWatcher)

			mdSyncer := NewSyncer(store, md)
			tasksSyncer := NewSyncer(store, tasks)

			mdTarget := &SyncTarget{
				Name:    "markdown",
				Syncer:  mdSyncer,
				Watcher: mdWatcher,
			}

			tasksTarget := &SyncTarget{
				Name:    "google_tasks",
				Syncer:  tasksSyncer,
				Watcher: tasksWatcher,
			}

			targets := []*SyncTarget{mdTarget, tasksTarget}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			handlerOpts := &slog.HandlerOptions{
				Level: slog.LevelError,
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, handlerOpts))

			errChan := make(chan error, 1)
			go func() {
				runner := NewRunner(targets, logger)
				errChan <- runner.Run(ctx)
			}()

			if tt.triggerEvent != nil {
				tt.triggerEvent(mdWatcher, tasksWatcher)
			}

			if tt.wantErr != "" {
				select {
				case err := <-errChan:
					if err == nil || err.Error() != tt.wantErr {
						t.Fatalf("expected error %q, got %v", tt.wantErr, err)
					}
				case <-time.After(1 * time.Second):
					t.Fatal("Run() failed to return expected error within 1 second")
				}

				return
			}

			assertEventually(t, 1*time.Second, func() error {
				opts := []cmp.Option{
					cmpopts.EquateEmpty(),
					cmpopts.IgnoreFields(model.List{}, "Modified"),
				}

				gotStoreLists, _ := store.ListLists(context.Background())
				if diff := cmp.Diff(tt.wantStore, gotStoreLists, opts...); diff != "" {
					return fmt.Errorf("Store state mismatch (-want +got):\n%s", diff)
				}

				gotMdLists, _ := md.ListLists(context.Background())
				if diff := cmp.Diff(tt.wantMd, gotMdLists, opts...); diff != "" {
					return fmt.Errorf("Markdown state mismatch (-want +got):\n%s", diff)
				}

				gotTasksLists, _ := tasks.ListLists(context.Background())
				if diff := cmp.Diff(tt.wantTasks, gotTasksLists, opts...); diff != "" {
					return fmt.Errorf("Tasks state mismatch (-want +got):\n%s", diff)
				}

				return nil
			})

			cancel()

			select {
			case err := <-errChan:
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatalf("Run() returned unexpected error: %v", err)
				}
			case <-time.After(1 * time.Second):
				t.Fatal("Run() failed to shut down within 1 second of context cancellation")
			}
		})
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func assertEventually(t *testing.T, timeout time.Duration, verify func() error) {
	t.Helper()

	var lastErr error
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		lastErr = verify()
		if lastErr == nil {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("assertEventually timed out after %v. Last error: %v", timeout, lastErr)
}
