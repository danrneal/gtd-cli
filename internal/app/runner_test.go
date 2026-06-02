package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/goleak"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/providers/sqlite"
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
		triggerEvent func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher, md, tasks RemoteProvider)
		wantStore    []model.List
		wantMd       []model.List
		wantTasks    []model.List
		wantErr      string
	}{
		{
			name: "bootstrap sync processes initial state without events",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, []model.List{})
				md := setupTestMarkdown(t, []model.List{
					{
						Name:     "New Offline List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasks := setupTestGoogleTasks(t, []model.List{})

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
				md := setupTestMarkdown(t, []model.List{})
				tasks := setupTestGoogleTasks(t, []model.List{})

				return store, md, tasks
			},
			triggerEvent: func(t *testing.T, mdWatcher, _ *FakeWatcher, md, _ RemoteProvider) {
				list := &model.List{
					Name:     "New List",
					Modified: modified,
					Items:    []*model.Item{},
				}

				err := md.CreateList(context.Background(), list)
				if err != nil {
					t.Fatalf("failed to insert data during event trigger: %v", err)
				}

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
				md := setupTestMarkdown(t, []model.List{})
				tasks := setupTestGoogleTasks(t, []model.List{})

				return store, md, tasks
			},
			triggerEvent: func(t *testing.T, _, tasksWatcher *FakeWatcher, _, tasks RemoteProvider) {
				list := &model.List{
					Name:     "Remote List",
					Modified: modified,
					Items:    []*model.Item{},
				}

				err := tasks.CreateList(context.Background(), list)
				if err != nil {
					t.Fatalf("failed to insert data during event trigger: %v", err)
				}

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
				md := setupTestMarkdown(t, nil)
				tasks := setupTestGoogleTasks(t, nil)

				return store, md, tasks
			},
			triggerEvent: func(t *testing.T, _, _ *FakeWatcher, _, _ RemoteProvider) {},
			wantStore:    []model.List{},
			wantMd:       []model.List{},
			wantTasks:    []model.List{},
			wantErr:      "failed to start watcher for markdown: simulated watcher error",
		},
		{
			name: "fatal watcher error aborts sync loop",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, nil)
				md := setupTestMarkdown(t, nil)
				tasks := setupTestGoogleTasks(t, nil)

				return store, md, tasks
			},
			triggerEvent: func(t *testing.T, mdWatcher, _ *FakeWatcher, _, _ RemoteProvider) {
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
				md := &errorProvider{
					Provider: setupTestMarkdown(t, []model.List{
						{
							Name:     "New List",
							Modified: modified,
							Items:    []*model.Item{},
						},
					}),
					errListLists: errors.New("transient i/o error"),
				}

				tasks := setupTestGoogleTasks(t, []model.List{})

				return store, md, tasks
			},
			triggerEvent: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher, _, _ RemoteProvider) {
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

				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: stringPtr("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
				})

				md := &errorProvider{
					Provider:     setupTestMarkdown(t, []model.List{}),
					errListLists: fs.ErrNotExist,
				}

				return store, md, tasks
			},
			triggerEvent: func(t *testing.T, mdWatcher, _ *FakeWatcher, _, _ RemoteProvider) {
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
				md := setupTestMarkdown(t, []model.List{
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasks := &errorProvider{
					Provider: setupTestGoogleTasks(t, []model.List{
						{
							Name:     "Old Remote List",
							Modified: modified.Add(-1),
							Items:    []*model.Item{},
						},
					}),
					errListLists: errors.New("transient network error"),
				}

				return store, md, tasks
			},
			triggerEvent: func(t *testing.T, mdWatcher, _ *FakeWatcher, _, _ RemoteProvider) {
				mdWatcher.Trigger(nil)
				time.Sleep(5 * time.Millisecond)
				mdWatcher.Trigger(nil)
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-2"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "Old Remote List",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:       "store-list-1",
					Name:     "New List",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "Old Remote List",
					Status:   model.StatusOpen,
					Position: 1,
					Items:    []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "Old Remote List",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "New List",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "push mutation failure sets retry flag and recovers on next event",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (Provider, RemoteProvider, RemoteProvider) {
				store := setupTestSQLite(t, []model.List{})
				md := setupTestMarkdown(t, []model.List{
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasks := &errorProvider{
					Provider:      setupTestGoogleTasks(t, []model.List{}),
					errCreateList: errors.New("transient api error"),
				}

				return store, md, tasks
			},
			triggerEvent: func(t *testing.T, _, tasksWatcher *FakeWatcher, _, _ RemoteProvider) {
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
			ready := make(chan struct{})
			go func() {
				onReadyOpt := WithOnReady(func() {
					close(ready)
				})

				runner := NewRunner(targets, logger, onReadyOpt)
				errChan <- runner.Run(ctx)
			}()

			select {
			case <-ready:
				// Runner is listening
			case <-time.After(1 * time.Second):
				t.Fatal("Runner failed to become ready within 1 second")
			case err := <-errChan:
				if tt.wantErr != "" {
					if err == nil || err.Error() != tt.wantErr {
						t.Fatalf("expected error %q, got %v", tt.wantErr, err)
					}

					return
				}

				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatalf("Runner crashed during startup: %v", err)
				}
			}

			if tt.triggerEvent != nil {
				tt.triggerEvent(t, mdWatcher, tasksWatcher, md, tasks)
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

				gotStoreLists, err := store.ListLists(context.Background())
				if err != nil {
					return fmt.Errorf("failed to list store lists: %w", err)
				}

				if diff := cmp.Diff(tt.wantStore, gotStoreLists, opts...); diff != "" {
					return fmt.Errorf("Store state mismatch (-want +got):\n%s", diff)
				}

				gotMdLists, err := md.ListLists(context.Background())
				if err != nil {
					return fmt.Errorf("failed to list md lists: %w", err)
				}

				if diff := cmp.Diff(tt.wantMd, gotMdLists, opts...); diff != "" {
					return fmt.Errorf("Markdown state mismatch (-want +got):\n%s", diff)
				}

				gotTasksLists, err := tasks.ListLists(context.Background())
				if err != nil {
					return fmt.Errorf("failed to list tasks lists: %w", err)
				}

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

func setupTestSQLite(t *testing.T, lists []model.List) Provider {
	logger := slog.New(slog.DiscardHandler)
	dbPath := filepath.Join(t.TempDir(), "test.db")

	listCounter := 1
	listIDGeneratorOpt := sqlite.WithListIDGenerator(func() string {
		id := fmt.Sprintf("store-list-%d", listCounter)
		listCounter++

		return id
	})

	itemCounter := 1
	itemIDGeneratorOpt := sqlite.WithItemIDGenerator(func() string {
		id := fmt.Sprintf("store-item-%d", itemCounter)
		itemCounter++

		return id
	})

	opts := []sqlite.StoreOption{listIDGeneratorOpt, itemIDGeneratorOpt}

	store, err := sqlite.NewStore(context.Background(), dbPath, logger, opts...)
	if err != nil {
		t.Fatalf("failed to init sqlite: %v", err)
	}

	t.Cleanup(func() {
		_ = store.Close()
	})

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open direct db connection for overrides: %v", err)
	}

	defer db.Close()

	for _, list := range lists {
		listStatus := list.Status
		if listStatus == model.StatusDeleted {
			list.Status = model.StatusOpen
		}

		if err := store.CreateList(context.Background(), &list); err != nil {
			t.Fatalf("failed to create list: %v", err)
		}

		if listStatus == model.StatusDeleted {
			list.Status = listStatus
			if err := store.UpdateList(context.Background(), &list, &list); err != nil {
				t.Fatalf("failed to update list to deleted: %v", err)
			}
		}

		if !list.Modified.IsZero() {
			_, err := db.ExecContext(
				context.Background(),
				"UPDATE lists SET modified = ? WHERE id = ?",
				list.Modified,
				list.ID,
			)
			if err != nil {
				t.Fatalf("failed to override list modified time: %v", err)
			}
		}

		for _, item := range list.Items {
			item.ListID = list.ID
			itemStatus := item.Status
			if itemStatus == model.StatusDeleted {
				item.Status = model.StatusNotStarted
			}

			if err := store.CreateItem(context.Background(), item, ""); err != nil {
				t.Fatalf("failed to create item: %v", err)
			}

			if itemStatus == model.StatusDeleted {
				item.Status = itemStatus
				if err := store.UpdateItem(context.Background(), item); err != nil {
					t.Fatalf("failed to update item to deleted: %v", err)
				}
			}

			if item.Modified.IsZero() {
				continue
			}

			_, err := db.ExecContext(
				context.Background(),
				"UPDATE items SET modified = ? WHERE id = ?",
				item.Modified,
				item.ID,
			)
			if err != nil {
				t.Fatalf("failed to override item modified time: %v", err)
			}
		}
	}

	return store
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

func stringPtr(s string) *string {
	return &s
}
