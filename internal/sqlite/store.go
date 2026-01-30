package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

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
	list.Name = strings.TrimSpace(list.Name)
	if list.Name == "" {
		return fmt.Errorf("list name cannot be empty")
	}

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

// GetAllLists returns all lists from the database, populated with their items.
func (s *Store) GetAllLists(ctx context.Context) ([]model.List, error) {
	items, err := s.GetAllItems(ctx)
	if err != nil {
		return nil, err
	}

	itemsByListID := make(map[int64][]model.Item)
	for _, item := range items {
		itemsByListID[item.ListID] = append(itemsByListID[item.ListID], item)
	}

	query := `
		SELECT
			id,
			name,
			position,
			modified,
			external_id
		FROM lists
		ORDER BY position
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query lists: %w", err)
	}

	defer rows.Close()

	var lists []model.List
	for rows.Next() {
		var list model.List
		err := rows.Scan(
			&list.ID,
			&list.Name,
			&list.Position,
			&list.Modified,
			&list.ExternalID,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan list: %w", err)
		}

		if items, ok := itemsByListID[list.ID]; ok {
			list.Items = items
		} else {
			list.Items = []model.Item{}
		}

		lists = append(lists, list)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}

	return lists, nil
}

// CreateItem inserts a new item into the database.
func (s *Store) CreateItem(ctx context.Context, item model.Item) error {
	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		return fmt.Errorf("item title cannot be empty")
	}

	item.Description = multilineTrim(item.Description)
	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	query := `
                INSERT INTO items (
                        list_id,
                        position,
                        completed,
                        title,
                        description,
                        project_id,
                        waiting_on,
                        snoozed,
                        due,
                        tags,
                        modified,
                        created,
                        external_id
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
        `

	if _, err := s.db.ExecContext(ctx, query,
		item.ListID,
		item.Position,
		item.Completed,
		item.Title,
		item.Description,
		item.ProjectID,
		item.WaitingOn,
		item.Snoozed,
		item.Due,
		string(tagsJSON),
		item.Modified,
		item.Created,
		item.ExternalID,
	); err != nil {
		return fmt.Errorf("failed to insert item: %w", err)
	}

	return nil
}

// GetAllItems returns all items from the database.
func (s *Store) GetAllItems(ctx context.Context) ([]model.Item, error) {
	query := `
		SELECT
			id,
			list_id,
			position,
			completed,
			title,
			description,
			project_id,
			waiting_on,
			snoozed,
			due,
			tags,
			modified,
			created,
			external_id
		FROM items
		ORDER BY position
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query items: %w", err)
	}

	defer rows.Close()

	var items []model.Item
	for rows.Next() {
		var item model.Item
		var tagsJSON string
		err := rows.Scan(
			&item.ID,
			&item.ListID,
			&item.Position,
			&item.Completed,
			&item.Title,
			&item.Description,
			&item.ProjectID,
			&item.WaitingOn,
			&item.Snoozed,
			&item.Due,
			&tagsJSON,
			&item.Modified,
			&item.Created,
			&item.ExternalID,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}

		if tagsJSON != "" {
			if err := json.Unmarshal([]byte(tagsJSON), &item.Tags); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tags for item %d: %w", item.ID, err)
			}
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}

	return items, nil
}

func multilineTrim(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		line = strings.TrimRightFunc(line, unicode.IsSpace)
		lines[i] = line
	}

	s = strings.Join(lines, "\n")

	return s
}
