package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// Store manages the SQLite database connection and executes queries.
type Store struct {
	db *sql.DB
}

// NewStore initializes a new SQLite store.
// It opens the database at dbPath, ensures it is accessible, and creates the necessary schema.
func NewStore(ctx context.Context, dbPath string) (*Store, error) {
	dataSourceName := fmt.Sprintf("%s?_foreign_keys=on", dbPath)
	db, err := sql.Open("sqlite3", dataSourceName)
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
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'open',
			modified DATETIME NOT NULL,
			external_id TEXT
		);

		CREATE TABLE IF NOT EXISTS items (
			id TEXT PRIMARY KEY,
			list_id TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
			position INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'not_started',
			title TEXT NOT NULL,
			description TEXT,
			project_id TEXT,
			waiting_on TEXT,
			snoozed DATETIME,
			due DATETIME,
			tags TEXT NOT NULL DEFAULT '[]',
			modified DATETIME NOT NULL,
			created DATETIME NOT NULL,
			external_id TEXT,
			external_list_id TEXT
		);
	`

	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// CreateList inserts a new list into the database.
func (s *Store) CreateList(ctx context.Context, list model.List) (model.List, error) {
	list.ID = uuid.NewString()[:8]
	list.Name = strings.TrimSpace(list.Name)
	if list.Name == "" {
		return model.List{}, fmt.Errorf("list name cannot be empty")
	}

	query := `
		INSERT INTO lists (
			id,
			name,
			position,
			status,
			modified,
			external_id
		) VALUES (?, ?, ?, ?, ?, ?);
	`

	if _, err := s.db.ExecContext(ctx, query,
		list.ID,
		list.Name,
		list.Position,
		model.StatusOpen,
		list.Modified,
		list.ExternalID,
	); err != nil {
		return model.List{}, fmt.Errorf("failed to insert list: %w", err)
	}

	return list, nil
}

// ListLists returns all lists from the database, populated with their items.
func (s *Store) ListLists(ctx context.Context) ([]model.List, error) {
	items, err := s.listAllItems(ctx)
	if err != nil {
		return nil, err
	}

	itemsByListID := make(map[string][]model.Item)
	for _, item := range items {
		itemsByListID[item.ListID] = append(itemsByListID[item.ListID], item)
	}

	query := `
		SELECT
			id,
			name,
			position,
			status,
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
			&list.Status,
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

// UpdateList updates an existing list in the database.
//
// Parameters:
//   - list: The list with the desired state (including Items with updated positions).
//   - currentItems: The items currently associated with this list (ordered by position).
//     This is used to optimize updates by skipping items that haven't changed.
func (s *Store) UpdateList(ctx context.Context, list model.List, currentItems []model.Item) error {
	list.Name = strings.TrimSpace(list.Name)
	if list.Name == "" {
		return fmt.Errorf("list name cannot be empty")
	}

	query := `
                UPDATE lists SET
                        name = ?,
                        position = ?,
                        status = ?,
                        modified = ?,
                        external_id = ?
                WHERE id = ?;
        `

	res, err := s.db.ExecContext(ctx, query,
		list.Name,
		list.Position,
		list.Status,
		list.Modified,
		list.ExternalID,
		list.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update list %s: %w", list.ID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("list with id %s not found", list.ID)
	}

	for i, item := range list.Items {
		if i < len(currentItems) && item.ID == currentItems[i].ID {
			continue
		}

		if err := s.updateItemLocation(ctx, item); err != nil {
			return err
		}
	}

	return nil
}

// DeleteList deletes a list from the database.
func (s *Store) DeleteList(ctx context.Context, id string) error {
	query := `DELETE FROM lists WHERE id = ?;`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete list %s: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("list with id %s not found", id)
	}

	return nil
}

