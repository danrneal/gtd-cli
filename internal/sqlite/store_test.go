package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/sqlite"
)

func stringPtr(s string) *string {
	return &s
}

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

				wantTables := []string{"lists", "items"}
				for _, tbl := range wantTables {
					var name string
					err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name)
					if err != nil {
						t.Errorf("table %q not found in database: %v", tbl, err)
					}
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

			store, err := sqlite.NewStore(ctx, dbPath)

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
		wantErr  bool
	}{
		{
			name: "valid list (auto-trimmed)",
			list: model.List{
				Name:     " Inbox ",
				Position: 0,
				Modified: time.Now(),
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

			store, err := sqlite.NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			err = store.CreateList(ctx, tt.list)

			if tt.wantErr {
				if err == nil {
					t.Error("CreateList() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("CreateList() unexpected error: %v", err)
				}

				db, err := sql.Open("sqlite3", dbPath)
				if err != nil {
					t.Fatalf("failed to open db for verification: %v", err)
				}

				defer db.Close()

				var count int
				wantName := strings.TrimSpace(tt.list.Name)
				err = db.QueryRow("SELECT COUNT(*) FROM lists WHERE name = ?", wantName).Scan(&count)
				if err != nil {
					t.Fatalf("failed to query list: %v", err)
				}

				if count != 1 {
					t.Errorf("expected 1 list with name %q, got %d", wantName, count)
				}
			}
		})
	}
}

