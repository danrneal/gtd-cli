package markdown

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/danrneal/gtd.nvim/internal/model"
)

func TestClient_GetKey(t *testing.T) {
	t.Parallel()
	client := &Client{}

	tests := []struct {
		name     string
		resource model.Resource
		wantKey  string
	}{
		{
			name: "list with ID",
			resource: &model.List{
				ID: "list-123",
			},
			wantKey: "list-123",
		},
		{
			name:     "item with empty ID",
			resource: &model.Item{},
			wantKey:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := client.GetKey(tt.resource)
			if got != tt.wantKey {
				t.Errorf("GetKey() = %v, want %v", got, tt.wantKey)
			}
		})
	}
}

func TestClient_CreateList(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		list    *model.List
		want    []model.List
		wantErr bool
	}{
		{
			name: "list created successfully",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "create_success.md")
				content := "# Inbox (0)\n\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			list: &model.List{
				Name:     "Next Actions",
				Status:   model.StatusOpen,
				Modified: modified,
			},
			want: []model.List{
				{
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
				{
					Name:     "Next Actions",
					Position: 1,
					Status:   model.StatusOpen,
					Modified: modified,
					Items:    []*model.Item{},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid list",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "invalid.md")
				return path
			},
			list: &model.List{
				Name:     "",
				Status:   model.StatusOpen,
				Modified: modified,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "cannot create list with status deleted",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "deleted.md")
				return path
			},
			list: &model.List{
				Name:     "Old Project",
				Status:   model.StatusDeleted,
				Modified: modified,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "readFile error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unreadable.md")
				if err := os.WriteFile(path, []byte("# Inbox"), 0o200); err != nil {
					t.Fatalf("failed to create unreadable file: %v", err)
				}

				return path
			},
			list: &model.List{
				Name:     "Next Actions",
				Status:   model.StatusOpen,
				Modified: modified,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "writeFile error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "dir.md")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}

				return path
			},
			list: &model.List{
				Name:     "Next Actions",
				Status:   model.StatusOpen,
				Modified: modified,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			err := client.CreateList(context.Background(), tt.list)

			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateList() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			got, err := client.readFile()
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(model.List{}, "Modified"),
			}

			if diff := cmp.Diff(tt.want, got, opts...); diff != "" {
				t.Errorf("CreateList() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_ListLists(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		want    []model.List
		wantErr bool
	}{
		{
			name: "success",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "list_success.md")
				content := "# Inbox\n* [ ] Task 1\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			want: []model.List{
				{
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Task 1",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "api error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unreadable.md")
				if err := os.WriteFile(path, []byte("# Inbox"), 0o200); err != nil {
					t.Fatalf("failed to create unreadable file: %v", err)
				}

				return path
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			got, err := client.ListLists(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("ListLists() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ListLists() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_UpdateList(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		list    *model.List
		want    []model.List
		wantErr bool
	}{
		{
			name: "success (backfill fallback)",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "update_success_backfill.md")
				content := "# Inbox\n* [ ] Task 1 {{item-1}}\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			list: &model.List{
				ID:       "list-1",
				Name:     "Inbox",
				Position: 0,
				Status:   model.StatusOpen,
				Modified: modified,
				Items: []*model.Item{
					{
						ID:       "item-1",
						ListID:   "list-1",
						Title:    "Task 1",
						Position: 0,
						Status:   model.StatusNotStarted,
						Modified: modified,
					},
				},
			},
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							ID:       "item-1",
							ListID:   "list-1",
							Title:    "Task 1",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "success (rename only)",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "update_success_rename.md")
				content := "# Inbox {{list-1}}\n* [ ] Task 1 {{item-1}}\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			list: &model.List{
				ID:       "list-1",
				Name:     "Updated Inbox",
				Position: 0,
				Status:   model.StatusOpen,
				Modified: modified,
				Items: []*model.Item{
					{
						ID:       "item-1",
						ListID:   "list-1",
						Title:    "Task 1",
						Position: 0,
						Status:   model.StatusNotStarted,
						Modified: modified,
					},
				},
			},
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Updated Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							ID:       "item-1",
							ListID:   "list-1",
							Title:    "Task 1",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "success (reorder and relocate items)",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "update_success_reorder.md")
				content := "# Inbox {{list-1}}\n* [ ] Task 1 {{item-1}}\n* [ ] Task 2 {{item-2}}\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			list: &model.List{
				ID:       "list-1",
				Name:     "Inbox",
				Position: 0,
				Status:   model.StatusOpen,
				Modified: modified,
				Items: []*model.Item{
					{
						ID:       "item-2", // Reordered to top
						ListID:   "list-1",
						Title:    "Task 2",
						Position: 0,
						Status:   model.StatusNotStarted,
						Modified: modified,
					},
					{
						ID:       "item-3", // Relocated from somewhere else
						ListID:   "list-1",
						Title:    "Task 3",
						Position: 1,
						Status:   model.StatusNotStarted,
						Modified: modified,
					},
					{
						ID:       "item-1", // Reordered to bottom
						ListID:   "list-1",
						Title:    "Task 1",
						Position: 2,
						Status:   model.StatusNotStarted,
						Modified: modified,
					},
				},
			},
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							ID:       "item-2",
							ListID:   "list-1",
							Title:    "Task 2",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
						{
							ID:       "item-3",
							ListID:   "list-1",
							Title:    "Task 3",
							Position: 1,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
						{
							ID:       "item-1",
							ListID:   "list-1",
							Title:    "Task 1",
							Position: 2,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid list",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "invalid.md")
				return path
			},
			list: &model.List{
				Name:     "",
				Status:   model.StatusOpen,
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "missing list ID",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "missing_id.md")
				return path
			},
			list: &model.List{
				Name:     "Valid Name",
				ID:       "", // missing
				Status:   model.StatusOpen,
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "read file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unreadable.md")
				if err := os.WriteFile(path, []byte("# Inbox"), 0o200); err != nil {
					t.Fatalf("failed to create unreadable file: %v", err)
				}

				return path
			},
			list: &model.List{
				ID:       "list-1",
				Name:     "Valid Name",
				Status:   model.StatusOpen,
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "list not found",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "not_found.md")
				if err := os.WriteFile(path, []byte("# Inbox {{list-1}}"), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				return path
			},
			list: &model.List{
				ID:       "nonexistent-list",
				Name:     "Valid Name",
				Status:   model.StatusOpen,
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "write file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "dir.md")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}

				return path
			},
			list: &model.List{
				ID:       "list-1",
				Name:     "Valid Name",
				Status:   model.StatusOpen,
				Modified: modified,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			err := client.UpdateList(context.Background(), tt.list, nil)

			if (err != nil) != tt.wantErr {
				t.Fatalf("UpdateList() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			got, err := client.readFile()
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified"),
			}

			if diff := cmp.Diff(tt.want, got, opts...); diff != "" {
				t.Errorf("UpdateList() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_DeleteList(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		list    *model.List
		want    []model.List
		wantErr bool
	}{
		{
			name: "success",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "delete_success.md")
				content := "# Inbox {{list-1}}\n* [ ] Task 1\n\n# Old Project {{list-2}}\n* [ ] Task 2\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			list: &model.List{
				ID: "list-2",
			},
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Task 1",
							ListID:   "list-1",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty list id",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "empty_id.md")
			},
			list: &model.List{
				ID:       "",
				Modified: modified,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "read file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unreadable.md")
				if err := os.WriteFile(path, []byte("# Inbox {{list-1}}"), 0o200); err != nil {
					t.Fatalf("failed to create unreadable file: %v", err)
				}

				return path
			},
			list: &model.List{
				ID:       "list-1",
				Modified: modified,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "list not found",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "not_found.md")
				if err := os.WriteFile(path, []byte("# Inbox {{list-1}}"), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				return path
			},
			list: &model.List{
				ID:       "nonexistent-list",
				Modified: modified,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "write file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "dir.md")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}

				return path
			},
			list: &model.List{
				ID:       "list-1",
				Modified: modified,
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			err := client.DeleteList(context.Background(), tt.list)

			if (err != nil) != tt.wantErr {
				t.Fatalf("DeleteList() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			got, err := client.readFile()
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(model.List{}, "Modified"),
			}

			if diff := cmp.Diff(tt.want, got, opts...); diff != "" {
				t.Errorf("DeleteList() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_CreateItem(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		setup          func(t *testing.T) string
		item           *model.Item
		previousItemID string
		want           []model.List
		wantErr        bool
	}{
		{
			name: "success (insert at top - no previous item)",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "create_success_top.md")
				content := "# Inbox {{list-1}}\n* [ ] Task 2 {{item-2}}\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			item: &model.Item{
				ListID:   "list-1",
				Title:    "Task 1",
				Status:   model.StatusNotStarted,
				Modified: modified,
			},
			previousItemID: "",
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Task 1",
							ListID:   "list-1",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
						{
							ID:       "item-2",
							Title:    "Task 2",
							ListID:   "list-1",
							Position: 1,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "success (insert after previous item)",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "create_success_after.md")
				content := "# Inbox {{list-1}}\n* [ ] Task 1 {{item-1}}\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			item: &model.Item{
				ListID:   "list-1",
				Title:    "Task 2",
				Status:   model.StatusNotStarted,
				Modified: modified,
			},
			previousItemID: "item-1",
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							ID:       "item-1",
							Title:    "Task 1",
							ListID:   "list-1",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
						{
							Title:    "Task 2",
							ListID:   "list-1",
							Position: 1,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid item",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "invalid.md")
				return path
			},
			item: &model.Item{
				Title:    "", // invalid
				ListID:   "list-1",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "cannot create item with status deleted",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "deleted.md")
				return path
			},
			item: &model.Item{
				Title:    "Bad Item",
				Status:   model.StatusDeleted,
				ListID:   "list-1",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "missing list ID",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "missing_list.md")
				return path
			},
			item: &model.Item{
				Title:          "Orphan Item",
				ExternalListID: stringPtr("ext-list-1"),
				Modified:       modified,
			},
			wantErr: true,
		},
		{
			name: "read file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unreadable.md")
				if err := os.WriteFile(path, []byte("# Inbox"), 0o200); err != nil {
					t.Fatalf("failed to create unreadable file: %v", err)
				}

				return path
			},
			item: &model.Item{
				Title:    "New Item",
				ListID:   "list-1",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "list not found",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "no_list.md")
				if err := os.WriteFile(path, []byte("# Inbox {{list-1}}"), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				return path
			},
			item: &model.Item{
				Title:    "New Item",
				ListID:   "nonexistent-list",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "prevItem not found",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "no_prev_item.md")
				if err := os.WriteFile(path, []byte("# Inbox {{list-1}}"), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				return path
			},
			item: &model.Item{
				Title:    "New Item",
				ListID:   "list-1",
				Modified: modified,
			},
			previousItemID: "ghost-item",
			wantErr:        true,
		},
		{
			name: "write file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "dir.md")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}

				return path
			},
			item: &model.Item{
				Title:    "New Item",
				ListID:   "list-1",
				Modified: modified,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			err := client.CreateItem(context.Background(), tt.item, tt.previousItemID)

			if (err != nil) != tt.wantErr {
				t.Fatalf("CreateItem() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			got, err := client.readFile()
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified"),
			}

			if diff := cmp.Diff(tt.want, got, opts...); diff != "" {
				t.Errorf("CreateItem() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_UpdateItem(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		item    *model.Item
		want    []model.List
		wantErr bool
	}{
		{
			name: "success (backfill fallback)",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "update_success_backfill.md")
				content := "# Inbox {{list-1}}\n* [ ] Task 1\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			item: &model.Item{
				ID:       "item-1",
				ListID:   "list-1",
				Title:    "Task 1",
				Position: 0,
				Status:   model.StatusNotStarted,
				Modified: modified,
			},
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							ID:       "item-1",
							ListID:   "list-1",
							Title:    "Task 1",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "success",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "update_success.md")
				content := "# Inbox {{list-1}}\n* [ ] Task 1 {{item-1}}\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			item: &model.Item{
				ID:       "item-1",
				ListID:   "list-1",
				Title:    "Task 1 Updated",
				Status:   model.StatusDone,
				Modified: modified,
			},
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							ID:       "item-1",
							ListID:   "list-1",
							Title:    "Task 1 Updated",
							Position: 0,
							Status:   model.StatusDone,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid item",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "invalid.md")
				return path
			},
			item: &model.Item{
				Title:    "", // invalid
				ListID:   "list-1",
				ID:       "item-1",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "missing item ID",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "missing_item_id.md")
				return path
			},
			item: &model.Item{
				Title:    "Valid Title",
				ListID:   "list-1",
				ID:       "", // missing
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "missing list ID",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "missing_list_id.md")
				return path
			},
			item: &model.Item{
				Title:    "Valid Title",
				ListID:   "", // missing
				ID:       "item-1",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "read file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unreadable.md")
				if err := os.WriteFile(path, []byte("# Inbox"), 0o200); err != nil {
					t.Fatalf("failed to create unreadable file: %v", err)
				}

				return path
			},
			item: &model.Item{
				Title:    "Valid Title",
				ListID:   "list-1",
				ID:       "item-1",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "list not found",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "no_list.md")
				if err := os.WriteFile(path, []byte("# Inbox {{list-1}}"), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				return path
			},
			item: &model.Item{
				Title:    "Valid Title",
				ListID:   "nonexistent-list",
				ID:       "item-1",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "item not found",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "no_item.md")
				if err := os.WriteFile(path, []byte("# Inbox {{list-1}}\n* [ ] Task 1 {{item-1}}"), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				return path
			},
			item: &model.Item{
				Title:    "Valid Title",
				ListID:   "list-1",
				ID:       "nonexistent-item",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "write file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "dir.md")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}

				return path
			},
			item: &model.Item{
				Title:    "Valid Title",
				ListID:   "list-1",
				ID:       "item-1",
				Modified: modified,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			err := client.UpdateItem(context.Background(), tt.item)

			if (err != nil) != tt.wantErr {
				t.Fatalf("UpdateItem() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			got, err := client.readFile()
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified"),
			}

			if diff := cmp.Diff(tt.want, got, opts...); diff != "" {
				t.Errorf("UpdateItem() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_DeleteItem(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		item    *model.Item
		want    []model.List
		wantErr bool
	}{
		{
			name: "success",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "delete_success.md")
				content := "# Inbox {{list-1}}\n* [ ] Task 1 {{item-1}}\n* [ ] Task 2 {{item-2}}\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			item: &model.Item{
				ID:     "item-1",
				ListID: "list-1",
			},
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							ID:       "item-2",
							ListID:   "list-1",
							Title:    "Task 2",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "item id missing",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "missing_item_id.md")
				return path
			},
			item: &model.Item{
				ID:       "", // missing
				ListID:   "list-1",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "list id missing",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "missing_list_id.md")
				return path
			},
			item: &model.Item{
				ID:       "item-1",
				ListID:   "", // missing
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "read file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unreadable.md")
				if err := os.WriteFile(path, []byte("# Inbox"), 0o200); err != nil {
					t.Fatalf("failed to create unreadable file: %v", err)
				}

				return path
			},
			item: &model.Item{
				ID:       "item-1",
				ListID:   "list-1",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "list not found",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "no_list.md")
				if err := os.WriteFile(path, []byte("# Inbox {{list-1}}"), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				return path
			},
			item: &model.Item{
				ID:       "item-1",
				ListID:   "nonexistent-list",
				Modified: modified,
			},
			wantErr: true,
		},
		{
			name: "item not found",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "no_item.md")
				if err := os.WriteFile(path, []byte("# Inbox {{list-1}}\n* [ ] Task 1 {{item-1}}"), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				return path
			},
			item: &model.Item{
				ID:       "nonexistent-item",
				ListID:   "list-1",
				Modified: modified,
			},
			want: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Items: []*model.Item{
						{
							ID:       "item-1",
							ListID:   "list-1",
							Title:    "Task 1",
							Position: 0,
							Status:   model.StatusNotStarted,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "write file error",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "dir.md")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}

				return path
			},
			item: &model.Item{
				ID:       "item-1",
				ListID:   "list-1",
				Modified: modified,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			err := client.DeleteItem(context.Background(), tt.item)

			if (err != nil) != tt.wantErr {
				t.Fatalf("DeleteItem() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			got, err := client.readFile()
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified"),
			}

			if diff := cmp.Diff(tt.want, got, opts...); diff != "" {
				t.Errorf("DeleteItem() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_readFile(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantLists   []model.List
		wantContent string
		wantErr     bool
	}{
		{
			name: "valid file",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "valid.md")
				content := "# Inbox (1)\n* [ ] Task 1\n\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			wantLists: []model.List{
				{
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Task 1",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantContent: "# Inbox (1)\n* [ ] Task 1\n\n",
		},
		{
			name: "self heals bad formatting",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "bad_format.md")
				content := "# Inbox (99) {{list-1}}\n* [ ] Task 1\n"
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("failed to create valid file: %v", err)
				}

				if err := os.Chtimes(path, modified, modified); err != nil {
					t.Fatalf("failed to change file times: %v", err)
				}

				return path
			},
			wantLists: []model.List{
				{
					ID:       "list-1",
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Task 1",
							ListID:   "list-1",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			wantContent: "# Inbox (1) {{list-1}}\n* [ ] Task 1\n\n",
		},
		{
			name: "file not found",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "does_not_exist.md")
				return path
			},
			wantLists: nil,
		},
		{
			name: "failed to open markdown file",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unreadable.md")
				if err := os.WriteFile(path, []byte("# Inbox"), 0o200); err != nil {
					t.Fatalf("failed to create unreadable file: %v", err)
				}

				return path
			},
			wantLists: nil,
			wantErr:   true,
		},
		{
			name: "failed to self-heal markdown file",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unwritable.md")
				content := "# Inbox (99) {{list-1}}\n* [ ] Task 1\n"
				if err := os.WriteFile(path, []byte(content), 0o400); err != nil {
					t.Fatalf("failed to create unreadable file: %v", err)
				}

				return path
			},
			wantLists: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			got, err := client.readFile()

			if (err != nil) != tt.wantErr {
				t.Fatalf("readFile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.wantLists, got); diff != "" {
				t.Errorf("readFile() mismatch (-want +got):\n%s", diff)
			}

			if tt.wantErr || tt.wantLists == nil {
				return
			}

			b, err := os.ReadFile(testPath)
			if err != nil {
				t.Fatalf("failed to read test file for content assertion: %v", err)
			}

			if gotContent := string(b); gotContent != tt.wantContent {
				t.Errorf("readFile() self-heal content mismatch:\nwant: %q\ngot:  %q", tt.wantContent, gotContent)
			}
		})
	}
}

