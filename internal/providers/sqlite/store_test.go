package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestNewStore(t *testing.T) {
	tests := []struct {
		name         string
		setupDBPath  func(t *testing.T) string
		wantErr      bool
		verifyTables func(t *testing.T, dbPath string)
	}{
		{
			name: "valid database path creates tables",
			setupDBPath: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "valid.db")
			},
			wantErr: false,
			verifyTables: func(t *testing.T, dbPath string) {
				db, err := sql.Open("sqlite3", dbPath)
				if err != nil {
					t.Fatalf("failed to open db for verification: %v", err)
				}

				defer db.Close()

				rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
				if err != nil {
					t.Fatalf("failed to query tables: %v", err)
				}

				defer rows.Close()

				var gotTables []string
				for rows.Next() {
					var name string
					if err := rows.Scan(&name); err != nil {
						t.Fatalf("failed to scan table name: %v", err)
					}

					if name != "sqlite_sequence" {
						gotTables = append(gotTables, name)
					}
				}

				wantTables := []string{"items", "lists"}
				if diff := cmp.Diff(wantTables, gotTables); diff != "" {
					t.Errorf("database tables mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "invalid database path (non-existent directory)",
			setupDBPath: func(t *testing.T) string {
				return "/non/existent/path/db.sqlite"
			},
			wantErr:      true,
			verifyTables: nil,
		},
		{
			name: "create tables fails on read-only db",
			setupDBPath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				dbPath := filepath.Join(tmpDir, "readonly.db")
				file, err := os.Create(dbPath)
				if err != nil {
					t.Fatalf("failed to create temp file: %v", err)
				}

				file.Close()

				if err := os.Chmod(dbPath, 0400); err != nil {
					t.Fatalf("failed to chmod: %v", err)
				}

				return dbPath
			},
			wantErr:      true,
			verifyTables: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := tt.setupDBPath(t)
			ctx := context.Background()

			store, err := NewStore(ctx, dbPath)

			if tt.wantErr {
				if err == nil {
					t.Error("NewStore() expected error, got nil")
				}

				if store != nil {
					t.Error("NewStore() expected nil store on error, got instance")
				}
			} else {
				if err != nil {
					t.Errorf("NewStore() unexpected error: %v", err)
				}

				if store == nil {
					t.Error("NewStore() expected store instance, got nil")
				}

				if tt.verifyTables != nil {
					tt.verifyTables(t, dbPath)
				}
			}
		})
	}
}

