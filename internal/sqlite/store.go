package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/danrneal/gtd.nvim/internal/model"
	_ "github.com/mattn/go-sqlite3"
)

// Store manages the SQLite database connection and executes queries.
type Store struct {
	db *sql.DB
}

// NewStore initializes a new SQLite store.
// It opens the database at dbPath, ensures it is accessible, and creates the necessary schema.
func NewStore(ctx context.Context, dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &Store{db: db}
	if err := store.createTables(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// createTables ensures that the required database tables exist.
func (s *Store) createTables(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			modified DATETIME NOT NULL,
			external_id TEXT
		);

		CREATE TABLE IF NOT EXISTS items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			list_id INTEGER NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			completed BOOLEAN NOT NULL DEFAULT 0,
			title TEXT NOT NULL,
			description TEXT,
			project_id TEXT,
			waiting_on TEXT,
			snoozed DATETIME,
			due DATETIME,
			tags TEXT NOT NULL DEFAULT '[]',
			modified DATETIME NOT NULL,
			created DATETIME NOT NULL,
			external_id TEXT
		);
	`

	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// CreateList inserts a new list into the database.
func (s *Store) CreateList(ctx context.Context, list model.List) error {
	query := `
		INSERT INTO lists (
			name,
			position,
			modified,
			external_id
		) VALUES (?, ?, ?, ?);
	`

	if _, err := s.db.ExecContext(ctx, query,
		list.Name,
		list.Position,
		list.Modified,
		list.ExternalID,
	); err != nil {
		return fmt.Errorf("failed to insert list: %w", err)
	}

	return nil
}
