package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/sqlite"
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
			name: "valid list",
			list: model.List{
				Name:     "Inbox",
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
				err = db.QueryRow("SELECT COUNT(*) FROM lists WHERE name = ?", tt.list.Name).Scan(&count)
				if err != nil {
					t.Fatalf("failed to query list: %v", err)
				}

				if count != 1 {
					t.Errorf("expected 1 list with name %q, got %d", tt.list.Name, count)
				}
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
