package app

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/providers/googletasks"
	"github.com/danrneal/gtd.nvim/internal/providers/googletasks/googletaskstest"
)

func setupTestGoogleTasks(t *testing.T, lists []model.List) RemoteProvider {
	fakeGoogleTasks := googletaskstest.NewFakeGoogleTasks(t)
	mockHTTPClient := &http.Client{
		Transport: fakeGoogleTasks,
	}

	tasksService, err := tasks.NewService(context.Background(), option.WithHTTPClient(mockHTTPClient))
	if err != nil {
		t.Fatalf("failed to create tasks service: %v", err)
	}

	logger := slog.New(slog.DiscardHandler)
	client := googletasks.NewClient(tasksService, 30*time.Second, logger)

	for _, list := range lists {
		if err := client.CreateList(context.Background(), &list); err != nil {
			t.Fatalf("failed to create list: %v", err)
		}

		if !list.Modified.IsZero() {
			idx := slices.IndexFunc(fakeGoogleTasks.TaskLists, func(t *tasks.TaskList) bool {
				return t.Id == *list.ExternalID
			})

			if idx == -1 {
				t.Fatalf("failed to override list modified time: list %q not found in fake", *list.ExternalID)
			}

			fakeGoogleTasks.TaskLists[idx].Updated = list.Modified.Format(time.RFC3339Nano)
		}

		for _, item := range list.Items {
			item.ExternalListID = list.ExternalID
			if err := client.CreateItem(context.Background(), item, ""); err != nil {
				t.Fatalf("failed to create item: %v", err)
			}

			if item.Modified.IsZero() {
				continue
			}

			fakeTasks := fakeGoogleTasks.Tasks[*list.ExternalID]
			idx := slices.IndexFunc(fakeTasks, func(t *tasks.Task) bool {
				return t.Id == *item.ExternalID
			})

			if idx == -1 {
				t.Fatalf("failed to override item modified time: item %q not found in fake", *item.ExternalID)
			}

			fakeTasks[idx].Updated = item.Modified.Format(time.RFC3339Nano)
		}
	}

	return client
}

