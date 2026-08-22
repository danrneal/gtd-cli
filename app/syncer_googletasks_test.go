package app

import (
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

	"github.com/danrneal/gtd-cli/model"
	"github.com/danrneal/gtd-cli/providers/googletasks"
	"github.com/danrneal/gtd-cli/providers/googletasks/googletaskstest"
)

func TestPushGoogleTasks(t *testing.T) {
	t.Parallel()
	baseTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := []struct {
		name                 string
		setupSqlite          func(t *testing.T) *errorProvider
		setupGoogleTasks     func(t *testing.T) *errorProvider
		wantSqliteLists      []model.List
		wantGoogleTasksLists []model.List
		wantErr              bool
	}{
		{
			name: "success (no updates needed)",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "skips creation of deleted list",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Status:   model.StatusDeleted,
						Modified: baseTime.Add(1),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "skips deleted items during list creation",
			setupSqlite: func(t *testing.T) *errorProvider {
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
							{
								Title:    "I2",
								Modified: baseTime,
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusDeleted,
							Position:       0,
							ListID:         "store-list-1",
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Status:         model.StatusNotStarted,
							Position:       1,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "creates new item in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: new("external-task-1"),
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
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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

				tasks.errUpdateList = errors.New("redundant update list called")

				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-2"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-1"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-2"),
						},
					},
				},
			},
		},
		{
			name: "creates new list with multiple items in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
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
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-2"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-1"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-2"),
						},
					},
				},
			},
		},
		{
			name: "safely anchors new items around cross-list moves",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
					},
					{
						Name:       "L2",
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-2"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Modified:       baseTime.Add(1),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
							{
								Title:    "I2",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       0,
							ListID:         "store-list-2",
							ExternalListID: new("external-list-2"),
							ExternalID:     new("external-task-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Status:         model.StatusNotStarted,
							Position:       1,
							ListID:         "store-list-2",
							ExternalListID: new("external-list-2"),
							ExternalID:     new("external-task-2"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalListID: new("external-list-2"),
							ExternalID:     new("external-task-1"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalListID: new("external-list-2"),
							ExternalID:     new("external-task-2"),
						},
					},
				},
			},
		},
		{
			name: "skips deletion of list due to concurrent edit",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Status:     model.StatusDeleted,
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Original",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "updates list name and content",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Original",
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
					{
						Name:       "L2 Unchanged",
						Modified:   baseTime,
						ExternalID: new("external-list-2"),
					},
					{
						Name:       "L3 Older",
						Modified:   baseTime,
						ExternalID: new("external-list-3"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
					{
						Name:     "L2 Unchanged",
						Modified: baseTime,
					},
					{
						Name:     "L3 Newer",
						Modified: baseTime.Add(1),
					},
				})

				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Original",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
				{
					ID:         "store-list-2",
					Name:       "L2 Unchanged",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-3",
					Name:       "L3 Older",
					Status:     model.StatusOpen,
					Position:   2,
					ExternalID: new("external-list-3"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Original",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
				{
					Name:       "L2 Unchanged",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L3 Newer",
					Status:     model.StatusOpen,
					Position:   2,
					ExternalID: new("external-list-3"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "skips deleted items during list update",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "Deleted Item",
								Status:   model.StatusDeleted,
								Modified: baseTime,
							},
							{
								Title:          "Valid Synced Item Updated",
								Status:         model.StatusNotStarted,
								Modified:       baseTime.Add(1),
								ExternalID:     new("external-task-2"),
								ExternalListID: new("external-list-1"),
							},
							{
								Title:          "Active Item",
								Status:         model.StatusNotStarted,
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
							{
								Title:    "Valid Synced Item Original",
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							ListID:         "store-list-1",
							Title:          "Deleted Item",
							Position:       0,
							Status:         model.StatusDeleted,
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-2",
							ListID:         "store-list-1",
							Title:          "Valid Synced Item Updated",
							Position:       1,
							Status:         model.StatusNotStarted,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-3",
							ListID:         "store-list-1",
							Title:          "Active Item",
							Position:       2,
							Status:         model.StatusNotStarted,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Valid Synced Item Updated",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "Active Item",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "skips deletion of item due to concurrent edit",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Status:         model.StatusDeleted,
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "updates item content",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "I1 Unsynced",
								Modified: baseTime,
							},
							{
								Title:          "I2 Updated",
								Modified:       baseTime.Add(1),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I2 Original",
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Unsynced",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-2"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2 Updated",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Unsynced",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "I2 Updated",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "skips project backfill for projects list",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       model.ListProjects,
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "Project A",
								Modified:       baseTime,
								ProjectTag:     new("project-a"),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "Project A",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Project A",
							Status:         model.StatusOpen,
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "skips project backfill if project id is present",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       model.ListProjects,
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
								ExternalID: new("external-task-1"),
							},
						},
					},
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-2"),
						Items: []*model.Item{
							{
								Title:      "Item with Project",
								Modified:   baseTime,
								ProjectID:  new("store-item-1"),
								ProjectTag: new("project-a"),
								ExternalID: new("external-task-2"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Item with Project",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				tasks.errUpdateItem = errors.New("boom")

				return tasks
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "Project A",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
				{
					ID:         "store-list-2",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							ID:                "store-item-2",
							Title:             "Item with Project",
							Status:            model.StatusNotStarted,
							ListID:            "store-list-2",
							ProjectID:         new("store-item-1"),
							ProjectTag:        new("project-a"),
							ExternalID:        new("external-task-2"),
							ExternalListID:    new("external-list-2"),
							ExternalProjectID: new("external-task-1"),
						},
					},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Project A",
							Status:         model.StatusOpen,
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "Item with Project",
							Status:         model.StatusOpen,
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-2"),
						},
					},
				},
			},
		},
		{
			name: "deletes list in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
					{
						Name:       "L2",
						Status:     model.StatusDeleted,
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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
					ExternalID: new("external-list-2"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-2"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "deletes item in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Status:         model.StatusDeleted,
								Modified:       baseTime.Add(1),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "fails to build source state",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errListLists = errors.New("boom")

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to build destination state",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				tasks.errListLists = errors.New("boom")

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to create list in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				tasks.errCreateList = errors.New("boom")

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to backfill list ID in source",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				sqlite.errUpdateList = errors.New("boom")

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to create item in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
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
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				tasks.errCreateItem = errors.New("boom")

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to backfill item ID in source",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
						},
					},
				})

				sqlite.errUpdateItem = errors.New("boom")

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			name: "fails to update list in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Updated",
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
					},
				})

				tasks.errUpdateList = errors.New("boom")

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to update item in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Updated",
								Modified:       baseTime.Add(1),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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

				tasks.errUpdateItem = errors.New("boom")

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete list from destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Status:     model.StatusDeleted,
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				tasks.errDeleteList = errors.New("boom")

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete list from source",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Status:     model.StatusDeleted,
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
					},
				})

				sqlite.errDeleteList = errors.New("boom")

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Status:         model.StatusDeleted,
								Modified:       baseTime.Add(1),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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

				tasks.errDeleteItem = errors.New("boom")

				return tasks
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete item from source",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Status:         model.StatusDeleted,
								Modified:       baseTime.Add(1),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				sqlite.errDeleteItem = errors.New("boom")

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			err := syncer.Push(t.Context(), syncStart)
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

			gotSqliteLists, err := sqlite.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list sqlite lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantSqliteLists, gotSqliteLists, opts...); diff != "" {
				t.Errorf("Sqlite state mismatch (-want +got):\n%s", diff)
			}

			gotGoogleTasksLists, err := googleTasks.ListLists(t.Context())
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
		setupGoogleTasks     func(t *testing.T) *errorProvider
		setupSqlite          func(t *testing.T) *errorProvider
		wantGoogleTasksLists []model.List
		wantSqliteLists      []model.List
		wantUpdated          bool
		wantErr              bool
	}{
		{
			name: "success (no updates needed)",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
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

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: new("external-task-1"),
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "creates new list in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "creates new item in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "I1",
								Modified:   baseTime,
								ExternalID: new("external-task-1"),
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "creates new list with multiple items in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "safely anchors new items around cross-list moves",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
					{
						Name:       "L2",
						Modified:   baseTime,
						ExternalID: new("external-list-2"),
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-2"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-2"),
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							Position:       0,
							ListID:         "store-list-2",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-2"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Status:         model.StatusNotStarted,
							Position:       1,
							ListID:         "store-list-2",
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-2"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "updates list name and content",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L2 Updated",
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
					{
						Name:       "L2 Original",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Original",
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L2 Updated",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Original",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2 Updated",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Original",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "updates list position only",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L2",
						Modified: baseTime,
					},
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-2"),
					},
					{
						Name:       "L2",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "promotes item to in progress during list update",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Updated",
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
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Original",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Status:         model.StatusInProgress,
								Modified:       baseTime,
								ExternalID:     new("external-task-2"),
								ExternalListID: new("external-list-1"),
							},
							{
								Title:          "I2",
								Status:         model.StatusNotStarted,
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "I1",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							Status:         model.StatusInProgress,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "preserves tombstoned items in destination during pull",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "Already Deleted Task",
								Status:     model.StatusDeleted,
								Modified:   baseTime,
								ExternalID: new("external-task-1"),
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "Already Deleted Task",
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "updates item content",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Original",
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Updated",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "promotes item to in progress during item update",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Original",
								Status:         model.StatusInProgress,
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Updated",
							Status:         model.StatusInProgress,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "skips project backfill entirely when all projects are linked",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Trap: Already Has ProjectID",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       model.ListProjects,
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
								ExternalID: new("external-task-1"),
							},
						},
					},
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-2"),
						Items: []*model.Item{
							{
								Title:      "Trap: Already Has ProjectID",
								Modified:   baseTime,
								ProjectID:  new("store-item-1"),
								ProjectTag: new("project-a"),
								ExternalID: new("external-task-2"),
							},
						},
					},
				})

				sqlite.errUpdateItem = errors.New("boom")

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Project A",
							Status:         model.StatusOpen,
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "Trap: Already Has ProjectID",
							Status:         model.StatusOpen,
							Position:       0,
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-2"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "Project A",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
				{
					ID:         "store-list-2",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							ID:                "store-item-2",
							Title:             "Trap: Already Has ProjectID",
							Status:            model.StatusNotStarted,
							Position:          0,
							ListID:            "store-list-2",
							ProjectID:         new("store-item-1"),
							ProjectTag:        new("project-a"),
							ExternalID:        new("external-task-2"),
							ExternalListID:    new("external-list-2"),
							ExternalProjectID: new("external-task-1"),
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "skips project backfill for projects list",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:       model.ListProjects,
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "Project A",
								Modified:       baseTime,
								ProjectTag:     new("project-a"),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       model.ListProjects,
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "Project A",
								Modified:       baseTime,
								ProjectTag:     new("project-a"),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Project A",
							Status:         model.StatusOpen,
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "Project A",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "continues backfill loop after optimized item",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Item 1",
								Modified:   baseTime,
								ProjectTag: new("project-old"),
							},
							{
								Title:      "Item 2",
								Modified:   baseTime,
								ProjectTag: new("project-new"),
							},
						},
					},
					{
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Project New",
								Modified:   baseTime,
								ProjectTag: new("project-new"),
							},
							{
								Title:      "Project Old",
								Modified:   baseTime,
								ProjectTag: new("project-old"),
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       model.ListProjects,
						Modified:   baseTime,
						ExternalID: new("external-list-2"),
						Items: []*model.Item{
							{
								Title:      "Project Old",
								Modified:   baseTime,
								ProjectTag: new("project-old"),
								ExternalID: new("external-task-4"),
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
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Item 1",
							Status:         model.StatusOpen,
							Position:       0,
							ProjectTag:     new("project-old"),
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "Item 2",
							Status:         model.StatusOpen,
							Position:       1,
							ProjectTag:     new("project-new"),
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
				{
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "Project New",
							Status:         model.StatusOpen,
							Position:       0,
							ProjectTag:     new("project-new"),
							ExternalID:     new("external-task-3"),
							ExternalListID: new("external-list-2"),
						},
						{
							Title:          "Project Old",
							Status:         model.StatusOpen,
							Position:       1,
							ProjectTag:     new("project-old"),
							ExternalID:     new("external-task-4"),
							ExternalListID: new("external-list-2"),
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-2",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:                "store-item-2",
							Title:             "Item 1",
							Status:            model.StatusNotStarted,
							Position:          0,
							ListID:            "store-list-2",
							ProjectID:         new("store-item-1"),
							ProjectTag:        new("project-old"),
							ExternalID:        new("external-task-1"),
							ExternalListID:    new("external-list-1"),
							ExternalProjectID: new("external-task-4"),
						},
						{
							ID:                "store-item-3",
							Title:             "Item 2",
							Status:            model.StatusNotStarted,
							Position:          1,
							ListID:            "store-list-2",
							ProjectID:         new("store-item-4"),
							ProjectTag:        new("project-new"),
							ExternalID:        new("external-task-2"),
							ExternalListID:    new("external-list-1"),
							ExternalProjectID: new("external-task-3"),
						},
					},
				},
				{
					ID:         "store-list-1",
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-4",
							Title:          "Project New",
							Status:         model.StatusNotStarted,
							Position:       0,
							ListID:         "store-list-1",
							ProjectTag:     new("project-new"),
							ExternalID:     new("external-task-3"),
							ExternalListID: new("external-list-2"),
						},
						{
							ID:             "store-item-1",
							Title:          "Project Old",
							Status:         model.StatusNotStarted,
							Position:       1,
							ListID:         "store-list-1",
							ProjectTag:     new("project-old"),
							ExternalID:     new("external-task-4"),
							ExternalListID: new("external-list-2"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "backfills project id when projects list is created last",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "Normal Item",
								Modified: baseTime,
							},
							{
								Title:      "Item with Project",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
					{
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Normal Item",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "Item with Project",
							Status:         model.StatusOpen,
							Position:       1,
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
				{
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							Title:          "Project A",
							Status:         model.StatusOpen,
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-3"),
							ExternalListID: new("external-list-2"),
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
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "Normal Item",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:                "store-item-2",
							Title:             "Item with Project",
							Status:            model.StatusNotStarted,
							Position:          1,
							ListID:            "store-list-1",
							ProjectID:         new("store-item-3"),
							ProjectTag:        new("project-a"),
							ExternalID:        new("external-task-2"),
							ExternalListID:    new("external-list-1"),
							ExternalProjectID: new("external-task-3"),
						},
					},
				},
				{
					ID:         "store-list-2",
					Name:       model.ListProjects,
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: new("external-list-2"),
					Items: []*model.Item{
						{
							ID:             "store-item-3",
							Title:          "Project A",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-3"),
							ExternalListID: new("external-list-2"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "skips already deleted list during deletion phase",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
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
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
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
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Status:     model.StatusOpen,
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "skips already deleted item during deletion phase",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Status:     model.StatusOpen,
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "I1",
								Status:   model.StatusDeleted,
								Modified: baseTime,
							},
							{
								Title:          "I2 To Be Deleted",
								Status:         model.StatusNotStarted,
								Modified:       baseTime,
								ExternalID:     new("external-task-2"),
								ExternalListID: new("external-list-1"),
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2 To Be Deleted",
							Position:       1,
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "skips deletion of item with empty key",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
							{
								Title:          "I2 To Be Deleted",
								Modified:       baseTime,
								ExternalID:     new("external-task-2"),
								ExternalListID: new("external-list-1"),
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2 To Be Deleted",
							Position:       1,
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "deletes item in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "fails to build source state",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				tasks.errListLists = errors.New("boom")

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to build destination state",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errListLists = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to create list in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errCreateList = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to create item in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				sqlite.errCreateItem = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to update list in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Original",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
					},
				})

				sqlite.errUpdateList = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to update item in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
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
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Original",
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				sqlite.errUpdateItem = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to backfill project id in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Item with Project",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
					{
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errUpdateItem = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to mark list as deleted in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
					},
				})

				sqlite.errUpdateList = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to mark item as deleted in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				sqlite.errUpdateItem = errors.New("boom")

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
			updated, err := syncer.Pull(t.Context(), syncStart)
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

			gotGoogleTasksLists, err := googleTasks.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list google tasks lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantGoogleTasksLists, gotGoogleTasksLists, opts...); diff != "" {
				t.Errorf("Google Tasks state mismatch (-want +got):\n%s", diff)
			}

			gotSqliteLists, err := sqlite.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list sqlite lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantSqliteLists, gotSqliteLists, opts...); diff != "" {
				t.Errorf("Sqlite state mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func setupTestGoogleTasks(t *testing.T, lists []model.List) *errorProvider {
	fakeGoogleTasks := googletaskstest.NewFakeGoogleTasks(t)
	mockHTTPClient := &http.Client{
		Transport: fakeGoogleTasks,
	}

	tasksService, err := tasks.NewService(t.Context(), option.WithHTTPClient(mockHTTPClient))
	if err != nil {
		t.Fatalf("failed to create tasks service: %v", err)
	}

	logger := slog.New(slog.DiscardHandler)
	client := googletasks.NewClient(tasksService, 30*time.Second, logger)

	for _, list := range lists {
		if err := client.CreateList(t.Context(), &list); err != nil {
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
			if err := client.CreateItem(t.Context(), item, prevItemID); err != nil {
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

	testGoogleTasks := &errorProvider{
		Provider: client,
	}

	return testGoogleTasks
}