func TestCreateList(t *testing.T) {
	tests := []struct {
		name     string
		list     model.List
		setupCtx func() (context.Context, context.CancelFunc)
		wantList *model.List
		wantErr  bool
	}{
		{
			name: "valid list (auto-trimmed)",
			list: model.List{
				Name:     " Inbox ",
				Position: 0,
				Modified: time.Now(),
			},
			wantList: &model.List{
				Name:     "Inbox",
				Position: 0,
			},
			wantErr: false,
		},
		{
			name: "valid list with external id",
			list: model.List{
				Name:       "Work",
				Position:   1,
				Modified:   time.Now(),
				ExternalID: stringPtr("ext-123"),
			},
			wantList: &model.List{
				Name:       "Work",
				Position:   1,
				ExternalID: stringPtr("ext-123"),
			},
			wantErr: false,
		},
		{
			name: "cancelled context",
			list: model.List{
				Name:     "Cancelled",
				Modified: time.Now(),
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantErr: true,
		},
		{
			name: "empty list name",
			list: model.List{
				Name:     "",
				Modified: time.Now(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "test.db")

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.setupCtx != nil {
				ctx, cancel = tt.setupCtx()
				defer cancel()
			} else {
				ctx = context.Background()
			}

			store, err := NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			gotID, err := store.CreateList(ctx, tt.list)

			if tt.wantErr {
				if err == nil {
					t.Error("CreateList() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("CreateList() unexpected error: %v", err)
				}

				if gotID == "" {
					t.Error("CreateList() returned empty ID")
				}

				db, err := sql.Open("sqlite3", dbPath)
				if err != nil {
					t.Fatalf("failed to open db for verification: %v", err)
				}

				defer db.Close()

				if tt.wantList != nil {
					var gotList model.List
					wantName := strings.TrimSpace(tt.list.Name)
					err = db.QueryRow(
						"SELECT id, name, position, external_id FROM lists WHERE name = ?", wantName,
					).Scan(&gotList.ID, &gotList.Name, &gotList.Position, &gotList.ExternalID)

					if err != nil {
						t.Fatalf("failed to query list: %v", err)
					}

					opts := []cmp.Option{
						cmpopts.IgnoreFields(model.List{}, "ID", "Modified", "Items"),
					}

					if diff := cmp.Diff(tt.wantList, &gotList, opts...); diff != "" {
						t.Errorf("CreateList() mismatch (-want +got):\n%s", diff)
					}
				}
			}
		})
	}
}

func TestListLists(t *testing.T) {
	tests := []struct {
		name      string
		setupDB   func(t *testing.T, db *sql.DB)
		setupCtx  func() (context.Context, context.CancelFunc)
		wantLists []model.List
		wantErr   bool
	}{
		{
			name:    "empty db",
			setupDB: nil,
			wantErr: false,
		},
		{
			name: "valid list (empty items)",
			setupDB: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Inbox", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}
			},
			wantLists: []model.List{
				{
					ID:    "list-1",
					Name:  "Inbox",
					Items: []model.Item{},
				},
			},
			wantErr: false,
		},
		{
			name: "valid list (with items)",
			setupDB: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-2", "Work", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, '[]', ?, ?)
					`, "item-1", "Task 1", "", "list-2", time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}
			},
			wantLists: []model.List{
				{
					ID:   "list-2",
					Name: "Work",
					Items: []model.Item{
						{
							ID:     "item-1",
							ListID: "list-2",
							Title:  "Task 1",
							Status: model.StatusNotStarted,
							Tags:   []string{},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid list (with complex items)",
			setupDB: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-3", "Complex", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							description,
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, ?, ?, ?)
					`, "item-2", "Task 2", "", "list-3", `["a", "b"]`, time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}
			},
			wantLists: []model.List{
				{
					ID:   "list-3",
					Name: "Complex",
					Items: []model.Item{
						{
							ID:     "item-2",
							ListID: "list-3",
							Title:  "Task 2",
							Status: model.StatusNotStarted,
							Tags:   []string{"a", "b"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "corrupt item data (bad tags)",
			setupDB: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-4", "Broken", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							description,
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, ?, ?, ?)
					`, "item-3", "Task 3", "", "list-4", `{badjson`, time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}
			},
			wantErr: true,
		},
		{
			name: "context cancellation",
			setupDB: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(
					`
						INSERT INTO lists (name, modified) 
						VALUES (?, ?)
					`, "Inbox", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "test.db")

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.setupCtx != nil {
				ctx, cancel = tt.setupCtx()
				defer cancel()
			} else {
				ctx = context.Background()
			}

			store, err := NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("failed to open db for setup: %v", err)
			}

			defer db.Close()

			if tt.setupDB != nil {
				tt.setupDB(t, db)
			}

			lists, err := store.ListLists(ctx)

			if tt.wantErr {
				if err == nil {
					t.Error("ListLists() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ListLists() unexpected error: %v", err)
				}

				opts := []cmp.Option{
					cmpopts.IgnoreFields(model.List{}, "Modified", "ID"),
					cmpopts.IgnoreFields(model.Item{}, "Modified", "Created", "Snoozed", "Due", "ID", "ListID"),
				}

				if diff := cmp.Diff(tt.wantLists, lists, opts...); diff != "" {
					t.Errorf("ListLists() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestUpdateList(t *testing.T) {
	tests := []struct {
		name         string
		setupDB      func(t *testing.T, db *sql.DB) string
		setupList    func(id string) model.List
		setupCurrent func() []model.Item
		setupCtx     func() (context.Context, context.CancelFunc)
		wantList     *model.List
		wantItems    []model.Item
		wantErr      bool
	}{
		{
			name: "valid update (rename and reorder)",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Old Name", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				if _, err := db.Exec(
					`
						INSERT INTO items (
							id, 
							title, 
							description,
							list_id, 
							position, 
							status, 
							modified, 
							created
						) 
						VALUES (?, ?, ?, ?, ?, ?, ?, ?)
					`, "item-1", "A", "", "list-1", 0, "not_started", time.Now(), time.Now()); err != nil {
					t.Fatalf("failed to insert item 1: %v", err)
				}

				if _, err := db.Exec(
					`
						INSERT INTO items (
							id, 
							title, 
							description,
							list_id, 
							position, 
							status, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
					`, "item-2", "B", "", "list-1", 1, "not_started", time.Now(), time.Now()); err != nil {
					t.Fatalf("failed to insert item 2: %v", err)
				}

				return "list-1"
			},
			setupList: func(id string) model.List {
				list := model.List{
					ID:       id,
					Name:     "  New Name  ",
					Position: 5,
					Modified: time.Now(),
					Items: []model.Item{
						{
							ID:       "item-2",
							ListID:   id,
							Position: 0,
							Title:    "B",
							Status:   model.StatusNotStarted,
						},
						{
							ID:       "item-1",
							ListID:   id,
							Position: 1,
							Title:    "A",
							Status:   model.StatusNotStarted,
						},
					},
				}

				return list
			},
			wantList: &model.List{
				ID:       "list-1",
				Name:     "New Name",
				Position: 5,
			},
			wantItems: []model.Item{
				{
					ID:       "item-2",
					ListID:   "list-1",
					Position: 0,
					Title:    "B",
					Status:   model.StatusNotStarted,
					Tags:     []string{},
				},
				{
					ID:       "item-1",
					ListID:   "list-1",
					Position: 1,
					Title:    "A",
					Status:   model.StatusNotStarted,
					Tags:     []string{},
				},
			},
			wantErr: false,
		},
		{
			name: "valid update (optimization skip)",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-opt", "Optimization", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id, 
							title, 
							description,
							list_id, 
							position, 
							status, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, ?, ?, '[]', ?, ?)
					`, "item-opt", "Task Opt", "", "list-opt", 99, "not_started", time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}

				return "list-opt"
			},
			setupList: func(id string) model.List {
				list := model.List{
					ID:   id,
					Name: "Optimization",
					Items: []model.Item{
						{
							ID:       "item-opt",
							ListID:   id,
							Position: 0,
							Title:    "Task Opt",
						},
					},
				}

				return list
			},
			setupCurrent: func() []model.Item {
				items := []model.Item{
					{
						ID:       "item-opt",
						ListID:   "list-opt",
						Position: 0,
					},
				}

				return items
			},
			wantList: &model.List{
				ID:   "list-opt",
				Name: "Optimization",
			},
			wantItems: []model.Item{
				{
					ID:       "item-opt",
					ListID:   "list-opt",
					Position: 99,
					Title:    "Task Opt",
					Status:   model.StatusNotStarted,
					Tags:     []string{},
				},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Valid", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				return "list-1"
			},
			setupList: func(id string) model.List {
				list := model.List{
					ID:       id,
					Name:     "",
					Modified: time.Now(),
				}

				return list
			},
			wantList: nil,
			wantErr:  true,
		},
		{
			name: "nonexistent id",
			setupDB: func(t *testing.T, db *sql.DB) string {
				return "non-existent"
			},
			setupList: func(id string) model.List {
				list := model.List{
					ID:       id,
					Name:     "Valid Name",
					Modified: time.Now(),
				}

				return list
			},
			wantList: nil,
			wantErr:  true,
		},
		{
			name: "nonexistent item id",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, _ = db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "List", time.Now())

				return "list-1"
			},
			setupList: func(id string) model.List {
				list := model.List{
					ID:   id,
					Name: "List",
					Items: []model.Item{
						{ID: "missing-item", ListID: id, Position: 0},
					},
				}

				return list
			},
			wantList: nil,
			wantErr:  true,
		},
		{
			name: "context cancellation",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Valid", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				return "list-1"
			},
			setupList: func(id string) model.List {
				list := model.List{
					ID:       id,
					Name:     "Valid Name",
					Modified: time.Now(),
				}

				return list
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantList: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "test.db")

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.setupCtx != nil {
				ctx, cancel = tt.setupCtx()
				defer cancel()
			} else {
				ctx = context.Background()
			}

			store, err := NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("failed to open db for setup: %v", err)
			}

			defer db.Close()

			id := tt.setupDB(t, db)
			list := tt.setupList(id)

			var currentItems []model.Item
			if tt.setupCurrent != nil {
				currentItems = tt.setupCurrent()
			}

			err = store.UpdateList(ctx, list, currentItems)

			if tt.wantErr {
				if err == nil {
					t.Error("UpdateList() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("UpdateList() unexpected error: %v", err)
				}

				var gotList model.List
				err = db.QueryRow(
					`
						SELECT name, position 
						FROM lists 
						WHERE id = ?
					`, id,
				).Scan(&gotList.Name, &gotList.Position)

				if err != nil {
					t.Fatalf("failed to query list: %v", err)
				}

				gotList.ID = id

				opts := []cmp.Option{
					cmpopts.IgnoreFields(model.List{}, "Modified", "ID", "ExternalID", "Items"),
				}

				if diff := cmp.Diff(*tt.wantList, gotList, opts...); diff != "" {
					t.Errorf("UpdateList() mismatch (-want +got):\n%s", diff)
				}

				if tt.wantItems != nil {
					items, err := store.listAllItems(context.Background())
					if err != nil {
						t.Fatalf("failed to list all items: %v", err)
					}

					itemOpts := []cmp.Option{
						cmpopts.IgnoreFields(model.Item{}, "Modified", "Created", "Snoozed", "Due"),
					}

					if diff := cmp.Diff(tt.wantItems, items, itemOpts...); diff != "" {
						t.Errorf("UpdateList() items mismatch (-want +got):\n%s", diff)
					}
				}
			}
		})
	}
}

