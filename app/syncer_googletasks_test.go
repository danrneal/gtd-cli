package app

import (
	"errors"
	"io/fs"
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
		setupSQLite          func(t *testing.T) *errorProvider
		setupGoogleTasks     func(t *testing.T) *errorProvider
		wantSQLiteLists      []model.List
		wantGoogleTasksLists []model.List
		wantErr              error
	}{
		{
			name: "success (no updates needed)",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Status:   model.StatusDeleted,
						Modified: baseTime.Add(1),
					},
					{
						Name:     "L2",
						Status:   model.StatusOpen,
						Modified: baseTime.Add(1),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 0,
					Status:   model.StatusDeleted,
					Items:    []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L2",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "creates new list in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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
				return tasks
			},
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			name: "prepends new item in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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
								Title:      "I2",
								Modified:   baseTime,
								ExternalID: new("external-task-1"),
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
								Title:    "I2",
								Modified: baseTime,
							},
						},
					},
				})

				tasks.errUpdateList = errors.New("redundant update list called")

				return tasks
			},
			wantSQLiteLists: []model.List{
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
							ExternalID:     new("external-task-2"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-1"),
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
							ExternalID:     new("external-task-2"),
						},
						{
							Title:          "I2",
							Status:         model.StatusOpen,
							Position:       1,
							ExternalListID: new("external-list-1"),
							ExternalID:     new("external-task-1"),
						},
					},
				},
			},
		},
		{
			name: "creates new list with multiple items in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
				})

				return tasks
			},
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
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
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Updated",
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
		},
		{
			name: "skips list update when destination is newer",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Older",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
					},
				})

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Newer",
						Modified: baseTime.Add(1),
					},
				})

				return tasks
			},
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Older",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1 Newer",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
		},
		{
			name: "skips deleted items when reordering list items",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "Deleted Item",
								Status:   model.StatusDeleted,
								Modified: baseTime,
							},
							{
								Title:          "Item 2",
								Status:         model.StatusNotStarted,
								Modified:       baseTime,
								ExternalID:     new("external-task-2"),
								ExternalListID: new("external-list-1"),
							},
							{
								Title:          "Item 1",
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
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "Item 1",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
							{
								Title:    "Item 2",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				tasks.errUpdateItem = errors.New("unexpected update item called")

				return tasks
			},
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
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
							Title:          "Item 2",
							Position:       1,
							Status:         model.StatusNotStarted,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-3",
							ListID:         "store-list-1",
							Title:          "Item 1",
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
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "Item 2",
							Status:         model.StatusOpen,
							Position:       0,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "Item 1",
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
			name: "skips deleted items when updating items",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "Deleted Item",
								Status:   model.StatusDeleted,
								Modified: baseTime,
							},
							{
								Title:          "I1 Updated",
								Status:         model.StatusNotStarted,
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
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				tasks.errUpdateList = errors.New("unexpected update list called")

				return tasks
			},
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
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
							Title:          "I1 Updated",
							Position:       1,
							Status:         model.StatusNotStarted,
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
							Title:          "I1 Updated",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "skips deletion of item due to concurrent edit",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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

				return tasks
			},
			wantSQLiteLists: []model.List{
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
		},
		{
			name: "skips item update when destination is newer",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1 Older",
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
								Title:    "I1 Newer",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return tasks
			},
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Older",
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
							Title:          "I1 Newer",
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "skips project backfill for projects list",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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

				return tasks
			},
			wantSQLiteLists:      []model.List{},
			wantGoogleTasksLists: []model.List{},
		},
		{
			name: "deletes item in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			name: "prepends item and deletes trailing item in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:    "I0",
								Modified: baseTime.Add(1),
							},
							{
								Title:          "I1",
								Modified:       baseTime,
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
							{
								Title:          "I2",
								Modified:       baseTime,
								ExternalID:     new("external-task-2"),
								ExternalListID: new("external-list-1"),
							},
							{
								Title:          "I3",
								Status:         model.StatusDeleted,
								Modified:       baseTime.Add(1),
								ExternalID:     new("external-task-3"),
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

				return tasks
			},
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-1",
							Title:          "I0",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-4"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I1",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-3",
							Title:          "I2",
							Position:       2,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-2"),
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
							Title:          "I0",
							Position:       0,
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-4"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "I1",
							Position:       1,
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "I2",
							Position:       2,
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
		},
		{
			name: "fails to build source state",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errListLists = fs.ErrPermission

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to build destination state",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				tasks.errListLists = fs.ErrPermission

				return tasks
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to create list in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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
				tasks.errCreateList = fs.ErrPermission

				return tasks
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to backfill list ID in source",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				sqlite.errUpdateList = fs.ErrPermission

				return sqlite
			},
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to create item in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				tasks.errCreateItem = fs.ErrPermission

				return tasks
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to backfill item ID in source",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				sqlite.errUpdateItem = fs.ErrPermission

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
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to update list in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				tasks.errUpdateList = fs.ErrPermission

				return tasks
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to update item in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				tasks.errUpdateItem = fs.ErrPermission

				return tasks
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to permanently delete list from destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				tasks.errDeleteList = fs.ErrPermission

				return tasks
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to permanently delete list from source",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Status:     model.StatusDeleted,
						Modified:   baseTime.Add(1),
						ExternalID: new("external-list-1"),
					},
				})

				sqlite.errDeleteList = fs.ErrPermission

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
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to permanently delete item from destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				tasks.errDeleteItem = fs.ErrPermission

				return tasks
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to permanently delete item from source",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				sqlite.errDeleteItem = fs.ErrPermission

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
			wantErr: fs.ErrPermission,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sqlite := tt.setupSQLite(t)
			googleTasks := tt.setupGoogleTasks(t)

			syncer := NewSyncer(sqlite, googleTasks)

			syncStart := baseTime.Add(time.Hour)
			err := syncer.Push(t.Context(), syncStart)

			errOpts := []cmp.Option{
				cmpopts.EquateErrors(),
			}

			if diff := cmp.Diff(tt.wantErr, err, errOpts...); diff != "" {
				t.Fatalf("Push() error mismatch (-want +got):\n%s", diff)
			}

			if err != nil {
				return
			}

			listOpts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified", "Created"),
			}

			gotSQLiteLists, err := sqlite.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list sqlite lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantSQLiteLists, gotSQLiteLists, listOpts...); diff != "" {
				t.Errorf("SQLite state mismatch (-want +got):\n%s", diff)
			}

			gotGoogleTasksLists, err := googleTasks.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list google tasks lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantGoogleTasksLists, gotGoogleTasksLists, listOpts...); diff != "" {
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
		setupSQLite          func(t *testing.T) *errorProvider
		wantGoogleTasksLists []model.List
		wantSQLiteLists      []model.List
		wantUpdated          bool
		wantErr              error
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			name: "preserves updated status across subsequent unchanged list",
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
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
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							Title:          "I1",
							Position:       0,
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
				{
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-2"),
					Items:      []*model.Item{},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Position:   0,
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
					},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-2"),
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Original",
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
					Name:       "L1 Updated",
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
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
							Title:          "Item with Project",
							Status:         model.StatusOpen,
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-1"),
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
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-2"),
						},
					},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:                "store-item-1",
							Title:             "Item with Project",
							Status:            model.StatusNotStarted,
							ListID:            "store-list-1",
							ProjectID:         new("store-item-2"),
							ProjectTag:        new("project-a"),
							ExternalID:        new("external-task-1"),
							ExternalListID:    new("external-list-1"),
							ExternalProjectID: new("external-task-2"),
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
							ID:             "store-item-2",
							Title:          "Project A",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ProjectTag:     new("project-a"),
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-2"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "continues backfill loop after item without project",
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return sqlite
			},
			wantGoogleTasksLists: []model.List{},
			wantSQLiteLists: []model.List{
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
			name: "skips unlinked list before synced list",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L2",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
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
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "deletes list in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantGoogleTasksLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items:      []*model.Item{},
				},
			},
			wantSQLiteLists: []model.List{
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
			name: "prepends item and deletes trailing item in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I0",
								Modified: baseTime.Add(1),
							},
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "I1",
								Modified:       baseTime,
								ExternalID:     new("external-task-2"),
								ExternalListID: new("external-list-1"),
							},
							{
								Title:          "I2",
								Modified:       baseTime,
								ExternalID:     new("external-task-3"),
								ExternalListID: new("external-list-1"),
							},
							{
								Title:          "I3",
								Modified:       baseTime,
								ExternalID:     new("external-task-99"),
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
							Title:          "I0",
							Position:       0,
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "I1",
							Position:       1,
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
						{
							Title:          "I2",
							Position:       2,
							Status:         model.StatusOpen,
							ExternalID:     new("external-task-3"),
							ExternalListID: new("external-list-1"),
						},
					},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: new("external-list-1"),
					Items: []*model.Item{
						{
							ID:             "store-item-4",
							Title:          "I0",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-1"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       1,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-2"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       2,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-3"),
							ExternalListID: new("external-list-1"),
						},
						{
							ID:             "store-item-3",
							Title:          "I3",
							Position:       3,
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalID:     new("external-task-99"),
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
				tasks.errListLists = fs.ErrPermission

				return tasks
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to build destination state",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errListLists = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errCreateList = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				sqlite.errCreateItem = fs.ErrPermission

				return sqlite
			},
			wantUpdated: true,
			wantErr:     fs.ErrPermission,
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Original",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
					},
				})

				sqlite.errUpdateList = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
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
			setupSQLite: func(t *testing.T) *errorProvider {
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

				sqlite.errUpdateItem = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to update item after updating list in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1 Original",
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

				sqlite.errUpdateItem = fs.ErrPermission

				return sqlite
			},
			wantUpdated: true,
			wantErr:     fs.ErrPermission,
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errUpdateItem = fs.ErrPermission

				return sqlite
			},
			wantUpdated: true,
			wantErr:     fs.ErrPermission,
		},
		{
			name: "fails to backfill project id when lists already synced",
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
				})

				return tasks
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
						Items: []*model.Item{
							{
								Title:          "Item with Project",
								Modified:       baseTime.Add(1),
								ProjectTag:     new("project-a"),
								ExternalID:     new("external-task-1"),
								ExternalListID: new("external-list-1"),
							},
						},
					},
				})

				sqlite.errUpdateItem = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to mark list as deleted in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{})
				return tasks
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L1",
						Modified:   baseTime,
						ExternalID: new("external-list-1"),
					},
				})

				sqlite.errUpdateList = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to mark list as deleted after creating list in destination",
			setupGoogleTasks: func(t *testing.T) *errorProvider {
				tasks := setupTestGoogleTasks(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return tasks
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:       "L2",
						Modified:   baseTime,
						ExternalID: new("external-list-99"),
					},
				})

				sqlite.errUpdateList = fs.ErrPermission

				return sqlite
			},
			wantUpdated: true,
			wantErr:     fs.ErrPermission,
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
			setupSQLite: func(t *testing.T) *errorProvider {
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

				sqlite.errUpdateItem = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			googleTasks := tt.setupGoogleTasks(t)
			sqlite := tt.setupSQLite(t)

			syncer := NewSyncer(sqlite, googleTasks)

			syncStart := baseTime.Add(time.Hour)
			updated, err := syncer.Pull(t.Context(), syncStart)

			errOpts := []cmp.Option{
				cmpopts.EquateErrors(),
			}

			if diff := cmp.Diff(tt.wantErr, err, errOpts...); diff != "" {
				t.Fatalf("Pull() error mismatch (-want +got):\n%s", diff)
			}

			if updated != tt.wantUpdated {
				t.Errorf("updated = %v, want %v", updated, tt.wantUpdated)
			}

			if err != nil {
				return
			}

			listOpts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified", "Created"),
			}

			gotGoogleTasksLists, err := googleTasks.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list google tasks lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantGoogleTasksLists, gotGoogleTasksLists, listOpts...); diff != "" {
				t.Errorf("Google Tasks state mismatch (-want +got):\n%s", diff)
			}

			gotSQLiteLists, err := sqlite.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list sqlite lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantSQLiteLists, gotSQLiteLists, listOpts...); diff != "" {
				t.Errorf("SQLite state mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func setupTestGoogleTasks(t *testing.T, lists []model.List) *errorProvider {
	fakeGoogleTasks := googletaskstest.NewFake(t)
	mockHTTPClient := &http.Client{
		Transport: fakeGoogleTasks,
	}

	opts := []option.ClientOption{
		option.WithHTTPClient(mockHTTPClient),
	}

	tasksService, err := tasks.NewService(t.Context(), opts...)
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
