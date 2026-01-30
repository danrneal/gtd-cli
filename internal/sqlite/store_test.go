package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danrneal/gtd.nvim/internal/sqlite"
)

func TestNewStore(t *testing.T) {
	tests := []struct {
		name        string
		dbPath   func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "valid database path",
			dbPath: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "valid.db")
			},
			wantErr: false,
		},
		{
			name: "invalid database path (non-existent directory)",
			dbPath: func(t *testing.T) string {
				return "/non/existent/path/db.sqlite"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := tt.dbPath(t)
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
			}
		})
	}
}