func TestGetAllLists(t *testing.T) {
	tests := []struct {
		name       string
		setupLists func(t *testing.T, db *sql.DB)
		setupCtx   func() (context.Context, context.CancelFunc)
		wantCount  int
		verifyList func(t *testing.T, lists []model.List)
		wantErr    bool
	}{
		{
			name:       "empty db",
			setupLists: nil,
			wantCount:  0,
			wantErr:    false,
		},
		{
			name: "valid list (empty items)",
			setupLists: func(t *testing.T, db *sql.DB) {
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
			wantCount: 1,
			verifyList: func(t *testing.T, lists []model.List) {
				if len(lists) != 1 {
					t.Fatalf("expected 1 list, got %d", len(lists))
				}

				if lists[0].Items == nil || len(lists[0].Items) != 0 {
					t.Errorf("expected empty non-nil items slice, got: %v", lists[0].Items)
				}
			},
			wantErr: false,
		},
		{
			name: "valid list (with items)",
			setupLists: func(t *testing.T, db *sql.DB) {
				rows, err := db.Exec(
					`
						INSERT INTO lists (name, modified) 
						VALUES (?, ?)
					`, "Work", time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert list: %v", err)
				}

				listID, _ := rows.LastInsertId()
				_, err = db.Exec(
					`
						INSERT INTO items (
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, '[]', ?, ?)
					`, "Task 1", "", listID, time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}
			},
			wantCount: 1,
			verifyList: func(t *testing.T, lists []model.List) {
				if len(lists) != 1 {
					t.Fatalf("expected 1 list, got %d", len(lists))
				}

				if len(lists[0].Items) != 1 {
					t.Errorf("expected 1 item in list, got %d", len(lists[0].Items))
				}

				if lists[0].Items[0].Title != "Task 1" {
					t.Errorf("expected item title 'Task 1', got %q", lists[0].Items[0].Title)
				}
			},
			wantErr: false,
		},
		{
			name: "context cancellation",
			setupLists: func(t *testing.T, db *sql.DB) {
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
			wantCount: 0,
			wantErr:   true,
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

			store, err := sqlite.NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("failed to open db for setup: %v", err)
			}

			defer db.Close()

			if tt.setupLists != nil {
				tt.setupLists(t, db)
			}

			lists, err := store.GetAllLists(ctx)

			if tt.wantErr {
				if err == nil {
					t.Error("GetAllLists() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("GetAllLists() unexpected error: %v", err)
				}

				if len(lists) != tt.wantCount {
					t.Errorf("GetAllLists() count mismatch: want %d, got %d", tt.wantCount, len(lists))
				}

				if tt.verifyList != nil {
					tt.verifyList(t, lists)
				}
			}
		})
	}
}

func TestCreateItem(t *testing.T) {
	tests := []struct {
		name     string
		item     model.Item
		setupCtx func() (context.Context, context.CancelFunc)
		wantDesc string
		wantErr  bool
	}{
		{
			name: "valid item minimal fields (auto-trimmed title)",
			item: model.Item{
				ListID:   1,
				Title:    "  Buy Milk  ",
				Modified: time.Now(),
				Created:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "valid item complex fields (multiline desc)",
			item: model.Item{
				ListID:      1,
				Title:       "Complex Task",
				Description: "  Line 1   \nLine 2   \n  Line 3",
				Completed:   true,
				ProjectID:   stringPtr("proj-1"),
				WaitingOn:   stringPtr("Alice"),
				Tags:        []string{"work", "urgent"},
				Modified:    time.Now(),
				Created:     time.Now(),
			},
			wantDesc: "Line 1\nLine 2\n  Line 3",
			wantErr:  false,
		},
		{
			name: "empty title",
			item: model.Item{
				ListID: 1,
				Title:  "",
			},
			wantErr: true,
		},
		{
			name: "cancelled context",
			item: model.Item{
				ListID: 1,
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

			store, err := sqlite.NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			list := model.List{Name: "Inbox", Modified: time.Now()}
			if err := store.CreateList(context.Background(), list); err != nil {
				t.Fatalf("failed to create prerequisite list: %v", err)
			}

			err = store.CreateItem(ctx, tt.item)

			if tt.wantErr {
				if err == nil {
					t.Error("CreateItem() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("CreateItem() unexpected error: %v", err)
				}

				db, err := sql.Open("sqlite3", dbPath)
				if err != nil {
					t.Fatalf("failed to open db for verification: %v", err)
				}

				defer db.Close()

				var count int
				wantTitle := strings.TrimSpace(tt.item.Title)
				err = db.QueryRow("SELECT COUNT(*) FROM items WHERE title = ?", wantTitle).Scan(&count)
				if err != nil {
					t.Fatalf("failed to query item: %v", err)
				}

				if count != 1 {
					t.Errorf("expected 1 item with title %q, got %d", wantTitle, count)
				}

				if tt.wantDesc != "" {
					var desc string
					err = db.QueryRow("SELECT description FROM items WHERE title = ?", wantTitle).Scan(&desc)
					if err != nil {
						t.Fatalf("failed to query description: %v", err)
					}

					if desc != tt.wantDesc {
						t.Errorf("Description mismatch.\nWant:\n%q\nGot:\n%q", tt.wantDesc, desc)
					}
				}
			}
		})
	}
}

func TestGetAllItems(t *testing.T) {
	tests := []struct {
		name       string
		setupItems func(t *testing.T, db *sql.DB)
		setupCtx   func() (context.Context, context.CancelFunc)
		wantCount  int
		wantErr    bool
	}{
		{
			name:       "empty db",
			setupItems: nil,
			wantCount:  0,
			wantErr:    false,
		},
		{
			name: "valid items (no tags)",
			setupItems: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(
					`
						INSERT INTO items (
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, '[]', ?, ?)
					`, "Item 1", "", 1, time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "valid items (with tags)",
			setupItems: func(t *testing.T, db *sql.DB) {
				tags := `["work", "urgent"]`
				_, err := db.Exec(
					`
						INSERT INTO items (
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, ?, ?, ?)
					`, "Item 2", "desc", 1, tags, time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "corrupt data (bad tags json)",
			setupItems: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(
					`
						INSERT INTO items (
							title, 
							description, 
							list_id, tags, 
							modified, 
							created
						) VALUES (?, ?, ?, '{bad-json', ?, ?)
					`, "Item 3", "", 1, time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}
			},
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "context cancellation",
			setupItems: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(
					`
						INSERT INTO items (
							title, 
							description, 
							list_id, 
							tags, 
							modified, 
							created
						) VALUES (?, ?, ?, '[]', ?, ?)
					`, "Item 4", "", 1, time.Now(), time.Now(),
				)

				if err != nil {
					t.Fatalf("failed to insert item: %v", err)
				}
			},
			setupCtx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantCount: 0,
			wantErr:   true,
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

			store, err := sqlite.NewStore(context.Background(), dbPath)
			if err != nil {
				t.Fatalf("failed to create store: %v", err)
			}

			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("failed to open db for setup: %v", err)
			}

			defer db.Close()

			_, err = db.Exec(
				`
					INSERT INTO lists (name, modified) 
					VALUES (?, ?)
				`, "Inbox", time.Now(),
			)

			if err != nil {
				t.Fatalf("failed to create list: %v", err)
			}

			if tt.setupItems != nil {
				tt.setupItems(t, db)
			}

			items, err := store.GetAllItems(ctx)

			if tt.wantErr {
				if err == nil {
					t.Error("GetAllItems() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("GetAllItems() unexpected error: %v", err)
				}

				if len(items) != tt.wantCount {
					t.Errorf("GetAllItems() count mismatch: want %d, got %d", tt.wantCount, len(items))
				}
			}
		})
	}
}
