package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/providers/markdown"
)

func TestPushMarkdown(t *testing.T) {
	t.Parallel()
	baseTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		setupSqlite       func(t *testing.T) Provider
		setupMarkdown     func(t *testing.T) RemoteProvider
		wantSqliteLists   []model.List
		wantMarkdownLists []model.List
	}{
		{
			name: "success (no updates needed)",
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) RemoteProvider {
				md := setupTestMarkdown(t, []model.List{})

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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
					Items:  []*model.Item{},
				},
			},
		},
		{
			name: "creates new item in destination",
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
						{
							ID:     "store-item-2",
							Title:  "I2",
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
			name: "safely anchors new items around cross-list moves",
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
					{
						Name:     "L2",
						Modified: baseTime.Add(1),
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
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
							Status: model.StatusNotStarted,
							ListID: "store-list-2",
						},
						{
							ID:     "store-item-2",
							Title:  "I2",
							Status: model.StatusNotStarted,
							ListID: "store-list-2",
						},
					},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
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
							Status: model.StatusNotStarted,
							ListID: "store-list-2",
						},
						{
							ID:     "store-item-2",
							Title:  "I2",
							Status: model.StatusNotStarted,
							ListID: "store-list-2",
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
						Name:     "L1 Updated",
						Status:   model.StatusDeleted,
						Modified: baseTime.Add(1),
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			name: "updates list name and content",
			setupSqlite: func(t *testing.T) Provider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupSqlite: func(t *testing.T) Provider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupSqlite: func(t *testing.T) Provider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "Active Item",
								Status:   model.StatusNotStarted,
								Modified: baseTime,
								Position: 0,
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
				md := setupTestMarkdown(t, []model.List{
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
							ListID: "store-list-1",
							Title:  "Active Item",
							Status: model.StatusNotStarted,
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
			wantMarkdownLists: []model.List{
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
		},
		{
			name: "skips deletion of item due to concurrent edit",
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupSqlite: func(t *testing.T) Provider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
							ID:     "store-item-1",
							Title:  "I1 Updated",
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
							Title:  "I1 Updated",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sqlite := tt.setupSqlite(t)
			md := tt.setupMarkdown(t)

			syncer := NewSyncer(sqlite, md)

			syncStart := baseTime.Add(time.Hour)
			err := syncer.Push(context.Background(), syncStart)
			if err != nil {
				t.Fatalf("Pull failed: %v", err)
			}

			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified", "Created"),
			}

			gotSqliteLists, _ := sqlite.ListLists(context.Background())
			if diff := cmp.Diff(tt.wantSqliteLists, gotSqliteLists, opts...); diff != "" {
				t.Errorf("Sqlite state mismatch (-want +got):\n%s", diff)
			}

			gotMarkdownLists, _ := md.ListLists(context.Background())
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
		setupMarkdown     func(t *testing.T) RemoteProvider
		setupSqlite       func(t *testing.T) Provider
		wantMarkdownLists []model.List
		wantSqliteLists   []model.List
		wantUpdated       bool
	}{
		{
			name: "success (no updates needed)",
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) Provider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
					},
				})

				return md
			},
			setupSqlite: func(t *testing.T) Provider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupSqlite: func(t *testing.T) Provider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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

				return md
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
			name: "creates new list with multiple items in destination",
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupSqlite: func(t *testing.T) Provider {
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
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
						{
							ID:     "store-item-2",
							Title:  "I2",
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
						{
							ID:     "store-item-2",
							Title:  "I2",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "safely anchors new items around cross-list moves",
			setupMarkdown: func(t *testing.T) RemoteProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
					{
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime.Add(1),
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
					{
						Name:     "L2",
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
					Items:  []*model.Item{},
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
							Status: model.StatusNotStarted,
							ListID: "store-list-2",
						},
						{
							ID:     "store-item-2",
							Title:  "I2",
							Status: model.StatusNotStarted,
							ListID: "store-list-2",
						},
					},
				},
			},
			wantSqliteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
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
							Status: model.StatusNotStarted,
							ListID: "store-list-2",
						},
						{
							ID:     "store-item-2",
							Title:  "I2",
							Status: model.StatusNotStarted,
							ListID: "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "updates list name and content",
			setupMarkdown: func(t *testing.T) RemoteProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
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
			setupSqlite: func(t *testing.T) Provider {
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
				})

				return sqlite
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
			wantUpdated: true,
		},
		{
			name: "updates list position only",
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupSqlite: func(t *testing.T) Provider {
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
			setupMarkdown: func(t *testing.T) RemoteProvider {
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
			setupSqlite: func(t *testing.T) Provider {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			md := tt.setupMarkdown(t)
			sqlite := tt.setupSqlite(t)

			syncer := NewSyncer(sqlite, md)

			syncStart := baseTime.Add(time.Hour)
			updated, err := syncer.Pull(context.Background(), syncStart)
			if err != nil {
				t.Fatalf("Pull failed: %v", err)
			}

			if updated != tt.wantUpdated {
				t.Errorf("updated = %v, want %v", updated, tt.wantUpdated)
			}

			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified", "Created"),
			}

			gotMarkdownLists, _ := md.ListLists(context.Background())
			if diff := cmp.Diff(tt.wantMarkdownLists, gotMarkdownLists, opts...); diff != "" {
				t.Errorf("Markdown state mismatch (-want +got):\n%s", diff)
			}

			gotSqliteLists, _ := sqlite.ListLists(context.Background())
			if diff := cmp.Diff(tt.wantSqliteLists, gotSqliteLists, opts...); diff != "" {
				t.Errorf("Sqlite state mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func setupTestMarkdown(t *testing.T, lists []model.List) RemoteProvider {
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

		if err := client.CreateList(context.Background(), &list); err != nil {
			t.Fatalf("failed to create list: %v", err)
		}

		for _, item := range slices.Backward(list.Items) {
			if item.Modified.After(lastModTime) {
				lastModTime = item.Modified
			}

			item.ListID = list.ID
			if err := client.CreateItem(context.Background(), item, ""); err != nil {
				t.Fatalf("failed to create item: %v", err)
			}
		}
	}

	if !lastModTime.IsZero() {
		if err := os.Chtimes(path, lastModTime, lastModTime); err != nil {
			t.Fatalf("failed to override markdown file time: %v", err)
		}
	}

	return client
}
