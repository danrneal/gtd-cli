package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/danrneal/gtd-cli/model"
	"github.com/danrneal/gtd-cli/providers/sqlite"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/goleak"
)

// FakeWatcher is a mock implementation of the Watcher interface for testing purposes.
type FakeWatcher struct {
	Events   chan error
	watchErr error
}

func NewFakeWatcher() *FakeWatcher {
	watcher := &FakeWatcher{
		Events: make(chan error, 1),
	}

	return watcher
}

func (f *FakeWatcher) Watch(_ context.Context) (<-chan error, error) {
	if f.watchErr != nil {
		return nil, f.watchErr
	}

	return f.Events, nil
}

func TestRun(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		setup      func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *errorProvider)
		sendEvents func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher, store, md, tasks *errorProvider)
		wantStore  []model.List
		wantMd     []model.List
		wantTasks  []model.List
		wantErr    bool
	}{
		{
			name: "bootstrap sync processes initial state without events",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *errorProvider) {
				store = setupTestSQLite(t, []model.List{})

				md = setupTestMarkdown(t, []model.List{
					{
						Name:     "New Offline List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasks = setupTestGoogleTasks(t, []model.List{})

				return store, md, tasks
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New Offline List",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "single event triggers full reconciliation and ID backfill",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *errorProvider) {
				store = setupTestSQLite(t, []model.List{})
				md = setupTestMarkdown(t, []model.List{})
				tasks = setupTestGoogleTasks(t, []model.List{})

				return store, md, tasks
			},
			sendEvents: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher, store, md, tasks *errorProvider) {
				list := &model.List{
					Name:     "New List",
					Modified: modified,
					Items:    []*model.Item{},
				}

				err := tasks.CreateList(t.Context(), list)
				if err != nil {
					t.Fatalf("failed to insert data during event trigger: %v", err)
				}

				tasksWatcher.Events <- nil
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "pull failure sets retry flag and recovers on next event",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *errorProvider) {
				store = setupTestSQLite(t, []model.List{
					{
						ID:         "custom-list-1",
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
				})
				md = setupTestMarkdown(t, []model.List{
					{
						ID:       "custom-list-1",
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: modified,
						Items:    []*model.Item{},
					},
				})
				tasks = setupTestGoogleTasks(t, []model.List{
					{
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				md.errListLists = errors.New("transient i/o error")

				return store, md, tasks
			},
			sendEvents: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher, store, md, tasks *errorProvider) {
				tasksWatcher.Events <- nil

				synctest.Wait()
			},
			wantStore: []model.List{
				{
					ID:         "custom-list-1",
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-1",
					Name:       "New List",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-2"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:       "custom-list-1",
					Name:     "Inbox",
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-1",
					Name:     "New List",
					Position: 1,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
				{
					Name:       "New List",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-2"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "missing provider aborts pull and schedules push",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *errorProvider) {
				store = setupTestSQLite(t, []model.List{
					{
						ID:         "custom-list-1",
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
				})

				md = setupTestMarkdown(t, []model.List{
					{
						ID:       "custom-list-1",
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: modified,
					},
				})

				tasks = setupTestGoogleTasks(t, []model.List{})

				return store, md, tasks
			},
			sendEvents: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher, store, md, tasks *errorProvider) {
				tasks.errListLists = errors.New("transient i/o error")

				tasksWatcher.Events <- nil

				synctest.Wait()

				tasks.errListLists = fs.ErrNotExist

				tasksWatcher.Events <- nil

				synctest.Wait()
			},
			wantStore: []model.List{
				{
					ID:         "custom-list-1",
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:       "custom-list-1",
					Name:     "Inbox",
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "push failure sets retry flag",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *errorProvider) {
				store = setupTestSQLite(t, []model.List{
					{
						ID:         "custom-list-1",
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
					},
				})

				md = setupTestMarkdown(t, []model.List{
					{
						ID:       "custom-list-1",
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: modified,
					},
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasks = setupTestGoogleTasks(t, []model.List{
					{
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
					},
				})

				tasks.errCreateList = errors.New("transient api error")

				return store, md, tasks
			},
			sendEvents: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher, store, md, tasks *errorProvider) {
				tasksWatcher.Events <- nil

				synctest.Wait()
			},
			wantStore: []model.List{
				{
					ID:         "custom-list-1",
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-1",
					Name:       "New List",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-2"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:       "custom-list-1",
					Name:     "Inbox",
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-1",
					Name:     "New List",
					Position: 1,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
				{
					Name:       "New List",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-2"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "pull failure on one target blocks its subsequent push",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *errorProvider) {
				store = setupTestSQLite(t, []model.List{
					{
						ID:         "custom-list-1",
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
					},
				})

				md = setupTestMarkdown(t, []model.List{
					{
						ID:       "custom-list-1",
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: modified,
					},
				})
				md.errListLists = errors.New("transient network error")

				tasks = setupTestGoogleTasks(t, []model.List{
					{
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
					},
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				return store, md, tasks
			},
			wantStore: []model.List{
				{
					ID:         "custom-list-1",
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-1",
					Name:       "New List",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-2"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantMd: []model.List{
				{
					ID:       "custom-list-1",
					Name:     "Inbox",
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
			wantTasks: []model.List{
				{
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
				{
					Name:       "New List",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-2"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "watcher startup failure",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *errorProvider) {
				store = setupTestSQLite(t, []model.List{})
				md = setupTestMarkdown(t, []model.List{})
				tasks = setupTestGoogleTasks(t, []model.List{})

				mdWatcher.watchErr = errors.New("simulated startup failure")

				return store, md, tasks
			},
			wantErr: true,
		},
		{
			name: "fatal watcher error aborts sync loop",
			setup: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *errorProvider) {
				store = setupTestSQLite(t, []model.List{})
				md = setupTestMarkdown(t, []model.List{})
				tasks = setupTestGoogleTasks(t, []model.List{})

				return store, md, tasks
			},
			sendEvents: func(t *testing.T, mdWatcher, tasksWatcher *FakeWatcher, store, md, tasks *errorProvider) {
				close(mdWatcher.Events)
				synctest.Wait()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
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

				logger := slog.New(slog.DiscardHandler)
				runner := NewRunner(targets, logger)

				errChan := make(chan error, 1)

				go func() {
					errChan <- runner.Run(ctx)
				}()

				synctest.Wait()

				select {
				case err := <-errChan:
					if tt.wantErr {
						return
					}

					t.Fatalf("Runner failed unexpectedly during startup: %v", err)
				default:
					// Non-blocking read: proceed to test logic if the runner booted successfully
				}

				if tt.sendEvents != nil {
					tt.sendEvents(t, mdWatcher, tasksWatcher, store, md, tasks)
					synctest.Wait()
				}

				if tt.wantErr {
					select {
					case <-errChan:
						return
					default:
						t.Fatal("expected runner to fail, but it did not")
					}
				}

				opts := []cmp.Option{
					cmpopts.EquateEmpty(),
					cmpopts.IgnoreFields(model.List{}, "Modified"),
				}

				gotStore, err := store.ListLists(t.Context())
				if err != nil {
					t.Fatalf("failed to list store lists: %v", err)
				}

				if diff := cmp.Diff(tt.wantStore, gotStore, opts...); diff != "" {
					t.Fatalf("Store state mismatch (-want +got):\n%s", diff)
				}

				gotMd, err := md.ListLists(t.Context())
				if err != nil {
					t.Fatalf("failed to list md lists: %v", err)
				}

				if diff := cmp.Diff(tt.wantMd, gotMd, opts...); diff != "" {
					t.Fatalf("Md state mismatch (-want +got):\n%s", diff)
				}

				gotTasks, err := tasks.ListLists(t.Context())
				if err != nil {
					t.Fatalf("failed to list tasks lists: %v", err)
				}

				if diff := cmp.Diff(tt.wantTasks, gotTasks, opts...); diff != "" {
					t.Fatalf("Tasks state mismatch (-want +got):\n%s", diff)
				}

				cancel()
				synctest.Wait()

				select {
				case err := <-errChan:
					if err != nil && !errors.Is(err, context.Canceled) {
						t.Fatalf("Run() returned unexpected error: %v", err)
					}
				default:
					t.Fatal("Run() failed to return an error after context cancellation")
				}
			})
		})
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func setupTestSQLite(t *testing.T, lists []model.List) *errorProvider {
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

	opts := []sqlite.Option{listIDGeneratorOpt, itemIDGeneratorOpt}

	store, err := sqlite.NewStore(t.Context(), dbPath, logger, opts...)
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

	for i, list := range lists {
		list.Position = i

		listStatus := list.Status
		if listStatus == model.StatusDeleted {
			list.Status = model.StatusOpen
		}

		listModified := list.Modified

		if err := store.CreateList(t.Context(), &list); err != nil {
			t.Fatalf("failed to create list: %v", err)
		}

		if listStatus == model.StatusDeleted {
			list.Status = listStatus
			if err := store.UpdateList(t.Context(), &list, &list); err != nil {
				t.Fatalf("failed to update list to deleted: %v", err)
			}
		}

		for j, item := range list.Items {
			item.Position = j
			item.ListID = list.ID

			itemStatus := item.Status
			if itemStatus == model.StatusDeleted {
				item.Status = model.StatusNotStarted
			}

			itemModified := item.Modified

			if err := store.CreateItem(t.Context(), item, ""); err != nil {
				t.Fatalf("failed to create item: %v", err)
			}

			if itemStatus == model.StatusDeleted {
				item.Status = itemStatus
				if err := store.UpdateItem(t.Context(), item); err != nil {
					t.Fatalf("failed to update item to deleted: %v", err)
				}
			}

			if itemModified.IsZero() {
				continue
			}

			query := `UPDATE items SET modified = ? WHERE id = ?`
			_, err := db.ExecContext(t.Context(), query, itemModified, item.ID)
			if err != nil {
				t.Fatalf("failed to override item modified time: %v", err)
			}

			item.Modified = itemModified
		}

		if listModified.IsZero() {
			continue
		}

		query := `UPDATE lists SET modified = ? WHERE id = ?`
		_, err := db.ExecContext(t.Context(), query, listModified, list.ID)
		if err != nil {
			t.Fatalf("failed to override list modified time: %v", err)
		}

		list.Modified = listModified
	}

	testSQLite := &errorProvider{
		Provider: store,
	}

	return testSQLite
}