func TestDeleteList(t *testing.T) {
	tests := []struct {
		name      string
		setupDB   func(t *testing.T, db *sql.DB) string
		setupCtx  func() (context.Context, context.CancelFunc)
		wantLists []model.List
		wantItems []model.Item
		wantErr   bool
	}{
		{
			name: "valid delete with cascade",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "To Delete", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, '[]', ?, ?)
					`, "item-1", "Linked Item", "list-1", time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}

				return "list-1"
			},
			wantErr: false,
		},
		{
			name: "nonexistent id",
			setupDB: func(t *testing.T, db *sql.DB) string {
				return "non-existent"
			},
			wantErr: true,
		},
		{
			name: "context cancellation",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "To Delete", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				return "list-1"
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "test.db")

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.setupCtx != nil {
				ctx, cancel = tt.setupCtx()
				defer cancel()
			} else {
				ctx = context.Background()
			}

			store, err := NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("failed to open db setup: %v", err)
			}

			defer db.Close()

			id := tt.setupDB(t, db)

			err = store.DeleteList(ctx, id)

			if tt.wantErr {
				if err == nil {
					t.Error("DeleteList() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("DeleteList() unexpected error: %v", err)
				}

				lists, err := store.ListLists(context.Background())
				if err != nil {
					t.Fatalf("failed to get all lists: %v", err)
				}

				if diff := cmp.Diff(tt.wantLists, lists); diff != "" {
					t.Errorf("DeleteList() lists mismatch (-want +got):\n%s", diff)
				}

				items, err := store.listAllItems(context.Background())
				if err != nil {
					t.Fatalf("failed to get all items: %v", err)
				}

				if diff := cmp.Diff(tt.wantItems, items); diff != "" {
					t.Errorf("DeleteList() items mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestCreateItem(t *testing.T) {
	tests := []struct {
		name     string
		setupDB  func(t *testing.T, db *sql.DB)
		item     model.Item
		setupCtx func() (context.Context, context.CancelFunc)
		wantItem *model.Item
		wantErr  bool
	}{
		{
			name: "valid item minimal fields (auto-trimmed title)",
			setupDB: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(`INSERT INTO lists (id, name, modified) VALUES (?, ?, ?)`, "list-1", "Inbox", time.Now())
				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}
			},
			item: model.Item{
				ListID:   "list-1",
				Title:    "  Buy Milk  ",
				Status:   model.StatusNotStarted,
				Modified: time.Now(),
				Created:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "valid item complex fields (multiline desc)",
			setupDB: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(`INSERT INTO lists (id, name, modified) VALUES (?, ?, ?)`, "list-1", "Inbox", time.Now())
				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}
			},
			item: model.Item{
				ListID:      "list-1",
				Title:       "Complex Task",
				Description: "  Line 1   \nLine 2   \n  Line 3",
				Status:      model.StatusDone,
				ProjectID:   stringPtr("proj-1"),
				WaitingOn:   stringPtr("Alice"),
				Tags:        []string{"work", "urgent"},
				Modified:    time.Now(),
				Created:     time.Now(),
			},
			wantItem: &model.Item{
				ListID:      "list-1",
				Title:       "Complex Task",
				Description: "Line 1\nLine 2\n  Line 3",
				Status:      model.StatusDone,
				ProjectID:   stringPtr("proj-1"),
				WaitingOn:   stringPtr("Alice"),
				Tags:        []string{"work", "urgent"},
			},
			wantErr: false,
		},
		{
			name:    "invalid status",
			setupDB: nil,
			item: model.Item{
				ListID: "list-1",
				Title:  "Invalid Status",
				Status: "bad_status",
			},
			wantErr: true,
		},
		{
			name:    "empty title",
			setupDB: nil,
			item: model.Item{
				ListID: "list-1",
				Title:  "",
			},
			wantErr: true,
		},
		{
			name:    "cancelled context",
			setupDB: nil,
			item: model.Item{
				ListID: "list-1",
				Title:  "Cancelled",
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "test.db")

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.setupCtx != nil {
				ctx, cancel = tt.setupCtx()
				defer cancel()
			} else {
				ctx = context.Background()
			}

			store, err := NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("failed to open db for setup: %v", err)
			}

			defer db.Close()

			if tt.setupDB != nil {
				tt.setupDB(t, db)
			}

			gotID, err := store.CreateItem(ctx, tt.item, "")

			if tt.wantErr {
				if err == nil {
					t.Error("CreateItem() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("CreateItem() unexpected error: %v", err)
				}

				if gotID == "" {
					t.Error("CreateItem() returned empty ID")
				}

				var count int
				wantTitle := strings.TrimSpace(tt.item.Title)
				err = db.QueryRow("SELECT COUNT(*) FROM items WHERE title = ?", wantTitle).Scan(&count)
				if err != nil {
					t.Fatalf("failed to query item: %v", err)
				}

				if count != 1 {
					t.Errorf("expected 1 item with title %q, got %d", wantTitle, count)
				}

				if tt.wantItem != nil {
					var gotItem model.Item
					var tagsJSON string
					err = db.QueryRow(
						`SELECT list_id, title, COALESCE(description, ''), status, tags, project_id, waiting_on 
						FROM items WHERE title = ?`, wantTitle,
					).Scan(&gotItem.ListID, &gotItem.Title, &gotItem.Description, &gotItem.Status, &tagsJSON, &gotItem.ProjectID, &gotItem.WaitingOn)
					if err != nil {
						t.Fatalf("failed to query item: %v", err)
					}

					if err := json.Unmarshal([]byte(tagsJSON), &gotItem.Tags); err != nil {
						t.Fatalf("failed to unmarshal tags: %v", err)
					}

					if gotItem.Tags == nil {
						gotItem.Tags = []string{}
					}

					opts := []cmp.Option{
						cmpopts.IgnoreFields(model.Item{}, "ID", "Modified", "Created", "Snoozed", "Due", "ExternalID"),
					}

					if diff := cmp.Diff(tt.wantItem, &gotItem, opts...); diff != "" {
						t.Errorf("CreateItem() mismatch (-want +got):\n%s", diff)
					}
				}
			}
		})
	}
}

func TestUpdateItem(t *testing.T) {
	tests := []struct {
		name      string
		setupDB   func(t *testing.T, db *sql.DB) string
		setupItem func(id string) model.Item
		setupCtx  func() (context.Context, context.CancelFunc)
		wantItem  *model.Item
		wantErr   bool
	}{
		{
			name: "valid update (complex fields)",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Inbox", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, '[]', ?, ?)
					`, "item-1", "Original", "", "list-1", time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}

				return "item-1"
			},
			setupItem: func(id string) model.Item {
				item := model.Item{
					ID:          id,
					ListID:      "list-2", // Should be ignored
					Position:    99,       // Should be ignored
					Title:       "  Updated Title  ",
					Description: "  Line 1  \n  Line 2",
					Status:      model.StatusDone,
					Tags:        []string{"updated", "tag"},
					Modified:    time.Now(),
					Created:     time.Now(),
				}

				return item
			},
			wantItem: &model.Item{
				ListID:      "list-1", // Verify original
				Position:    0,        // Verify original
				Title:       "Updated Title",
				Description: "Line 1\n  Line 2",
				Status:      model.StatusDone,
				Tags:        []string{"updated", "tag"},
			},
			wantErr: false,
		},
		{
			name: "invalid status",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Inbox", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, '[]', ?, ?)
					`, "item-1", "Valid", "", "list-1", time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}

				return "item-1"
			},
			setupItem: func(id string) model.Item {
				item := model.Item{
					ID:       id,
					ListID:   "list-1",
					Title:    "Valid",
					Status:   "bad_status",
					Modified: time.Now(),
				}

				return item
			},
			wantErr: true,
		},
		{
			name: "empty title",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Inbox", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, '[]', ?, ?)
					`, "item-1", "Valid", "", "list-1", time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}

				return "item-1"
			},
			setupItem: func(id string) model.Item {
				item := model.Item{
					ID:       id,
					ListID:   "list-1",
					Title:    "",
					Modified: time.Now(),
				}

				return item
			},
			wantErr: true,
		},
		{
			name: "nonexistent id",
			setupDB: func(t *testing.T, db *sql.DB) string {
				return "non-existent"
			},
			setupItem: func(id string) model.Item {
				item := model.Item{
					ID:       id,
					ListID:   "list-1",
					Title:    "Valid",
					Modified: time.Now(),
				}

				return item
			},
			wantErr: true,
		},
		{
			name: "context cancellation",
			setupDB: func(t *testing.T, db *sql.DB) string {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Inbox", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, '[]', ?, ?)
					`, "item-1", "Valid", "", "list-1", time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}

				return "item-1"
			},
			setupItem: func(id string) model.Item {
				item := model.Item{
					ID:       id,
					ListID:   "list-1",
					Title:    "Valid",
					Modified: time.Now(),
				}

				return item
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "test.db")

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.setupCtx != nil {
				ctx, cancel = tt.setupCtx()
				defer cancel()
			} else {
				ctx = context.Background()
			}

			store, err := NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("failed to open db for setup: %v", err)
			}

			defer db.Close()

			id := tt.setupDB(t, db)
			item := tt.setupItem(id)

			err = store.UpdateItem(ctx, item)

			if tt.wantErr {
				if err == nil {
					t.Error("UpdateItem() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("UpdateItem() unexpected error: %v", err)
				}

				var gotItem model.Item
				var tagsJSON string
				err = db.QueryRow(
					`
						SELECT id, list_id, title, COALESCE(description, ''), status, tags, position
						FROM items 
						WHERE id = ?
					`, id,
				).Scan(&gotItem.ID, &gotItem.ListID, &gotItem.Title, &gotItem.Description, &gotItem.Status, &tagsJSON, &gotItem.Position)

				if err != nil {
					t.Fatalf("failed to query item: %v", err)
				}

				if err := json.Unmarshal([]byte(tagsJSON), &gotItem.Tags); err != nil {
					t.Fatalf("failed to unmarshal tags: %v", err)
				}

				if gotItem.Tags == nil {
					gotItem.Tags = []string{}
				}

				tt.wantItem.ID = id

				opts := []cmp.Option{
					cmpopts.IgnoreFields(model.Item{}, "Modified", "Created", "Snoozed", "Due", "ProjectID", "WaitingOn", "ExternalID"),
				}

				if diff := cmp.Diff(tt.wantItem, &gotItem, opts...); diff != "" {
					t.Errorf("UpdateItem() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestDeleteItem(t *testing.T) {
	tests := []struct {
		name     string
		setupDB  func(t *testing.T, db *sql.DB) model.Item
		setupCtx func() (context.Context, context.CancelFunc)
		wantErr  bool
	}{
		{
			name: "valid delete",
			setupDB: func(t *testing.T, db *sql.DB) model.Item {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Inbox", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, '[]', ?, ?)
					`, "item-1", "Item to Delete", "", "list-1", time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}

				item := model.Item{ID: "item-1"}

				return item
			},
			wantErr: false,
		},
		{
			name: "nonexistent id",
			setupDB: func(t *testing.T, db *sql.DB) model.Item {
				item := model.Item{ID: "non-existent"}
				return item
			},
			wantErr: true,
		},
		{
			name: "context cancellation",
			setupDB: func(t *testing.T, db *sql.DB) model.Item {
				_, err := db.Exec(
					`
						INSERT INTO lists (id, name, modified) 
						VALUES (?, ?, ?)
					`, "list-1", "Inbox", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				_, err = db.Exec(
					`
						INSERT INTO items (
							id,
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, '[]', ?, ?)
					`, "item-1", "Valid", "", "list-1", time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}

				item := model.Item{ID: "item-1"}

				return item
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "test.db")

			var ctx context.Context
			var cancel context.CancelFunc

			if tt.setupCtx != nil {
				ctx, cancel = tt.setupCtx()
				defer cancel()
			} else {
				ctx = context.Background()
			}

			store, err := NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("failed to open db for setup: %v", err)
			}

			defer db.Close()

			item := tt.setupDB(t, db)

			err = store.DeleteItem(ctx, item)

			if tt.wantErr {
				if err == nil {
					t.Error("DeleteItem() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("DeleteItem() unexpected error: %v", err)
				}

				items, err := store.listAllItems(context.Background())
				if err != nil {
					t.Fatalf("failed to get all items: %v", err)
				}

				if diff := cmp.Diff([]model.Item(nil), items); diff != "" {
					t.Errorf("DeleteItem() items mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