// CreateItem inserts a new item into the database.
// The previousItemID parameter is ignored by the SQLite store but kept for interface consistency.
func (s *Store) CreateItem(ctx context.Context, item model.Item, _ string) (model.Item, error) {
	item.ID = uuid.NewString()[:8]
	if !isValidStatus(item.Status) {
		return model.Item{}, fmt.Errorf("invalid status: %q", item.Status)
	}

	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		return model.Item{}, fmt.Errorf("item title cannot be empty")
	}

	item.Description = multilineTrim(item.Description)
	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		return model.Item{}, fmt.Errorf("failed to marshal tags: %w", err)
	}

	query := `
                INSERT INTO items (
                        id,
                        list_id,
                        position,
                        status,
                        title,
                        description,
                        project_id,
                        waiting_on,
                        snoozed,
                        due,
                        tags,
                        modified,
                        created,
                        external_id,
			external_list_id
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
        `

	if _, err := s.db.ExecContext(ctx, query,
		item.ID,
		item.ListID,
		item.Position,
		item.Status,
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
		item.ExternalListID,
	); err != nil {
		return model.Item{}, fmt.Errorf("failed to insert item: %w", err)
	}

	return item, nil
}

// listAllItems returns all items from the database.
func (s *Store) listAllItems(ctx context.Context) ([]model.Item, error) {
	query := `
		SELECT
			id,
			list_id,
			position,
			status,
			title,
			description,
			project_id,
			waiting_on,
			snoozed,
			due,
			tags,
			modified,
			created,
			external_id,
			external_list_id
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
			&item.Status,
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
			&item.ExternalListID,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}

		if tagsJSON != "" {
			if err := json.Unmarshal([]byte(tagsJSON), &item.Tags); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tags for item %s: %w", item.ID, err)
			}
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}

	return items, nil
}

// UpdateItem updates an existing item in the database.
func (s *Store) UpdateItem(ctx context.Context, item model.Item) error {
	if !isValidStatus(item.Status) {
		return fmt.Errorf("invalid status: %q", item.Status)
	}

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
                UPDATE items SET
                        status = ?,
                        title = ?,
                        description = ?,
                        project_id = ?,
                        waiting_on = ?,
                        snoozed = ?,
                        due = ?,
                        tags = ?,
                        modified = ?,
                        external_id = ?
                WHERE id = ?;
        `

	res, err := s.db.ExecContext(ctx, query,
		item.Status,
		item.Title,
		item.Description,
		item.ProjectID,
		item.WaitingOn,
		item.Snoozed,
		item.Due,
		string(tagsJSON),
		item.Modified,
		item.ExternalID,
		item.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update item %s: %w", item.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("item with id %s not found", item.ID)
	}

	return nil
}

// updateItemLocation updates the list_id, position, and external_list_id of an item.
// It is used to implement moves and reordering within UpdateList.
func (s *Store) updateItemLocation(ctx context.Context, item model.Item) error {
	query := `
		UPDATE items SET 
			list_id = ?, 
			position = ?,
			external_list_id = ?
		WHERE id = ?;
	`

	res, err := s.db.ExecContext(ctx, query,
		item.ListID,
		item.Position,
		item.ExternalListID,
		item.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to move item %s: %w", item.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("item with id %s not found", item.ID)
	}

	return nil
}

// DeleteItem deletes an item from the database.
func (s *Store) DeleteItem(ctx context.Context, item model.Item) error {
	query := `DELETE FROM items WHERE id = ?;`
	res, err := s.db.ExecContext(ctx, query, item.ID)
	if err != nil {
		return fmt.Errorf("failed to delete item %s: %w", item.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("item with id %s not found", item.ID)
	}

	return nil
}

// isValidStatus checks if the provided status is a valid enum value.
func isValidStatus(status model.Status) bool {
	switch status {
	case model.StatusNotStarted, model.StatusInProgress, model.StatusDone, model.StatusDeleted:
		return true
	default:
		return false
	}
}

// multilineTrim trims whitespace from the beginning of the first line and the end of all lines.
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
