package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/goleak"

	"github.com/danrneal/gtd.nvim/internal/model"
)

type FakeWatcher struct {
	events   chan error
	watchErr error
}

func NewFakeWatcher() *FakeWatcher {
	watcher := &FakeWatcher{
		events: make(chan error),
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
		f.events <- err
	}()
}

func TestRun(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		setup        func(mdWatcher, tasksWatcher *FakeWatcher) (store, md, tasks *FakeProvider)
		triggerEvent func(mdWatcher, tasksWatcher *FakeWatcher)
		wantStore    []model.List
		wantMd       []model.List
		wantTasks    []model.List
		wantErr      string
	}{
		{
			name: "single event triggers full reconciliation and ID backfill",
			setup: func(_, _ *FakeWatcher) (*FakeProvider, *FakeProvider, *FakeProvider) {
				store := NewFakeProvider("store", []model.List{})
				md := NewFakeProvider("generic", []model.List{
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasks := NewFakeProvider("external", []model.List{})

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
			setup: func(_, _ *FakeWatcher) (*FakeProvider, *FakeProvider, *FakeProvider) {
				store := NewFakeProvider("store", []model.List{})
				md := NewFakeProvider("generic", []model.List{})
				tasks := NewFakeProvider("external", []model.List{
					{
						Name:     "Remote List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

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
					ID:         "store-list-1",
					Name:       "Remote List",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
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
			setup: func(mdWatcher, _ *FakeWatcher) (*FakeProvider, *FakeProvider, *FakeProvider) {
				mdWatcher.watchErr = errors.New("simulated watcher error")
				return NewFakeProvider("store", nil), NewFakeProvider("generic", nil), NewFakeProvider("external", nil)
			},
			triggerEvent: func(_, _ *FakeWatcher) {},
			wantStore:    []model.List{},
			wantMd:       []model.List{},
			wantTasks:    []model.List{},
			wantErr:      "failed to start watcher for markdown: simulated watcher error",
		},
		{
			name: "fatal watcher error aborts sync loop",
			setup: func(mdWatcher, _ *FakeWatcher) (*FakeProvider, *FakeProvider, *FakeProvider) {
				return NewFakeProvider("store", nil), NewFakeProvider("generic", nil), NewFakeProvider("external", nil)
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
			setup: func(_, _ *FakeWatcher) (*FakeProvider, *FakeProvider, *FakeProvider) {
				store := NewFakeProvider("store", []model.List{})
				md := NewFakeProvider("generic", []model.List{
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				md.errNextRead = errors.New("transient i/o error")

				tasks := NewFakeProvider("external", []model.List{})

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
			name: "push failure sets retry flag and recovers on next event",
			setup: func(_, _ *FakeWatcher) (*FakeProvider, *FakeProvider, *FakeProvider) {
				store := NewFakeProvider("store", []model.List{})
				md := NewFakeProvider("generic", []model.List{
					{
						Name:     "New List",
						Modified: modified,
						Items:    []*model.Item{},
					},
				})

				tasks := NewFakeProvider("external", []model.List{})
				tasks.errNextRead = errors.New("transient network error")

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mdWatcher := NewFakeWatcher()
			tasksWatcher := NewFakeWatcher()

			store, md, tasks := tt.setup(mdWatcher, tasksWatcher)

			mdSyncer := NewSyncer(store, md)
			tasksSyncer := NewSyncer(store, tasks)

			mdNode := &SyncNode{
				Name:    "markdown",
				Syncer:  mdSyncer,
				Watcher: mdWatcher,
			}

			tasksNode := &SyncNode{
				Name:    "google_tasks",
				Syncer:  tasksSyncer,
				Watcher: tasksWatcher,
			}

			syncNodes := []*SyncNode{mdNode, tasksNode}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			handlerOpts := &slog.HandlerOptions{
				Level: slog.LevelError,
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, handlerOpts))

			errChan := make(chan error, 1)
			go func() {
				runner := NewRunner(logger, syncNodes)
				errChan <- runner.Run(ctx)
			}()

			tt.triggerEvent(mdWatcher, tasksWatcher)

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
					cmpopts.IgnoreFields(model.List{}, "Modified"),
				}

				if diff := cmp.Diff(tt.wantStore, store.Lists, opts...); diff != "" {
					return fmt.Errorf("Store state mismatch (-want +got):\n%s", diff)
				}

				if diff := cmp.Diff(tt.wantMd, md.Lists, opts...); diff != "" {
					return fmt.Errorf("Markdown state mismatch (-want +got):\n%s", diff)
				}

				if diff := cmp.Diff(tt.wantTasks, tasks.Lists, opts...); diff != "" {
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
