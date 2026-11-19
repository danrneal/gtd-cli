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

	"github.com/danrneal/gtd-cli/model"
	"github.com/danrneal/gtd-cli/providers/markdown"
)

func TestPushMarkdown(t *testing.T) {
	t.Parallel()
	baseTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := []struct {
		name              string
		setupSQLite       func(t *testing.T) *errorProvider
		setupMarkdown     func(t *testing.T) *errorProvider
		wantSQLiteLists   []model.List
		wantMarkdownLists []model.List
		wantErr           error
	}{
		{
			name: "success (no updates needed)",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Status:   model.StatusDeleted,
						Modified: baseTime.Add(1),
					},
					{
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
				})

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
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
					ID:       "store-list-2",
					Name:     "Inbox",
					Position: 1,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
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
					{
						Name:     "L2",
						Modified: baseTime,
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
			wantSQLiteLists: []model.List{
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
			},
			wantSQLiteLists: []model.List{
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
			wantSQLiteLists: []model.List{
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
			name: "prepends new item in destination",
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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
				})

				md.errUpdateList = errors.New("redundant update list called")

				return md
			},
			wantSQLiteLists: []model.List{
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
			},
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "Inbox",
						Status:   model.StatusOpen,
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
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
					{
						ID:       "store-list-2",
						Name:     "L1",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
				})

				return md
			},
			wantSQLiteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "Inbox",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "Inbox",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
		},
		{
			name: "updates list name and content",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
					{
						Name:     "L2",
						Modified: baseTime,
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
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return md
			},
			wantSQLiteLists: []model.List{
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
			name: "skips deleted items when reordering list items",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								Title:    "Deleted Item",
								Status:   model.StatusDeleted,
								Modified: baseTime,
							},
							{
								Title:    "Item 2",
								Status:   model.StatusNotStarted,
								Modified: baseTime,
							},
							{
								Title:    "Item 1",
								Status:   model.StatusNotStarted,
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
								ID:       "store-item-3",
								Title:    "Item 1",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
							{
								ID:       "store-item-2",
								Title:    "Item 2",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				md.errUpdateItem = errors.New("unexpected update item called")

				return md
			},
			wantSQLiteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
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
							Title:    "Item 2",
							Position: 1,
							Status:   model.StatusNotStarted,
						},
						{
							ID:       "store-item-3",
							ListID:   "store-list-1",
							Title:    "Item 1",
							Position: 2,
							Status:   model.StatusNotStarted,
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
							ListID:   "store-list-1",
							Title:    "Item 2",
							Position: 0,
							Status:   model.StatusNotStarted,
						},
						{
							ID:       "store-item-3",
							ListID:   "store-list-1",
							Title:    "Item 1",
							Position: 1,
							Status:   model.StatusNotStarted,
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
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "Deleted Item",
								Status:   model.StatusDeleted,
								Modified: baseTime,
							},
							{
								Title:    "I1 Updated",
								Status:   model.StatusNotStarted,
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
								ID:       "store-item-2",
								Title:    "I1 Original",
								Modified: baseTime,
								Status:   model.StatusNotStarted,
							},
						},
					},
				})

				md.errUpdateList = errors.New("unexpected update list called")

				return md
			},
			wantSQLiteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
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
							Title:    "I1 Updated",
							Position: 1,
							Status:   model.StatusNotStarted,
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
							ListID: "store-list-1",
							Title:  "I1 Updated",
							Status: model.StatusNotStarted,
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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

				return md
			},
			wantSQLiteLists: []model.List{
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
		{
			name: "skips project backfill for projects list",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-1",
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return md
			},
			wantSQLiteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   model.ListProjects,
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-1",
							ProjectTag: new("project-a"),
						},
					},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   model.ListProjects,
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-1",
							ProjectTag: new("project-a"),
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
								Title:     "Item with Project",
								Modified:  baseTime,
								ProjectID: new("store-item-1"),
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
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-1",
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
					{
						ID:       "store-list-2",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-2",
								Title:      "Item with Project",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				md.errUpdateItem = errors.New("boom")

				return md
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-1",
							ProjectTag: new("project-a"),
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-2",
							Title:      "Item with Project",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-2",
							ProjectID:  new("store-item-1"),
							ProjectTag: new("project-a"),
						},
					},
				},
			},
			wantMarkdownLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-1",
							ProjectTag: new("project-a"),
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-2",
							Title:      "Item with Project",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-2",
							ProjectTag: new("project-a"),
						},
					},
				},
			},
		},
		{
			name: "skips deletion of list with empty key",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{},
			wantMarkdownLists: []model.List{
				{
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
		},
		{
			name: "skips deletion of item with empty key",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 To Be Deleted",
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
								Title:    "I2",
								Modified: baseTime,
							},
							{
								ID:       "store-item-1",
								Title:    "I1 To Be Deleted",
								Modified: baseTime,
							},
						},
					},
				})

				return md
			},
			wantSQLiteLists: []model.List{
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
							Title:  "I2",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
		},
		{
			name: "deletes item in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			name: "prepends item and deletes trailing item in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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
							{
								Title:    "I3",
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
								ID:       "store-item-2",
								Title:    "I1",
								Modified: baseTime,
							},
							{
								ID:       "store-item-3",
								Title:    "I2",
								Modified: baseTime,
							},
							{
								ID:       "store-item-4",
								Title:    "I3",
								Modified: baseTime,
							},
						},
					},
				})

				return md
			},
			wantSQLiteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I0",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-2",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-3",
							Title:    "I2",
							Position: 2,
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
							Title:    "I0",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-2",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-3",
							Title:    "I2",
							Position: 2,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				return md
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "creates destination file if missing",
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				md.errListLists = fs.ErrPermission

				return md
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{})
				md.errCreateList = fs.ErrPermission

				return md
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				md.errCreateItem = fs.ErrPermission

				return md
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to update list in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				md.errUpdateList = fs.ErrPermission

				return md
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to update item in destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				md.errUpdateItem = fs.ErrPermission

				return md
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to permanently delete list from destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				md.errDeleteList = fs.ErrPermission

				return md
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to permanently delete list from source",
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Status:   model.StatusDeleted,
						Modified: baseTime.Add(1),
					},
				})

				sqlite.errDeleteList = fs.ErrPermission

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
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to permanently delete item from destination",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				md.errDeleteItem = fs.ErrPermission

				return md
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to permanently delete item from source",
			setupSQLite: func(t *testing.T) *errorProvider {
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

				sqlite.errDeleteItem = fs.ErrPermission

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
			wantErr: fs.ErrPermission,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sqlite := tt.setupSQLite(t)
			md := tt.setupMarkdown(t)

			syncer := NewSyncer(sqlite, md)

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

			gotMarkdownLists, err := md.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list markdown lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantMarkdownLists, gotMarkdownLists, listOpts...); diff != "" {
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
		setupSQLite       func(t *testing.T) *errorProvider
		wantMarkdownLists []model.List
		wantSQLiteLists   []model.List
		wantUpdated       bool
		wantErr           error
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
			wantSQLiteLists: []model.List{
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			name: "preserves updated status across subsequent unchanged list",
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
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
					Position: 0,
					Status:   model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 1,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 0,
					Status:   model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
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
			name: "backfills ID for new list created in markdown",
			setupMarkdown: func(t *testing.T) *errorProvider {
				path := filepath.Join(t.TempDir(), "gtd.md")
				content := "# L1\n* [ ] I1\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to write markdown file: %v", err)
				}

				modTime := baseTime.Add(1)
				if err := os.Chtimes(path, modTime, modTime); err != nil {
					t.Fatalf("failed to override markdown file time: %v", err)
				}

				logger := slog.New(slog.DiscardHandler)
				client := markdown.NewClient(path, logger)
				md := &errorProvider{
					Provider: client,
				}

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
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
					},
				},
			},
			wantSQLiteLists: []model.List{
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
					},
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
								Title:    "I2 (Unsynced)",
								Modified: baseTime.Add(1),
							},
							{
								ID:       "store-item-1",
								Title:    "I1 (Synced)",
								Modified: baseTime,
							},
						},
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "I1 (Synced)",
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
							ID:       "store-item-2",
							Title:    "I2 (Unsynced)",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-1",
							Title:    "I1 (Synced)",
							Position: 1,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2 (Unsynced)",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-1",
							Title:    "I1 (Synced)",
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
						ID:       "store-list-2",
						Name:     "L2",
						Modified: baseTime,
					},
					{
						ID:       "store-list-1",
						Name:     "L1",
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
					},
					{
						Name:     "L2",
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
			wantSQLiteLists: []model.List{
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
			name: "updates list name and content",
			setupMarkdown: func(t *testing.T) *errorProvider {
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
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

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
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
			wantSQLiteLists: []model.List{
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
			wantUpdated: true,
		},
		{
			name: "preserves tombstoned items in destination during pull",
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:    "Already Deleted Task",
								Status:   model.StatusDeleted,
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
			wantSQLiteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:     "store-item-1",
							Title:  "Already Deleted Task",
							Status: model.StatusDeleted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: false,
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
			name: "skips project backfill entirely when all projects are linked",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-1",
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
					{
						ID:       "store-list-2",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-2",
								Title:      "Trap: Already Has ProjectID",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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
								ProjectID:  new("store-item-1"),
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				sqlite.errUpdateItem = errors.New("boom")

				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ProjectTag: new("project-a"),
							ListID:     "store-list-1",
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-2",
							Title:      "Trap: Already Has ProjectID",
							Status:     model.StatusNotStarted,
							Position:   0,
							ProjectTag: new("project-a"),
							ListID:     "store-list-2",
						},
					},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-1",
							ProjectTag: new("project-a"),
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-2",
							Title:      "Trap: Already Has ProjectID",
							Status:     model.StatusNotStarted,
							Position:   0,
							ListID:     "store-list-2",
							ProjectID:  new("store-item-1"),
							ProjectTag: new("project-a"),
						},
					},
				},
			},
			wantUpdated: false,
		},

		{
			name: "skips project backfill for projects list",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-1",
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
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

				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   model.ListProjects,
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-1",
							ProjectTag: new("project-a"),
						},
					},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   model.ListProjects,
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-1",
							ProjectTag: new("project-a"),
						},
					},
				},
			},
		},
		{
			name: "continues backfill loop after optimized item",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-2",
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
						ID:       "store-list-1",
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Project New",
								Modified:   baseTime,
								ProjectTag: new("project-new"),
							},
							{
								ID:         "store-item-1",
								Title:      "Project Old",
								Modified:   baseTime,
								ProjectTag: new("project-old"),
							},
						},
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Project Old",
								Modified:   baseTime,
								ProjectTag: new("project-old"),
							},
						},
					},
				})

				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:         "store-item-2",
							Title:      "Item 1",
							Status:     model.StatusNotStarted,
							Position:   0,
							ListID:     "store-list-2",
							ProjectTag: new("project-old"),
						},
						{
							ID:         "store-item-3",
							Title:      "Item 2",
							Status:     model.StatusNotStarted,
							Position:   1,
							ListID:     "store-list-2",
							ProjectTag: new("project-new"),
						},
					},
				},
				{
					ID:       "store-list-1",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-4",
							Title:      "Project New",
							Status:     model.StatusNotStarted,
							Position:   0,
							ListID:     "store-list-1",
							ProjectTag: new("project-new"),
						},
						{
							ID:         "store-item-1",
							Title:      "Project Old",
							Status:     model.StatusNotStarted,
							Position:   1,
							ListID:     "store-list-1",
							ProjectTag: new("project-old"),
						},
					},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:         "store-item-2",
							Title:      "Item 1",
							Status:     model.StatusNotStarted,
							Position:   0,
							ListID:     "store-list-2",
							ProjectID:  new("store-item-1"),
							ProjectTag: new("project-old"),
						},
						{
							ID:         "store-item-3",
							Title:      "Item 2",
							Status:     model.StatusNotStarted,
							Position:   1,
							ListID:     "store-list-2",
							ProjectID:  new("store-item-4"),
							ProjectTag: new("project-new"),
						},
					},
				},
				{
					ID:       "store-list-1",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-4",
							Title:      "Project New",
							Status:     model.StatusNotStarted,
							Position:   0,
							ListID:     "store-list-1",
							ProjectTag: new("project-new"),
						},
						{
							ID:         "store-item-1",
							Title:      "Project Old",
							Status:     model.StatusNotStarted,
							Position:   1,
							ListID:     "store-list-1",
							ProjectTag: new("project-old"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "backfills project id when projects list is created last",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-1",
								Title:      "Item with Project",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
					{
						ID:       "store-list-2",
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-2",
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Item with Project",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-1",
							ProjectTag: new("project-a"),
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-2",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-2",
							ProjectTag: new("project-a"),
						},
					},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:         "store-item-1",
							Title:      "Item with Project",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-1",
							ProjectID:  new("store-item-2"),
							ProjectTag: new("project-a"),
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-2",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-2",
							ProjectTag: new("project-a"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "continues backfill loop after item without project",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:       "store-item-1",
								Title:    "Normal Item",
								Modified: baseTime,
							},
							{
								ID:         "store-item-2",
								Title:      "Item with Project",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
					{
						ID:       "store-list-2",
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-3",
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "Normal Item",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-1",
						},
						{
							ID:         "store-item-2",
							Title:      "Item with Project",
							Status:     model.StatusNotStarted,
							Position:   1,
							ListID:     "store-list-1",
							ProjectTag: new("project-a"),
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-3",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-2",
							ProjectTag: new("project-a"),
						},
					},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items: []*model.Item{
						{
							ID:       "store-item-1",
							Title:    "Normal Item",
							Status:   model.StatusNotStarted,
							Position: 0,
							ListID:   "store-list-1",
						},
						{
							ID:         "store-item-2",
							Title:      "Item with Project",
							Status:     model.StatusNotStarted,
							Position:   1,
							ListID:     "store-list-1",
							ProjectID:  new("store-item-3"),
							ProjectTag: new("project-a"),
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     model.ListProjects,
					Status:   model.StatusOpen,
					Position: 1,
					Items: []*model.Item{
						{
							ID:         "store-item-3",
							Title:      "Project A",
							Status:     model.StatusNotStarted,
							ListID:     "store-list-2",
							ProjectTag: new("project-a"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "skips already deleted list during deletion phase",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
					{
						Name:     "L2",
						Status:   model.StatusDeleted,
						Modified: baseTime,
					},
				})

				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "Inbox",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 1,
					Status:   model.StatusDeleted,
					Items:    []*model.Item{},
				},
			},
			wantUpdated: false,
		},
		{
			name: "deletes list in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
					{
						Name:     "L2",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
				})

				return sqlite
			},
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "Inbox",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []*model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 1,
					Status:   model.StatusDeleted,
					Items:    []*model.Item{},
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
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantSQLiteLists: []model.List{
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
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
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
			wantMarkdownLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items:  []*model.Item{},
				},
			},
			wantSQLiteLists: []model.List{
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
			name: "prepends item and deletes trailing item in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime.Add(1),
						Items: []*model.Item{
							{
								ID:       "store-item-4",
								Title:    "I0",
								Modified: baseTime.Add(1),
							},
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
							{
								Title:    "I3",
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
							ID:       "store-item-4",
							Title:    "I0",
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
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 2,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantSQLiteLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "store-item-4",
							Title:    "I0",
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
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 2,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 3,
							Status:   model.StatusDeleted,
							ListID:   "store-list-1",
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
				md.errListLists = fs.ErrPermission

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to build destination state",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
				})

				return md
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				sqlite.errCreateList = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
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

				md.errUpdateList = fs.ErrPermission

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{})
				return sqlite
			},
			wantErr: fs.ErrPermission,
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
			wantErr: fs.ErrPermission,
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

				md.errUpdateItem = fs.ErrPermission

				return md
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
			wantErr: fs.ErrPermission,
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
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1 Original",
						Modified: baseTime,
					},
				})

				sqlite.errUpdateList = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
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
			setupSQLite: func(t *testing.T) *errorProvider {
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

				sqlite.errUpdateItem = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to update item after updating list in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1 Updated",
						Modified: baseTime.Add(1),
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
			setupSQLite: func(t *testing.T) *errorProvider {
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

				sqlite.errUpdateItem = fs.ErrPermission

				return sqlite
			},
			wantUpdated: true,
			wantErr:     fs.ErrPermission,
		},
		{
			name: "fails to backfill project id in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-1",
								Title:      "Item with Project",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
					{
						ID:       "store-list-2",
						Name:     model.ListProjects,
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-2",
								Title:      "Project A",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return md
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								ID:         "store-item-1",
								Title:      "Item with Project",
								Modified:   baseTime,
								ProjectTag: new("project-a"),
							},
						},
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L1",
						Modified: baseTime,
						Items: []*model.Item{
							{
								Title:      "Item with Project",
								Modified:   baseTime.Add(1),
								ProjectTag: new("project-a"),
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
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-1",
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "Inbox",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
					{
						Name:     "L2",
						Status:   model.StatusOpen,
						Modified: baseTime,
					},
				})

				sqlite.errUpdateList = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
		},
		{
			name: "fails to mark list as deleted after creating list in destination",
			setupMarkdown: func(t *testing.T) *errorProvider {
				md := setupTestMarkdown(t, []model.List{
					{
						ID:       "store-list-99",
						Name:     "L1",
						Modified: baseTime,
					},
				})

				return md
			},
			setupSQLite: func(t *testing.T) *errorProvider {
				sqlite := setupTestSQLite(t, []model.List{
					{
						Name:     "L2",
						Modified: baseTime,
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
			setupSQLite: func(t *testing.T) *errorProvider {
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

				sqlite.errUpdateItem = fs.ErrPermission

				return sqlite
			},
			wantErr: fs.ErrPermission,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			md := tt.setupMarkdown(t)
			sqlite := tt.setupSQLite(t)

			syncer := NewSyncer(sqlite, md)

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

			gotMarkdownLists, err := md.ListLists(t.Context())
			if err != nil {
				t.Fatalf("failed to list markdown lists: %v", err)
			}

			if diff := cmp.Diff(tt.wantMarkdownLists, gotMarkdownLists, listOpts...); diff != "" {
				t.Errorf("Markdown state mismatch (-want +got):\n%s", diff)
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

func setupTestMarkdown(t *testing.T, lists []model.List) *errorProvider {
	dir := t.TempDir()
	path := filepath.Join(dir, "gtd.md")
	logger := slog.New(slog.DiscardHandler)
	client := markdown.NewClient(path, logger)

	var lastModTime time.Time
	for i, list := range lists {
		list.Position = i

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