func TestClient_writeFile(t *testing.T) {
	t.Parallel()

	modified := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		lists   []model.List
		want    string
		wantErr bool
	}{
		{
			name: "successfully write file",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "write_success.md")
				return path
			},
			lists: []model.List{
				{
					Name:     "Inbox",
					Position: 0,
					Status:   model.StatusOpen,
					Modified: modified,
					Items: []*model.Item{
						{
							Title:    "Task 1",
							ListID:   "",
							Position: 0,
							Status:   model.StatusNotStarted,
							Modified: modified,
						},
					},
				},
			},
			want: `# Inbox (1)
* [ ] Task 1

`,
		},
		{
			name: "failed to render markdown file",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "render_fail.md")
				if err := os.WriteFile(path, []byte("initial content"), 0o600); err != nil {
					t.Fatalf("failed to create initial file: %v", err)
				}

				return path
			},
			lists: []model.List{
				{
					Name: "Invalid Status List",
					Items: []*model.Item{
						{
							Title:  "Bad Task",
							Status: "unknown_status",
							ID:     "item-1",
						},
					},
				},
			},
			want:    "initial content",
			wantErr: true,
		},
		{
			name: "failed to open markdown file for writing",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "unwritable.md")
				if err := os.WriteFile(path, []byte("# Inbox"), 0o400); err != nil {
					t.Fatalf("failed to create unwritable file: %v", err)
				}

				return path
			},
			lists:   nil,
			want:    "# Inbox",
			wantErr: true,
		},
		{
			name: "failed to write to markdown file",
			setup: func(t *testing.T) string {
				if _, err := os.Stat("/dev/full"); os.IsNotExist(err) {
					t.Skip("skipping test; /dev/full not available on this OS")
				}

				return "/dev/full"
			},
			lists: []model.List{
				{
					Name:   "Inbox",
					Status: model.StatusOpen,
					Items: []*model.Item{
						{
							Title:  "Task 1",
							Status: model.StatusNotStarted,
						},
					},
				},
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testPath := tt.setup(t)
			logger := slog.New(slog.DiscardHandler)
			client := NewClient(testPath, logger)

			err := client.writeFile(tt.lists)

			if (err != nil) != tt.wantErr {
				t.Fatalf("writeFile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && client.lastModTime.IsZero() {
				t.Fatal("expected lastModTime to be updated after successful mutation")
			}

			if testPath == "/dev/full" {
				return
			}

			b, err := os.ReadFile(testPath)
			if err != nil {
				t.Fatalf("failed to read test file: %v", err)
			}

			got := string(b)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("writeFile() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClient_Concurrency(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "concurrency.md")
	if err := os.WriteFile(path, []byte("# Inbox\n"), 0o600); err != nil {
		t.Fatalf("failed to setup file: %v", err)
	}

	logger := slog.New(slog.DiscardHandler)
	client := NewClient(path, logger)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	events, err := client.Watch(ctx)
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	go func() {
		for range events {
			// Drain the channel to prevent the watcher from blocking
		}
	}()

	for i := range 50 {
		list := &model.List{
			Name:     fmt.Sprintf("List %d", i),
			Status:   model.StatusOpen,
			Modified: time.Now(),
		}

		if err := client.CreateList(ctx, list); err != nil {
			t.Fatalf("mutation failed during concurrency test: %v", err)
		}
	}
}

func stringPtr(s string) *string {
	return &s
}
