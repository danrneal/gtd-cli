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

	"github.com/danrneal/gtd-cli/model"
	"github.com/danrneal/gtd-cli/providers/sqlite"
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
		setup        func(t *testing.T) (*errorProvider, []*SyncTarget)
		triggerEvent func(t *testing.T, targets []*SyncTarget)
		wantStore    []model.List
		wantRemotes  map[string][]model.List
		wantErr      bool
	}{
		{
			name: "bootstrap sync processes initial state without events",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget) {
				store := setupTestSQLite(t, []model.List{})
				md := setupTestMarkdown(t, []model.List{
					{
						Name:     "New Offline List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				mdSyncer := NewSyncer(store, md)
				mdWatcher := NewFakeWatcher()
				mdTarget := &SyncTarget{
					Name:    "markdown",
					Syncer:  mdSyncer,
					Watcher: mdWatcher,
				}

				tasks := setupTestGoogleTasks(t, []model.List{})
				tasksSyncer := NewSyncer(store, tasks)
				tasksWatcher := NewFakeWatcher()
				tasksTarget := &SyncTarget{
					Name:    "google_tasks",
					Syncer:  tasksSyncer,
					Watcher: tasksWatcher,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}

				return store, targets
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
			wantRemotes: map[string][]model.List{
				"markdown": {
					{
						ID:     "store-list-1",
						Name:   "New Offline List",
						Status: model.StatusOpen,
						Items:  []*model.Item{},
					},
				},
				"google_tasks": {
					{
						Name:       "New Offline List",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Items:      []*model.Item{},
					},
				},
			},
		},
		{
			name: "single event triggers full reconciliation and ID backfill",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget) {
				store := setupTestSQLite(t, []model.List{})

				md := setupTestMarkdown(t, []model.List{})
				mdSyncer := NewSyncer(store, md)
				mdWatcher := NewFakeWatcher()
				mdTarget := &SyncTarget{
					Name:    "markdown",
					Syncer:  mdSyncer,
					Watcher: mdWatcher,
				}

				tasks := setupTestGoogleTasks(t, []model.List{})
				tasksSyncer := NewSyncer(store, tasks)
				tasksWatcher := NewFakeWatcher()
				tasksTarget := &SyncTarget{
					Name:    "google_tasks",
					Syncer:  tasksSyncer,
					Watcher: tasksWatcher,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}

				return store, targets
			},
			triggerEvent: func(t *testing.T, targets []*SyncTarget) {
				list := &model.List{
					Name:     "New List",
					Modified: modified,
					Items:    []*model.Item{},
				}

				mdTarget := targets[0]
				err := mdTarget.Syncer.remote.CreateList(t.Context(), list)
				if err != nil {
					t.Fatalf("failed to insert data during event trigger: %v", err)
				}

				mdWatcher := mustFakeWatcher(t, mdTarget.Watcher)
				mdWatcher.Trigger(nil)
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantRemotes: map[string][]model.List{
				"markdown": {
					{
						ID:     "store-list-1",
						Name:   "New List",
						Status: model.StatusOpen,
						Items:  []*model.Item{},
					},
				},
				"google_tasks": {
					{
						Name:       "New List",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Items:      []*model.Item{},
					},
				},
			},
		},
		{
			name: "watcher startup failure",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget) {
				store := setupTestSQLite(t, nil)

				md := setupTestMarkdown(t, nil)
				mdSyncer := NewSyncer(store, md)
				mdWatcher := NewFakeWatcher()
				mdWatcher.watchErr = errors.New("simulated watcher error")
				mdTarget := &SyncTarget{
					Name:    "markdown",
					Syncer:  mdSyncer,
					Watcher: mdWatcher,
				}

				tasks := setupTestGoogleTasks(t, nil)
				tasksSyncer := NewSyncer(store, tasks)
				tasksWatcher := NewFakeWatcher()
				tasksTarget := &SyncTarget{
					Name:    "google_tasks",
					Syncer:  tasksSyncer,
					Watcher: tasksWatcher,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}

				return store, targets
			},
			triggerEvent: func(t *testing.T, targets []*SyncTarget) {},
			wantStore:    []model.List{},
			wantRemotes: map[string][]model.List{
				"markdown":     {},
				"google_tasks": {},
			},
			wantErr: true,
		},
		{
			name: "fatal watcher error aborts sync loop",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget) {
				store := setupTestSQLite(t, nil)

				md := setupTestMarkdown(t, nil)
				mdSyncer := NewSyncer(store, md)
				mdWatcher := NewFakeWatcher()
				mdTarget := &SyncTarget{
					Name:    "markdown",
					Syncer:  mdSyncer,
					Watcher: mdWatcher,
				}

				tasks := setupTestGoogleTasks(t, nil)
				tasksSyncer := NewSyncer(store, tasks)
				tasksWatcher := NewFakeWatcher()
				tasksTarget := &SyncTarget{
					Name:    "google_tasks",
					Syncer:  tasksSyncer,
					Watcher: tasksWatcher,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}

				return store, targets
			},
			triggerEvent: func(t *testing.T, targets []*SyncTarget) {
				mdTarget := targets[0]
				mdWatcher := mustFakeWatcher(t, mdTarget.Watcher)
				close(mdWatcher.events)
			},
			wantStore: []model.List{},
			wantRemotes: map[string][]model.List{
				"markdown":     {},
				"google_tasks": {},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, targets := tt.setup(t)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			handlerOpts := &slog.HandlerOptions{
				Level: slog.LevelError,
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, handlerOpts))
			errChan, err := startRunner(t, ctx, targets, logger)
			if err != nil && !errors.Is(err, context.Canceled) {
				if tt.wantErr {
					return
				}

				t.Fatalf("expected fatal watcher error, got %v", err)
			}

			if tt.triggerEvent != nil {
				tt.triggerEvent(t, targets)
			}

			if tt.wantErr {
				select {
				case err := <-errChan:
					if err == nil || errors.Is(err, context.Canceled) {
						t.Fatalf("expected fatal watcher error, got %v", err)
					}
				case <-time.After(1 * time.Second):
					t.Fatal("Run() failed to return expected error within 1 second")
				}

				return
			}

			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(model.List{}, "Modified"),
			}

			diff := ""
			deadline := time.Now().Add(1 * time.Second)
			for {
				if time.Now().After(deadline) {
					t.Fatal(diff)
				}

				gotStoreLists, err := store.ListLists(t.Context())
				if err != nil {
					t.Fatalf("failed to list store lists: %v", err)
				}

				if diff = cmp.Diff(tt.wantStore, gotStoreLists, opts...); diff != "" {
					time.Sleep(5 * time.Millisecond)
					diff = fmt.Sprintf("Store state mismatch (-want +got):\n%s", diff)
					continue
				}

				for _, target := range targets {
					gotLists, err := target.Syncer.remote.ListLists(t.Context())
					if err != nil {
						t.Fatalf("%v, %v", target.Name, err)
					}

					if diff = cmp.Diff(tt.wantRemotes[target.Name], gotLists, opts...); diff != "" {
						diff = fmt.Sprintf("Target %q state mismatch (-want +got):\n%s", target.Name, diff)
						break
					}
				}

				if diff != "" {
					time.Sleep(5 * time.Millisecond)
					continue
				}

				break
			}

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

func TestProcessEvent(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	_ = modified

	tests := []struct {
		name            string
		setup           func(t *testing.T) (*errorProvider, []*SyncTarget, syncEvent)
		wantSyncTargets []*SyncTarget
		wantStore       []model.List
		wantRemotes     map[string][]model.List
	}{
		{
			name: "no targets",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget, syncEvent) {
				store := setupTestSQLite(t, []model.List{})
				return store, nil, syncEvent{}
			},
			wantSyncTargets: nil,
			wantStore:       []model.List{},
			wantRemotes:     map[string][]model.List{},
		},
		{
			name: "event skips pull on unaffected target",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget, syncEvent) {
				store := setupTestSQLite(t, []model.List{})

				md := setupTestMarkdown(t, []model.List{})
				mdSyncer := NewSyncer(store, md)
				mdTarget := &SyncTarget{
					Name:   "markdown",
					Syncer: mdSyncer,
				}

				tasks := setupTestGoogleTasks(t, []model.List{})
				tasksSyncer := NewSyncer(store, tasks)
				tasksTarget := &SyncTarget{
					Name:   "google_tasks",
					Syncer: tasksSyncer,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}

				event := syncEvent{
					target: mdTarget,
				}

				return store, targets, event
			},
			wantSyncTargets: []*SyncTarget{
				{
					Name:           "markdown",
					needsPullRetry: false,
					needsPushRetry: false,
				},
				{
					Name:           "google_tasks",
					needsPullRetry: false,
					needsPushRetry: false,
				},
			},
			wantStore: []model.List{},
			wantRemotes: map[string][]model.List{
				"markdown":     {},
				"google_tasks": {},
			},
		},
		{
			name: "successful pull after prior failure triggers push of pending local changes",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget, syncEvent) {
				store := setupTestSQLite(t, []model.List{
					{
						ID:         "store-list-1",
						Name:       "Updated Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified.Add(1),
						Items:      []*model.Item{},
					},
				})

				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				mdSyncer := NewSyncer(store, md)
				mdTarget := &SyncTarget{
					Name:           "markdown",
					Syncer:         mdSyncer,
					needsPullRetry: true,
				}

				tasks := setupTestGoogleTasks(t, []model.List{})
				tasksSyncer := NewSyncer(store, tasks)
				tasksTarget := &SyncTarget{
					Name:   "google_tasks",
					Syncer: tasksSyncer,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}

				event := syncEvent{
					target: mdTarget,
				}

				return store, targets, event
			},
			wantSyncTargets: []*SyncTarget{
				{
					Name:           "markdown",
					needsPullRetry: false,
					needsPushRetry: false,
				},
				{
					Name:           "google_tasks",
					needsPullRetry: false,
					needsPushRetry: false,
				},
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "Updated Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantRemotes: map[string][]model.List{
				"markdown": {
					{
						ID:     "store-list-1",
						Name:   "Updated Inbox",
						Status: model.StatusOpen,
						Items:  []*model.Item{},
					},
				},
				"google_tasks": {},
			},
		},
		{
			name: "missing provider aborts pull and schedules push",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget, syncEvent) {
				store := setupTestSQLite(t, []model.List{
					{
						ID:         "store-list-1",
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
				})

				md := setupTestMarkdown(t, []model.List{})
				mdSyncer := NewSyncer(store, md)
				mdTarget := &SyncTarget{
					Name:   "markdown",
					Syncer: mdSyncer,
				}

				tasks := setupTestGoogleTasks(t, []model.List{})
				tasks.errListLists = fs.ErrNotExist
				tasksSyncer := NewSyncer(store, tasks)
				tasksTarget := &SyncTarget{
					Name:           "google_tasks",
					Syncer:         tasksSyncer,
					needsPullRetry: true,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}

				event := syncEvent{
					target: tasksTarget,
				}

				return store, targets, event
			},
			wantSyncTargets: []*SyncTarget{
				{
					Name:           "markdown",
					needsPullRetry: false,
					needsPushRetry: false,
				},
				{
					Name:           "google_tasks",
					needsPullRetry: false,
					needsPushRetry: false,
				},
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "Inbox",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantRemotes: map[string][]model.List{
				"markdown": {},
				"google_tasks": {
					{
						Name:       "Inbox",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
				},
			},
		},
		{
			name: "pull failure sets retry flag and skips push",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget, syncEvent) {
				store := setupTestSQLite(t, []model.List{})

				md := setupTestMarkdown(t, []model.List{})
				md.errListLists = errors.New("transient network error")
				mdSyncer := NewSyncer(store, md)
				mdTarget := &SyncTarget{
					Name:   "markdown",
					Syncer: mdSyncer,
				}

				tasks := setupTestGoogleTasks(t, []model.List{})
				tasksSyncer := NewSyncer(store, tasks)
				tasksTarget := &SyncTarget{
					Name:   "google_tasks",
					Syncer: tasksSyncer,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}

				event := syncEvent{
					target: mdTarget,
				}

				return store, targets, event
			},
			wantSyncTargets: []*SyncTarget{
				{
					Name:           "markdown",
					needsPullRetry: true,
					needsPushRetry: false,
				},
				{
					Name:           "google_tasks",
					needsPullRetry: false,
					needsPushRetry: false,
				},
			},
			wantStore: []model.List{},
			wantRemotes: map[string][]model.List{
				"markdown":     {},
				"google_tasks": {},
			},
		},
		{
			name: "pull with changes triggers successful push",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget, syncEvent) {
				store := setupTestSQLite(t, []model.List{})

				md := setupTestMarkdown(t, []model.List{})
				mdSyncer := NewSyncer(store, md)
				mdTarget := &SyncTarget{
					Name:   "markdown",
					Syncer: mdSyncer,
				}

				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasksSyncer := NewSyncer(store, tasks)
				tasksTarget := &SyncTarget{
					Name:   "google_tasks",
					Syncer: tasksSyncer,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}

				event := syncEvent{
					target: tasksTarget,
				}

				return store, targets, event
			},
			wantSyncTargets: []*SyncTarget{
				{
					Name:           "markdown",
					needsPullRetry: false,
					needsPushRetry: false,
				},
				{
					Name:           "google_tasks",
					needsPullRetry: false,
					needsPushRetry: false,
				},
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantRemotes: map[string][]model.List{
				"markdown": {
					{
						ID:       "store-list-1",
						Name:     "New List",
						Status:   model.StatusOpen,
						Modified: modified,
						Items:    []*model.Item{},
					},
				},
				"google_tasks": {
					{
						Name:       "New List",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
				},
			},
		},
		{
			name: "pull failure on one target blocks its subsequent push",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget, syncEvent) {
				store := setupTestSQLite(t, []model.List{})

				md := setupTestMarkdown(t, []model.List{})
				md.errListLists = errors.New("transient network error")
				mdSyncer := NewSyncer(store, md)
				mdTarget := &SyncTarget{
					Name:   "markdown",
					Syncer: mdSyncer,
				}

				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasksSyncer := NewSyncer(store, tasks)
				tasksTarget := &SyncTarget{
					Name:   "google_tasks",
					Syncer: tasksSyncer,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}
				event := syncEvent{}

				return store, targets, event
			},
			wantSyncTargets: []*SyncTarget{
				{
					Name:           "markdown",
					needsPullRetry: true,
					needsPushRetry: false,
				},
				{
					Name:           "google_tasks",
					needsPullRetry: false,
					needsPushRetry: false,
				},
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantRemotes: map[string][]model.List{
				"markdown": {},
				"google_tasks": {
					{
						Name:       "New List",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
				},
			},
		},
		{
			name: "push failure sets retry flag",
			setup: func(t *testing.T) (*errorProvider, []*SyncTarget, syncEvent) {
				store := setupTestSQLite(t, []model.List{})

				md := setupTestMarkdown(t, []model.List{})
				md.errCreateList = errors.New("transient api error")
				mdSyncer := NewSyncer(store, md)
				mdTarget := &SyncTarget{
					Name:   "markdown",
					Syncer: mdSyncer,
				}

				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasksSyncer := NewSyncer(store, tasks)
				tasksTarget := &SyncTarget{
					Name:   "google_tasks",
					Syncer: tasksSyncer,
				}

				targets := []*SyncTarget{mdTarget, tasksTarget}
				event := syncEvent{}

				return store, targets, event
			},
			wantSyncTargets: []*SyncTarget{
				{
					Name:           "markdown",
					needsPullRetry: false,
					needsPushRetry: true,
				},
				{
					Name:           "google_tasks",
					needsPullRetry: false,
					needsPushRetry: false,
				},
			},
			wantStore: []model.List{
				{
					ID:         "store-list-1",
					Name:       "New List",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Modified:   modified,
					Items:      []*model.Item{},
				},
			},
			wantRemotes: map[string][]model.List{
				"markdown": {},
				"google_tasks": {
					{
						Name:       "New List",
						Status:     model.StatusOpen,
						ExternalID: new("external-list-1"),
						Modified:   modified,
						Items:      []*model.Item{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, targets, event := tt.setup(t)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			handlerOpts := &slog.HandlerOptions{
				Level: slog.LevelError,
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, handlerOpts))
			runner := NewRunner(targets, logger)

			runner.processEvent(ctx, event)

			opts := []cmp.Option{
				cmp.AllowUnexported(SyncTarget{}),
				cmpopts.IgnoreFields(SyncTarget{}, "Syncer", "Watcher"),
			}

			if diff := cmp.Diff(tt.wantSyncTargets, targets, opts...); diff != "" {
				t.Fatalf("Targets state mismatch (-want +got):\n%s", diff)
			}

			opts = []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(model.List{}, "Modified"),
			}

			gotStoreLists, err := store.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list store lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantStore, gotStoreLists, opts...); diff != "" {
				t.Fatalf("Store state mismatch (-want +got):\n%s", diff)
			}

			for _, target := range targets {
				gotLists, err := target.Syncer.remote.ListLists(t.Context())
				if err != nil {
					t.Fatalf("%v, %v", target.Name, err)
				}

				if diff := cmp.Diff(tt.wantRemotes[target.Name], gotLists, opts...); diff != "" {
					t.Fatalf("Target %q state mismatch (-want +got):\n%s", target.Name, diff)
				}
			}
		})
	}
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func startRunner(t *testing.T, ctx context.Context, targets []*SyncTarget, logger *slog.Logger) (<-chan error, error) {
	t.Helper()

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
		return errChan, nil
	case <-time.After(1 * time.Second):
		return errChan, errors.New("Runner failed to become ready within 1 second")
	case err := <-errChan:
		return errChan, err
	}
}

func mustFakeWatcher(t *testing.T, w Watcher) *FakeWatcher {
	t.Helper()
	fakeWatcher, ok := w.(*FakeWatcher)
	if !ok {
		t.Fatalf("expected watcher to be *FakeWatcher, got %T", w)
	}

	return fakeWatcher
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

	opts := []sqlite.StoreOption{listIDGeneratorOpt, itemIDGeneratorOpt}

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

	for _, list := range lists {
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

		for _, item := range list.Items {
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
