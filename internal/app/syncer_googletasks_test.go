package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/providers/googletasks"
	"github.com/danrneal/gtd.nvim/internal/providers/googletasks/googletaskstest"
)

func TestPushGoogleTasks(t *testing.T) {
	t.Parallel()
	baseTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := []struct {
		name                 string
		setupSqlite          func(t *testing.T) Provider
		setupGoogleTasks     func(t *testing.T) RemoteProvider
		wantSqliteLists      []model.List
		wantGoogleTasksLists []model.List
		wantErr              bool
	}{
		{
			name: "success (no updates needed)",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Modified:       baseTime,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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

				return tasks
			},
			wantSqliteLists: []model.List{
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
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "skips creation of deleted list",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Status:   model.StatusDeleted,
						Modified: baseTime.Add(1),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusDeleted,
					Items:  []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{},
		},
		{
			name: "creates new list in destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "skips deleted items during list creation",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Status:   model.StatusDeleted,
								Modified: baseTime,
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
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
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "creates new item in destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			wantSqliteLists: []model.List{
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
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
					},
				},
			},
		},
		{
			name: "creates new list with multiple items in destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
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

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantSqliteLists: []model.List{
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
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-2"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-task-2"),
						},
					},
				},
			},
		},
		{
			name: "safely anchors new items around cross-list moves",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
					},
					{
						Name:       "L2",
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-2"),
						Items: []*model.Item{
							{
								ID:             "store-item-1",
								Title:          "I1",
								Modified:       baseTime.Add(1),
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
							{
								ID:       "store-item-2",
								Title:    "I2",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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
					},
				})

				return tasks
			},
			wantSqliteLists: []model.List{
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
					},
				},
			},
			wantGoogleTasksLists: []model.List{
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
							Status:         model.StatusOpen,
							Position:       0,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-1"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-task-2"),
						},
					},
				},
			},
		},
		{
			name: "skips deletion of list due to concurrent edit",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Status:     model.StatusDeleted,
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime.Add(2 * time.Hour),
					},
				})

				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusDeleted,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Original",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "updates list name and content",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Original",
								Modified:       baseTime,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Original",
								Modified: baseTime,
							},
						},
					},
				})

				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Original",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Original",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "skips deleted items during list update",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "Active Item",
								Status:         model.StatusNotStarted,
								Modified:       baseTime,
								Position:       0,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
							{
								Title:    "Deleted Item",
								Status:   model.StatusDeleted,
								Modified: baseTime,
								Position: 1,
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "Active Item",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return tasks
			},
			wantSqliteLists: []model.List{
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
						{
							ID:     "store-item-2",
							ListID: "store-list-1",
							Title:  "Deleted Item",
							Status: model.StatusDeleted,
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Active Item",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "skips deletion of item due to concurrent edit",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Status:         model.StatusDeleted,
								Modified:       baseTime,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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

				return tasks
			},
			wantSqliteLists: []model.List{
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
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "updates item content",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Updated",
								Modified:       baseTime.Add(1),
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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

				return tasks
			},
			wantSqliteLists: []model.List{
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
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "deletes list in destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Status:     model.StatusDeleted,
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			wantSqliteLists:      []model.List{},
			wantGoogleTasksLists: []model.List{},
		},
		{
			name: "deletes item in destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Status:         model.StatusDeleted,
								Modified:       baseTime.Add(1),
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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

				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "fails to build source state",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider:     setupTestSQLite(t, []model.List{}),
					errListLists: errors.New("boom"),
				}

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to build destination state",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := &errorProvider{
					Provider:     setupTestGoogleTasks(t, []model.List{}),
					errListLists: errors.New("boom"),
				}

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to create list in destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := &errorProvider{
					Provider:      setupTestGoogleTasks(t, []model.List{}),
					errCreateList: errors.New("boom"),
				}

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to backfill list ID in source",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider: setupTestSQLite(t, []model.List{
						{
							Name:     "L1",
							Modified: baseTime,
						},
					}),
					errUpdateList: errors.New("boom"),
				}

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to create item in destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := &errorProvider{
					Provider: setupTestGoogleTasks(t, []model.List{
						{
							Name:     "L1",
							Modified: baseTime,
						},
					}),
					errCreateItem: errors.New("boom"),
				}

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to backfill item ID in source",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider: setupTestSQLite(t, []model.List{
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
					}),
					errUpdateItem: errors.New("boom"),
				}

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to update list in destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := &errorProvider{
					Provider: setupTestGoogleTasks(t, []model.List{
						{
							Name:     "L1 Original",
							Modified: baseTime,
						},
					}),
					errUpdateList: errors.New("boom"),
				}

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to update item in destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Updated",
								Modified:       baseTime.Add(1),
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := &errorProvider{
					Provider: setupTestGoogleTasks(t, []model.List{
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
					}),
					errUpdateItem: errors.New("boom"),
				}

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete list from destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Status:     model.StatusDeleted,
						Modified:   baseTime.Add(1),
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := &errorProvider{
					Provider: setupTestGoogleTasks(t, []model.List{
						{
							Name:     "L1",
							Modified: baseTime,
						},
					}),
					errDeleteList: errors.New("boom"),
				}

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete list from source",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider: setupTestSQLite(t, []model.List{
						{
							Name:       "L1",
							Status:     model.StatusDeleted,
							Modified:   baseTime.Add(1),
							ExternalID: stringPtr("external-list-1"),
						},
					}),
					errDeleteList: errors.New("boom"),
				}

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete item from destination",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Status:         model.StatusDeleted,
								Modified:       baseTime.Add(1),
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := &errorProvider{
					Provider: setupTestGoogleTasks(t, []model.List{
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
					}),
					errDeleteItem: errors.New("boom"),
				}

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete item from source",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider: setupTestSQLite(t, []model.List{
						{
							Name:       "L1",
							Modified:   baseTime,
							ExternalID: stringPtr("external-list-1"),
							Items: []*model.Item{
								{
									Title:          "I1",
									Status:         model.StatusDeleted,
									Modified:       baseTime.Add(1),
									ExternalID:     stringPtr("external-task-1"),
									ExternalListID: stringPtr("external-list-1"),
								},
							},
						},
					}),
					errDeleteItem: errors.New("boom"),
				}

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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

				return tasks
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sqlite := tt.setupSqlite(t)
			googleTasks := tt.setupGoogleTasks(t)

			syncer := NewSyncer(sqlite, googleTasks)

			syncStart := baseTime.Add(time.Hour)
			err := syncer.Push(context.Background(), syncStart)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Push error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified", "Created"),
			}

			gotSqliteLists, err := sqlite.ListLists(context.Background())
			if err != nil {
				t.Fatalf("failed to list sqlite lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantSqliteLists, gotSqliteLists, opts...); diff != "" {
				t.Errorf("Sqlite state mismatch (-want +got):\n%s", diff)
			}

			gotGoogleTasksLists, err := googleTasks.ListLists(context.Background())
			if err != nil {
				t.Fatalf("failed to list google tasks lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantGoogleTasksLists, gotGoogleTasksLists, opts...); diff != "" {
				t.Errorf("Google Tasks state mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPullGoogleTasks(t *testing.T) {
	t.Parallel()
	baseTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := []struct {
		name                 string
		setupGoogleTasks     func(t *testing.T) RemoteProvider
		setupSqlite          func(t *testing.T) Provider
		wantGoogleTasksLists []model.List
		wantSqliteLists      []model.List
		wantUpdated          bool
		wantErr              bool
	}{
		{
			name: "success (no updates needed)",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
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
			wantUpdated: false,
		},
		{
			name: "creates new list in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
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
			name: "creates new item in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
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
			name: "creates new list with multiple items in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
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

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalID:     stringPtr("external-task-2"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
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
						{
							ID:             "store-item-2",
							Title:          "I2",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-task-2"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "safely anchors new items around cross-list moves",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime.Add(1),
							},
							{
								Title:    "I2",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Modified:       baseTime,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
					{
						Name:       "L2",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-2"),
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
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
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-2"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalID:     stringPtr("external-task-2"),
							ExternalListID: stringPtr("external-list-2"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
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
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-2"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Status:         model.StatusNotStarted,
							Position:       1,
							ListID:         "store-list-2",
							ExternalID:     stringPtr("external-task-2"),
							ExternalListID: stringPtr("external-list-2"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "updates list name and content",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1 Original",
								Modified: baseTime,
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Original",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Original",
								Modified:       baseTime,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Original",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Original",
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
			name: "updates list position only",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
					{
						Name:     "L2",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
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
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
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
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "promotes item to in progress during list update",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Original",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Status:         model.StatusInProgress,
								Modified:       baseTime,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusInProgress,
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
			name: "updates item content",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Original",
								Modified:       baseTime,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
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
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "promotes item to in progress during item update",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Original",
								Status:         model.StatusInProgress,
								Modified:       baseTime,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusOpen,
							ExternalID:     stringPtr("external-task-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Updated",
							Status:         model.StatusInProgress,
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
			name: "skips already deleted list during deletion phase",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Status:   model.StatusDeleted,
						Modified: baseTime,
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusDeleted,
					Items:  []*model.Item{},
				},
			},
			wantUpdated: false,
		},
		{
			name: "skips deletion of list with empty key",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{},
			wantSqliteLists: []model.List{
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
			name: "deletes list in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Status:     model.StatusOpen,
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusDeleted,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "skips already deleted item during deletion phase",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Status:     model.StatusOpen,
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "I1",
								Status:   model.StatusDeleted,
								Modified: baseTime,
							},
						},
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
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
			name: "skips deletion of item with empty key",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
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
			wantUpdated: false,
		},
		{
			name: "deletes item in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: stringPtr("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Modified:       baseTime,
								ExternalID:     stringPtr("external-task-1"),
								ExternalListID: stringPtr("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
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
			wantUpdated: true,
		},
		{
			name: "fails to build source state",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := &errorProvider{
					Provider:     setupTestGoogleTasks(t, []model.List{}),
					errListLists: errors.New("boom"),
				}

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to build destination state",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider:     setupTestSQLite(t, []model.List{}),
					errListLists: errors.New("boom"),
				}

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to create list in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider:      setupTestSQLite(t, []model.List{}),
					errCreateList: errors.New("boom"),
				}

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to create item in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider: setupTestSQLite(t, []model.List{
						{
							Name:     "L1",
							Modified: baseTime,
						},
					}),
					errCreateItem: errors.New("boom"),
				}

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to update list in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider: setupTestSQLite(t, []model.List{
						{
							Name:       "L1 Original",
							Modified:   baseTime,
							ExternalID: stringPtr("external-list-1"),
						},
					}),
					errUpdateList: errors.New("boom"),
				}

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to update item in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider: setupTestSQLite(t, []model.List{
						{
							Name:       "L1",
							Modified:   baseTime,
							ExternalID: stringPtr("external-list-1"),
							Items: []*model.Item{
								{
									Title:          "I1 Original",
									Modified:       baseTime,
									ExternalID:     stringPtr("external-task-1"),
									ExternalListID: stringPtr("external-list-1"),
								},
							},
						},
					}),
					errUpdateItem: errors.New("boom"),
				}

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to mark list as deleted in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider: setupTestSQLite(t, []model.List{
						{
							Name:       "L1",
							Modified:   baseTime,
							ExternalID: stringPtr("external-list-1"),
						},
					}),
					errUpdateList: errors.New("boom"),
				}

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to mark item as deleted in destination",
			setupGoogleTasks: func(t *testing.T) RemoteProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) Provider {
				sqlite := &errorProvider{
					Provider: setupTestSQLite(t, []model.List{
						{
							Name:       "L1",
							Modified:   baseTime,
							ExternalID: stringPtr("external-list-1"),
							Items: []*model.Item{
								{
									Title:          "I1",
									Modified:       baseTime,
									ExternalID:     stringPtr("external-task-1"),
									ExternalListID: stringPtr("external-list-1"),
								},
							},
						},
					}),
					errUpdateItem: errors.New("boom"),
				}

				return sqlite
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			googleTasks := tt.setupGoogleTasks(t)
			sqlite := tt.setupSqlite(t)

			syncer := NewSyncer(sqlite, googleTasks)

			syncStart := baseTime.Add(time.Hour)
			updated, err := syncer.Pull(context.Background(), syncStart)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Pull error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil {
				return
			}

			if updated != tt.wantUpdated {
				t.Errorf("updated = %v, want %v", updated, tt.wantUpdated)
			}

			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified", "Created"),
			}

			gotGoogleTasksLists, err := googleTasks.ListLists(context.Background())
			if err != nil {
				t.Fatalf("failed to list google tasks lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantGoogleTasksLists, gotGoogleTasksLists, opts...); diff != "" {
				t.Errorf("Google Tasks state mismatch (-want +got):\n%s", diff)
			}

			gotSqliteLists, err := sqlite.ListLists(context.Background())
			if err != nil {
				t.Fatalf("failed to list sqlite lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantSqliteLists, gotSqliteLists, opts...); diff != "" {
				t.Errorf("Sqlite state mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

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

		prevItemID := ""
		for _, item := range list.Items {
			item.ExternalListID = list.ExternalID
			if err := client.CreateItem(context.Background(), item, prevItemID); err != nil {
				t.Fatalf("failed to create item: %v", err)
			}

			prevItemID = *item.ExternalID

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
