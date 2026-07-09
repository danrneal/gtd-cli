package app

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/danrneal/gtd-cli/internal/model"
	"github.com/danrneal/gtd-cli/internal/providers/markdown"
)

func TestPushMarkdown(t *testing.T) {
	t.Parallel()
	baseTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		setupSqlite       func(t *testing.T) *errorProvider
		setupMarkdown     func(t *testing.T) *errorProvider
		wantSqliteLists   []model.List
		wantMarkdownLists []model.List
		wantErr           bool
	}{
		{
			name: "success (no updates needed)",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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

				return md
			},
			wantSqliteLists: []model.List{
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
			wantMarkdownLists: []model.List{
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusDeleted,
					Items:  []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{},
		},
		{
			name: "creates new list in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime,
					},
				})

				return md
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
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items:    []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{
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
					Items:    []*model.Item{},
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusDeleted,
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
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-2",
							Title:  "I2",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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
				md.errUpdateList = errors.New("redundant update list called")

				return md
			},
			wantSqliteLists: []model.List{
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
			wantMarkdownLists: []model.List{
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
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
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
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
		},
		{
			name: "safely anchors new items around cross-list moves",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Position: 0,
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime.Add(1),
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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
					},
				})

				return md
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
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusNotStarted,
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
			wantMarkdownLists: []model.List{
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
							Status:   model.StatusNotStarted,
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
		},
		{
			name: "skips deletion of list due to concurrent edit",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Updated",
						Status:   model.StatusDeleted,
						Modified: baseTime.Add(1),
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Original",
						Modified: baseTime.Add(2 * time.Hour),
					},
				})

				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusDeleted,
					Items:  []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Original",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
		},
		{
			name: "deletes list in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime,
					},
					{
						Name:     "L1",
						Status:   model.StatusDeleted,
						Modified: baseTime.Add(1),
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-2",
					Name:   "L2",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-2",
					Name:   "L2",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
		},
		{
			name: "updates list name and content",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Original",
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

				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Original",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Original",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
		},
		{
			name: "updates list position only",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Position: 1,
					},
					{
						Name:     "L2",
						Modified: baseTime,
						Position: 0,
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime,
					},
				})

				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 1,
					Items:    []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 1,
					Items:    []*model.Item{},
				},
			},
		},
		{
			name: "skips deleted items during list update",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "Deleted Item",
								Status:   model.StatusDeleted,
								Modified: baseTime,
								Position: 0,
							},
							{
								Title:    "Valid Synced Item Updated",
								Status:   model.StatusNotStarted,
								Modified: baseTime.Add(1),
								Position: 1,
							},
							{
								Title:    "Active Item",
								Status:   model.StatusNotStarted,
								Modified: baseTime,
								Position: 2,
							},
						},
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Original",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-3",
								Title:    "Active Item",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
							{
								ID:       "store-item-2",
								Title:    "Valid Synced Item Original",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							ListID:   "store-list-1",
							Title:    "Deleted Item",
							Position: 0,
							Status:   model.StatusDeleted,
						},
						{
							ID:       "store-item-2",
							ListID:   "store-list-1",
							Title:    "Valid Synced Item Updated",
							Position: 1,
							Status:   model.StatusNotStarted,
						},
						{
							ID:       "store-item-3",
							ListID:   "store-list-1",
							Title:    "Active Item",
							Position: 2,
							Status:   model.StatusNotStarted,
						},
					},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							ListID:   "store-list-1",
							Title:    "Valid Synced Item Updated",
							Position: 0,
							Status:   model.StatusNotStarted,
						},
						{
							ID:       "store-item-3",
							ListID:   "store-list-1",
							Title:    "Active Item",
							Position: 1,
							Status:   model.StatusNotStarted,
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime.Add(2 * time.Hour),
							},
						},
					},
				})

				return md
			},
			wantSqliteLists: []model.List{
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
			wantMarkdownLists: []model.List{
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
		},
		{
			name: "updates item content",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-2",
								Title:    "I2 Unsynced",
								Modified: baseTime,
							},
							{
								ID:       "store-item-1",
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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

				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2 Unsynced",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-1",
							Title:    "I1 Updated",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2 Unsynced",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-1",
							Title:    "I1 Updated",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
			},
		},
		{
			name: "skips deletion of list with empty key",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return md
			},
			wantSqliteLists: []model.List{},
			wantMarkdownLists: []model.List{
				{
					ID:     "",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
		},
		{
			name: "skips deletion of item with empty key",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-2",
								Title:    "I2 To Be Deleted",
								Status:   model.StatusDeleted,
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime,
							},
							{
								ID:       "store-item-2",
								Title:    "I2 To Be Deleted",
								Modified: baseTime,
							},
						},
					},
				})

				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
		},
		{
			name: "deletes item in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "I1",
								Status:   model.StatusDeleted,
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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

				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
			},
			wantErr: true,
		},
		{
			name: "creates destination file if missing",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				md.errListLists = fs.ErrNotExist

				return md
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
		},
		{
			name: "fails to build destination state",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				md.errListLists = errors.New("boom")

				return md
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				md.errCreateList = errors.New("boom")

				return md
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})
				md.errCreateItem = errors.New("boom")

				return md
			},
			wantErr: true,
		},
		{
			name: "fails to update list in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Original",
						Modified: baseTime,
					},
				})
				md.errUpdateList = errors.New("boom")

				return md
			},
			wantErr: true,
		},
		{
			name: "fails to update item in destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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
				md.errUpdateItem = errors.New("boom")

				return md
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete list from destination",
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})
				md.errDeleteList = errors.New("boom")

				return md
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete list from source",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Status:   model.StatusDeleted,
						Modified: baseTime.Add(1),
					},
				})
				sqlite.errDeleteList = errors.New("boom")

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return md
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete item from destination",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Status:   model.StatusDeleted,
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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
				md.errDeleteItem = errors.New("boom")

				return md
			},
			wantErr: true,
		},
		{
			name: "fails to permanently delete item from source",
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Status:   model.StatusDeleted,
								Modified: baseTime.Add(1),
							},
						},
					},
				})
				sqlite.errDeleteItem = errors.New("boom")

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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

				return md
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sqlite := tt.setupSqlite(t)
			md := tt.setupMarkdown(t)

			syncer := NewSyncer(sqlite, md)

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

			gotMarkdownLists, err := md.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list markdown lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantMarkdownLists, gotMarkdownLists, opts...); diff != "" {
				t.Errorf("Markdown state mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPullMarkdown(t *testing.T) {
	t.Parallel()
	baseTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		setupMarkdown     func(t *testing.T) *errorProvider
		setupSqlite       func(t *testing.T) *errorProvider
		wantMarkdownLists []model.List
		wantSqliteLists   []model.List
		wantUpdated       bool
		wantErr           bool
	}{
		{
			name: "success (no updates needed)",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
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

				return md
			},
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
			wantMarkdownLists: []model.List{
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
			wantSqliteLists: []model.List{
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
			wantUpdated: false,
		},
		{
			name: "creates new list in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
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
			name: "backfills ID for new list created in markdown",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
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
			name: "creates new item in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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

				return md
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
			wantMarkdownLists: []model.List{
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
			wantSqliteLists: []model.List{
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
			name: "backfills ID for new item created in markdown",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 (Unsynced)",
								Modified: baseTime.Add(1),
							},
							{
								ID:       "store-item-2",
								Title:    "I2 (Synced)",
								Modified: baseTime,
							},
						},
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-2",
								Title:    "I2 (Synced)",
								Modified: baseTime,
							},
						},
					},
				})

				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1 (Unsynced)",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-2",
							Title:    "I2 (Synced)",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1 (Unsynced)",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-2",
							Title:    "I2 (Synced)",
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
			name: "creates new list with multiple items in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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
							{
								ID:       "store-item-2",
								Title:    "I2",
								Modified: baseTime,
							},
						},
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
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
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
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
			name: "safely anchors new items around cross-list moves",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Position: 0,
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime.Add(1),
						Position: 1,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1",
								Modified: baseTime.Add(1),
							},
							{
								ID:       "store-item-2",
								Title:    "I2",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return md
			},
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
					{
						Name:     "L2",
						Modified: baseTime,
					},
				})

				return sqlite
			},
			wantMarkdownLists: []model.List{
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
							Status:   model.StatusNotStarted,
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
			wantSqliteLists: []model.List{
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
							Status:   model.StatusNotStarted,
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
			name: "updates list name and content",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
						Position: 0,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1 Original",
								Modified: baseTime,
							},
						},
					},
					{
						ID:       "store-list-2",
						Name:     "L2 Unchanged",
						Modified: baseTime,
						Position: 1,
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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
						ID:       "store-list-2",
						Name:     "L2 Unchanged",
						Modified: baseTime,
					},
				})

				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1 Updated",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Original",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     "L2 Unchanged",
					Status:   model.StatusOpen,
					Position: 1,
					Items:    []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1 Updated",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Original",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     "L2 Unchanged",
					Status:   model.StatusOpen,
					Position: 1,
					Items:    []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "updates list position only",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Position: 0,
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime,
						Position: 1,
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Position: 1,
					},
					{
						Name:     "L2",
						Modified: baseTime,
						Position: 0,
					},
				})

				return sqlite
			},
			wantMarkdownLists: []model.List{
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
					Items:    []*model.Item{},
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
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items:    []*model.Item{},
				},
			},
			wantUpdated: true,
		},
		{
			name: "updates item content",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "skips already deleted list during deletion phase",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
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
			wantMarkdownLists: []model.List{},
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
			name: "deletes list in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
				})

				return sqlite
			},
			wantMarkdownLists: []model.List{},
			wantSqliteLists: []model.List{
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
			name: "skips already deleted item during deletion phase",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Status:   model.StatusOpen,
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Status:   model.StatusDeleted,
								Modified: baseTime,
							},
							{
								Title:    "I2 To Be Deleted",
								Status:   model.StatusNotStarted,
								Modified: baseTime,
							},
						},
					},
				})

				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusDeleted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-2",
							Title:    "I2 To Be Deleted",
							Position: 1,
							Status:   model.StatusDeleted,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "deletes item in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return md
			},
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
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantSqliteLists: []model.List{
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
			wantUpdated: true,
		},
		{
			name: "fails to build source state",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				md.errListLists = errors.New("boom")

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to build destination state",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errCreateList = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to backfill list ID in source",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
					},
				})
				md.errUpdateList = errors.New("boom")

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to create item in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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

				return md
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
			name: "fails to backfill item ID in source",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1",
								Modified: baseTime.Add(1),
							},
						},
					},
				})
				md.errUpdateItem = errors.New("boom")

				return md
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
			wantErr: true,
		},
		{
			name: "fails to update list in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
					},
				})
				sqlite.errUpdateList = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to update item in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "I1 Updated",
								Modified: baseTime.Add(1),
							},
						},
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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
				sqlite.errUpdateItem = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
		{
			name: "fails to mark list as deleted in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
			},
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
			wantErr: true,
		},
		{
			name: "fails to mark item as deleted in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items:    []*model.Item{},
					},
				})

				return md
			},
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
				sqlite.errUpdateItem = errors.New("boom")

				return sqlite
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			md := tt.setupMarkdown(t)
			sqlite := tt.setupSqlite(t)

			syncer := NewSyncer(sqlite, md)

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

			gotMarkdownLists, err := md.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list markdown lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantMarkdownLists, gotMarkdownLists, opts...); diff != "" {
				t.Errorf("Markdown state mismatch (-want +got):\n%s", diff)
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

func setupTestMarkdown(t *testing.T, lists []model.List) *errorProvider {
	dir := t.TempDir()
	path := filepath.Join(dir, "gtd.md")
	logger := slog.New(slog.DiscardHandler)
	client := markdown.NewClient(path, logger)

	if len(lists) == 0 {
		_ = os.WriteFile(path, []byte(""), 0o600)
	}

	var lastModTime time.Time
	for _, list := range lists {
		if list.Modified.After(lastModTime) {
			lastModTime = list.Modified
		}

		if err := client.CreateList(t.Context(), &list); err != nil {
			t.Fatalf("failed to create list: %v", err)
		}

		for _, item := range slices.Backward(list.Items) {
			if item.Modified.After(lastModTime) {
				lastModTime = item.Modified
			}

			item.ListID = list.ID
			if err := client.CreateItem(t.Context(), item, ""); err != nil {
				t.Fatalf("failed to create item: %v", err)
			}
		}
	}

	if !lastModTime.IsZero() {
		if err := os.Chtimes(path, lastModTime, lastModTime); err != nil {
			t.Fatalf("failed to override markdown file time: %v", err)
		}
	}

	testMarkdown := &errorProvider{
		Provider: client,
	}

	return testMarkdown
}