func testOneWaySync(t *testing.T) {
	t.Parallel()
	baseTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		setupSrc     func(t *testing.T) Provider
		setupDst     func(t *testing.T) Provider
		wantSrcLists []model.List
		wantDstLists []model.List
		wantUpdated  bool
	}{
		{
			name: "create list (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{})
				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{})
				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{})
				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{})
				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create deleted item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusDeleted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusDeleted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: false,
		},
		{
			name: "create deleted item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusDeleted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: false,
		},
		{
			name: "create list and create item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and create item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and create item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{})
				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and create item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and move item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Position: 0,
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 1,
					Status:   model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and move item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 1,
					Status:   model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and move item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						Position:   0,
						ExternalID: stringPtr("external-list-1"),
						Items:      []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       0,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       0,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and move item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       0,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       0,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list create item and move item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Position: 0,
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								ID:       "store-item-2",
								Title:    "I2",
								Modified: baseTime,
								Status:   model.StatusInProgress,
								Position: 0,
							},
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusInProgress,
								Position: 1,
							},
							{
								ID:       "store-item-3",
								Title:    "I3",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
								Position: 2,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusInProgress,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list create item and move item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I2",
								Modified: baseTime,
								Status:   model.StatusInProgress,
							},
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusInProgress,
							},
							{
								Title:    "I3",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusInProgress,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list create item and move item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						Position:   0,
						ExternalID: stringPtr("external-list-1"),
						Items:      []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								ID:       "store-item-2",
								Title:    "I2",
								Modified: baseTime,
								Status:   model.StatusInProgress,
								Position: 0,
							},
							{
								ID:         "store-item-1",
								Title:      "I1",
								Modified:   baseTime,
								Status:     model.StatusInProgress,
								Position:   1,
								ExternalID: stringPtr("external-task-1"),
							},
							{
								ID:       "store-item-3",
								Title:    "I3",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
								Position: 2,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       0,
							Status:         model.StatusInProgress,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       1,
							Status:         model.StatusInProgress,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							ID:             "store-item-3",
							Title:          "I3",
							Position:       2,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-3"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I2",
							Position:       0,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							Title:          "I1",
							Position:       1,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							Title:          "I3",
							Position:       2,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-3"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list create item and move item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I2",
								Modified: baseTime,
							},
							{
								Title:    "I1",
								Modified: baseTime,
							},
							{
								Title:    "I3",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-2"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I2",
							Position:       0,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							Title:          "I1",
							Position:       1,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							Title:          "I3",
							Position:       2,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-3"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							ID:             "store-item-3",
							Title:          "I3",
							Position:       2,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-3"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item after cross list move (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Position: 0,
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusInProgress,
								Position: 0,
							},
							{
								Title:    "I2",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
								Position: 1,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusInProgress,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item after cross list move (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusInProgress,
							},
							{
								Title:    "I2",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusInProgress,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item after cross list move external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						Position:   0,
						ExternalID: stringPtr("external-list-1"),
						Items:      []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								Status:     model.StatusInProgress,
								Position:   0,
								ExternalID: stringPtr("external-task-1"),
							},
							{
								Title:    "I2",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
								Position: 1,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							Status:         model.StatusInProgress,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Position:       0,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item after cross list move external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
							{
								Title:    "I2",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								Status:     model.StatusInProgress,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Position:       0,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							Status:         model.StatusInProgress,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Original",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Original",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list reorder (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Position: 0,
					},
					{
						Name:     "L2",
						Modified: baseTime,
						Position: 1,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime.Add(1),
					},
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime.Add(1),
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 1,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 1,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list reorder (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime,
					},
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Position: 0,
						Modified: baseTime.Add(1),
					},
					{
						Name:     "L2",
						Position: 1,
						Modified: baseTime.Add(1),
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 1,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 1,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list reorder external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						ID:         "store-list-2",
						Name:       "L2",
						Modified:   baseTime,
						Position:   0,
						ExternalID: stringPtr("external-list-2"),
					},
					{
						ID:         "store-list-1",
						Name:       "L1",
						Modified:   baseTime,
						Position:   1,
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-2",
					Name:       "L2",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-2"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-1",
					Name:       "L1",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-2"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list reorder external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
					{
						Name:     "L2",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						ID:         "store-list-2",
						Name:       "L2",
						Modified:   baseTime.Add(1),
						Position:   0,
						ExternalID: stringPtr("external-list-2"),
					},
					{
						ID:         "store-list-1",
						Name:       "L1",
						Modified:   baseTime.Add(1),
						Position:   1,
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-2"),
					Items:      []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-2"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list drops deleted items (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "Active Item",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
								Position: 0,
							},
							{
								Title:    "Deleted Item",
								Modified: baseTime,
								Status:   model.StatusDeleted,
								Position: 1,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Original",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "Active Item",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
							{
								ID:       "store-item-2",
								Title:    "Deleted Item",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							ListID: "store-list-1",
							Title:  "Active Item",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							ListID: "store-list-1",
							Title:  "Active Item",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list drops deleted items external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "Active Item",
								Modified:   baseTime,
								Status:     model.StatusNotStarted,
								Position:   0,
								ExternalID: stringPtr("external-task-1"),
							},
							{
								Title:      "Deleted Item",
								Modified:   baseTime,
								Status:     model.StatusDeleted,
								Position:   1,
								ExternalID: stringPtr("external-task-2"),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "Active Item",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
							{
								Title:    "Deleted Item",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							ListID:         "store-list-1",
							Title:          "Active Item",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Active Item",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list identical content (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: false,
		},
		{
			name: "update list identical content (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime.Add(1),
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: false,
		},
		{
			name: "update item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
								Status:   model.StatusDone,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1 Original",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusDone,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusDone,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
								Status:   model.StatusInProgress,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Original",
								Modified: baseTime,
								Status:   model.StatusDone,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusInProgress,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusInProgress,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1 Updated",
								Modified:   baseTime.Add(1),
								Status:     model.StatusDone,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Original",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Updated",
							Status:         model.StatusDone,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusDone,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1 Original",
								Modified:   baseTime,
								Status:     model.StatusDone,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Updated",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item identical content (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Original",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1 Original",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							ListID: "store-list-1",
							Title:  "I1 Original",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							ListID: "store-list-1",
							Title:  "I1 Original",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "update item identical content (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1 Original",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Original",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							ListID: "store-list-1",
							Title:  "I1 Original",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							ListID: "store-list-1",
							Title:  "I1 Original",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "update item status skipped external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime.Add(1),
								Status:     model.StatusInProgress,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							ListID:         "store-list-1",
							Title:          "I1",
							Status:         model.StatusInProgress,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item status skipped external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime.Add(1),
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						ExternalID: stringPtr("external-list-1"),
						Modified:   baseTime,
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								Status:     model.StatusInProgress,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							ListID:         "store-list-1",
							Title:          "I1",
							Status:         model.StatusInProgress,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "reorder items (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Position: 0,
							},
							{
								Title:    "I2",
								Modified: baseTime,
								Position: 1,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Original",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-2",
								Title:    "I2",
								Modified: baseTime,
							},
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "reorder items (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								ID:       "store-item-2",
								Title:    "I2",
								Modified: baseTime,
							},
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Position: 0,
							},
							{
								Title:    "I2",
								Modified: baseTime,
								Position: 0,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "reorder items external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								ID:         "store-item-2",
								Title:      "I2",
								Modified:   baseTime,
								Position:   0,
								ExternalID: stringPtr("external-task-2"),
							},
							{
								ID:         "store-item-1",
								Title:      "I1",
								Modified:   baseTime,
								Position:   1,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
							{
								Title:    "I2",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       0,
							ListID:         "store-list-1",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       1,
							ListID:         "store-list-1",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I2",
							Position:       0,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							Title:          "I1",
							Position:       1,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "reorder items external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
							{
								Title:    "I2",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Original",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								ID:         "store-item-2",
								Title:      "I2",
								Modified:   baseTime,
								Position:   0,
								ExternalID: stringPtr("external-task-2"),
							},
							{
								ID:         "store-item-1",
								Title:      "I1",
								Modified:   baseTime,
								Position:   1,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Position:       0,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-2"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							ListID:         "store-list-1",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							ListID:         "store-list-1",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-2"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "move item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Position: 0,
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Position: 0,
							},
							{
								Title:    "I2",
								Modified: baseTime,
								Position: 1,
							},
							{
								Title:    "I3",
								Modified: baseTime,
								Position: 2,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-2",
								Title:    "I2",
								Modified: baseTime,
							},
						},
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
							{
								ID:       "store-item-3",
								Title:    "I3",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Status:   model.StatusNotStarted,
							Position: 1,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Status:   model.StatusNotStarted,
							Position: 2,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Status:   model.StatusNotStarted,
							Position: 1,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Status:   model.StatusNotStarted,
							Position: 2,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "move item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								ID:       "store-item-2",
								Title:    "I2",
								Modified: baseTime,
							},
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
							{
								ID:       "store-item-3",
								Title:    "I3",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Position: 0,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Position: 0,
							},
						},
					},
					{
						Name:     "L2",
						Modified: baseTime,
						Position: 1,
						Items: []*model.Item{
							{
								Title:    "I2",
								Modified: baseTime,
								Position: 0,
							},
							{
								Title:    "I3",
								Modified: baseTime,
								Position: 1,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Position: 1,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Status:   model.StatusNotStarted,
							Position: 2,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Position: 1,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Status:   model.StatusNotStarted,
							Position: 2,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "move item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						Position:   0,
						ExternalID: stringPtr("external-list-1"),
						Items:      []*model.Item{},
					},
					{
						Name:       "L2",
						Modified:   baseTime.Add(1),
						Position:   1,
						ExternalID: stringPtr("external-list-2"),
						Items: []*model.Item{
							{
								ID:         "store-item-2",
								Title:      "I2",
								Modified:   baseTime,
								Position:   0,
								ExternalID: stringPtr("external-task-2"),
							},
							{
								ID:         "store-item-1",
								Title:      "I1",
								Modified:   baseTime,
								Position:   1,
								ExternalID: stringPtr("external-task-1"),
							},
							{
								ID:         "store-item-3",
								Title:      "I3",
								Modified:   baseTime,
								Position:   2,
								ExternalID: stringPtr("external-task-3"),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
					{
						Name:     "L2",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I2",
								Modified: baseTime,
							},
							{
								Title:    "I3",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-2",
							Title:          "I2",
							Status:         model.StatusNotStarted,
							Position:       0,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       1,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							ID:             "store-item-3",
							Title:          "I3",
							Status:         model.StatusNotStarted,
							Position:       2,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-3"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I2",
							Status:         model.StatusNotStarted,
							Position:       0,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       1,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							Title:          "I3",
							Status:         model.StatusNotStarted,
							Position:       2,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-3"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "move item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
							{
								Title:    "I2",
								Modified: baseTime,
							},
							{
								Title:    "I3",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								ID:         "store-item-2",
								Title:      "I2",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-2"),
							},
						},
					},
					{
						Name:       "L2",
						ExternalID: stringPtr("external-list-2"),
						Modified:   baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-1",
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-1"),
							},
							{
								ID:         "store-item-3",
								Title:      "I3",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-3"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       0,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							Title:          "I2",
							Status:         model.StatusNotStarted,
							Position:       1,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							Title:          "I3",
							Status:         model.StatusNotStarted,
							Position:       2,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-3"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       0,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Status:         model.StatusNotStarted,
							Position:       1,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
						{
							ID:             "store-item-3",
							Title:          "I3",
							Status:         model.StatusNotStarted,
							Position:       2,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-3"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item and move item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Position: 0,
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
								Status:   model.StatusDone,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1 Original",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusDone,
							ListID: "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusDone,
							ListID: "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item and move item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
								Status:   model.StatusInProgress,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Position: 0,
						Items: []*model.Item{
							{
								Title:    "I1 Original",
								Modified: baseTime,
								Status:   model.StatusDone,
							},
						},
					},
					{
						Name:     "L2",
						Modified: baseTime,
						Position: 1,
						Items:    []*model.Item{},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusInProgress,
							ListID: "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusInProgress,
							ListID: "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item and move item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						Position:   0,
						ExternalID: stringPtr("external-list-1"),
						Items:      []*model.Item{},
					},
					{
						Name:       "L2",
						Modified:   baseTime.Add(1),
						Position:   1,
						ExternalID: stringPtr("external-list-2"),
						Items: []*model.Item{
							{
								Title:      "I1 Updated",
								Modified:   baseTime.Add(1),
								Status:     model.StatusDone,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Original",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
					{
						Name:     "L2",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Updated",
							Status:         model.StatusDone,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusDone,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item and move item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items:    []*model.Item{},
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						Position:   0,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1 Original",
								Modified:   baseTime,
								Status:     model.StatusDone,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
					{
						Name:       "L2",
						Modified:   baseTime,
						Position:   1,
						ExternalID: stringPtr("external-list-2"),
						Items:      []*model.Item{},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Updated",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Status:   model.StatusDeleted,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{},
			wantUpdated:  true,
		},
		{
			name: "delete list (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusDeleted,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						Status:     model.StatusDeleted,
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{},
			wantUpdated:  true,
		},
		{
			name: "delete list external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusDeleted,
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list move item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Status:   model.StatusDeleted,
						Position: 0,
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							ListID: "store-list-2",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list move item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Position: 0,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
					{
						Name:     "L2",
						Modified: baseTime,
						Position: 1,
						Items:    []*model.Item{},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-2",
					Name:   "L2",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							ListID: "store-list-2",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusDeleted,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							ListID: "store-list-2",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list move item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						Status:     model.StatusDeleted,
						Position:   0,
						ExternalID: stringPtr("external-list-1"),
					},
					{
						Name:       "L2",
						Modified:   baseTime.Add(1),
						Position:   1,
						ExternalID: stringPtr("external-list-2"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
					{
						Name:     "L2",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list move item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						Position:   0,
						ExternalID: stringPtr("external-list-1"),
						Items:      []*model.Item{},
					},
					{
						Name:       "L2",
						Modified:   baseTime,
						Position:   1,
						ExternalID: stringPtr("external-list-2"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusDeleted,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete already deleted list (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{})
				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Status:   model.StatusDeleted,
					},
				})

				return dst
			},
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusDeleted,
					Position: 0,
					Items:    []*model.Item{},
				},
			},
			wantUpdated: false,
		},
		{
			name: "delete already deleted list external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{})
				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						Status:     model.StatusDeleted,
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return dst
			},
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusDeleted,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: false,
		},
		{
			name: "delete item (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusDeleted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							ListID: "store-list-1",
							Status: model.StatusDeleted,
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete item external (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								Status:     model.StatusDeleted,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusOpen,
					Items:      []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusOpen,
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: stringPtr("external-task-1"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusOpen,
					Items:      []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusOpen,
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
							Status:         model.StatusDeleted,
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete already deleted item (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusDeleted,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusDeleted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "delete already deleted item external (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								Status:     model.StatusDeleted,
								ExternalID: stringPtr("external-item-1"),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "delete item skipped due to concurrent edit (push)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
								Status:   model.StatusDeleted,
							},
						},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime.Add(2 * time.Hour),
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusDeleted,
							ListID:   "store-list-1",
							Position: 0,
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
							Position: 0,
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "delete item skipped due to concurrent edit (pull)",
			setupSrc: func(t *testing.T) Provider {
				src := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return src
			},
			setupDst: func(t *testing.T) Provider {
				dst := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime.Add(2 * time.Hour),
							},
						},
					},
				})

				return dst
			},
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
							Position: 0,
						},
					},
				},
			},
			wantUpdated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := tt.setupSrc(t)
			dst := tt.setupDst(t)

			var s *Syncer
			if remote, ok := dst.(RemoteProvider); ok {
				s = NewSyncer(src, remote)
			} else if remote, ok := src.(RemoteProvider); ok {
				s = NewSyncer(dst, remote)
			} else {
				t.Fatalf("test must have at least one RemoteProvider")
			}

			syncStart := baseTime.Add(time.Hour)
			updated, err := s.oneWaySync(context.Background(), src, dst, syncStart)
			if err != nil {
				t.Fatalf("oneWaySync failed: %v", err)
			}

			if updated != tt.wantUpdated {
				t.Errorf("updated = %v, want %v", updated, tt.wantUpdated)
			}

			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified", "Created"),
			}

			gotSrcLists, _ := src.ListLists(context.Background())
			if diff := cmp.Diff(tt.wantSrcLists, gotSrcLists, opts...); diff != "" {
				t.Errorf("Source state mismatch (-want +got):\n%s", diff)
			}

			gotDstLists, _ := dst.ListLists(context.Background())
			if diff := cmp.Diff(tt.wantDstLists, gotDstLists, opts...); diff != "" {
				t.Errorf("Destination state mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
