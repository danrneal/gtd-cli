// Package sqlite implements the SQLite database provider.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/providers/util/text"
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
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &Store{db: db}
	if err := store.createTables(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

// createTables ensures that the required database tables exist and have the correct constraints.
func (s *Store) createTables(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS lists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'open',
			modified DATETIME NOT NULL,
			external_id TEXT UNIQUE
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
			external_id TEXT UNIQUE
		);
	`

	if _, err := s.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// CreateList inserts a new list into the database.
func (s *Store) CreateList(ctx context.Context, list model.List) (string, error) {
	list.ID = uuid.NewString()[:8]
	if list.Status != "" && list.Status != model.StatusOpen {
		return "", errors.New("new lists must have status 'open'")
	}

	list.Name = strings.TrimSpace(list.Name)
	if list.Name == "" {
		return "", errors.New("list name cannot be empty")
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

	_, err := s.db.ExecContext(ctx, query,
		list.ID,
		list.Name,
		list.Position,
		model.StatusOpen,
		list.Modified,
		list.ExternalID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert list %q: %w", list.Name, err)
	}

	return list.ID, nil
}

// ListLists returns all lists from the database, populated with their items.
// It uses a read-only transaction to ensure a consistent snapshot of lists and items.
func (s *Store) ListLists(ctx context.Context) ([]model.List, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	items, err := s.listAllItems(ctx, tx)
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

	rows, err := tx.QueryContext(ctx, query)
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

// getListID resolves the internal list ID using the provided external ID.
func (s *Store) getListID(ctx context.Context, externalID *string) (string, error) {
	var id string
	query := `SELECT id FROM lists WHERE external_id = ?`
	row := s.db.QueryRowContext(ctx, query, externalID)
	err := row.Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("list with external ID %v not found", externalID)
		}

		return "", fmt.Errorf("failed to scan list ID: %w", err)
	}

	return id, nil
}

// UpdateList updates an existing list in the database.
//
// Parameters:
//   - list: The list with the desired state. It identifies the record via ID or ExternalID.
//   - currentItems: The items currently associated with this list, used to optimize position updates.
func (s *Store) UpdateList(ctx context.Context, list model.List, currentItems []model.Item) error {
	if list.ID == "" && list.ExternalID == nil {
		return errors.New("failed to update list: no internal or external ID provided")
	}

	if list.Status == "" {
		list.Status = model.StatusOpen
	}

	if !isValidListStatus(list.Status) {
		return fmt.Errorf("invalid list status: %q", list.Status)
	}

	list.Name = strings.TrimSpace(list.Name)
	if list.Name == "" {
		return errors.New("list name cannot be empty")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	query := `
                UPDATE lists SET
                        name = ?,
                        position = ?,
                        status = ?,
                        modified = ?,
                        external_id = COALESCE(?, external_id)
                WHERE id = ? OR external_id = ?;
        `

	res, err := tx.ExecContext(ctx, query,
		list.Name,
		list.Position,
		list.Status,
		list.Modified,
		list.ExternalID,
		list.ID,
		list.ExternalID,
	)
	if err != nil {
		return fmt.Errorf("failed to update list %q (ID: %s): %w", list.Name, list.ID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("list with ID %q or ExternalID %v not found", list.ID, list.ExternalID)
	}

	if list.ID == "" {
		listID, err := s.getListID(ctx, list.ExternalID)
		if err != nil {
			return err
		}

		list.ID = listID
	}

	if list.Status == model.StatusDeleted {
		if err := s.deleteListItems(ctx, tx, list); err != nil {
			return err
		}
	}

	for i, item := range list.Items {
		if i < len(currentItems) && item.ID == currentItems[i].ID {
			continue
		}

		item.ListID = list.ID
		if err := s.updateItemLocation(ctx, tx, item); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteList deletes a list from the database.
func (s *Store) DeleteList(ctx context.Context, list model.List) error {
	if list.ID == "" && list.ExternalID == nil {
		return errors.New("failed to delete list: no internal or external ID provided")
	}

	query := `DELETE FROM lists WHERE id = ? OR external_id = ?;`
	res, err := s.db.ExecContext(ctx, query, list.ID, list.ExternalID)
	if err != nil {
		return fmt.Errorf("failed to delete list %q (ID: %s): %w", list.Name, list.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("list with ID %q or ExternalID %v not found", list.ID, list.ExternalID)
	}

	return nil
}

// CreateItem inserts a new item into the database.
// If item.ListID is empty, it attempts to resolve it using item.ExternalListID.
// The previousItemID parameter is ignored by the SQLite store but kept for interface consistency.
func (s *Store) CreateItem(ctx context.Context, item model.Item, _ string) (string, error) {
	item.ID = uuid.NewString()[:8]
	if !isValidItemStatus(item.Status) {
		return "", fmt.Errorf("invalid status: %q", item.Status)
	}

	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		return "", errors.New("item title cannot be empty")
	}

	item.Description = text.MultilineTrim(item.Description)
	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tags: %w", err)
	}

	if item.ListID == "" {
		var listID string
		listID, err = s.getListID(ctx, item.ExternalListID)
		if err != nil {
			return "", err
		}

		item.ListID = listID
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
                        external_id
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
        `

	_, err = s.db.ExecContext(ctx, query,
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
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert item %q: %w", item.Title, err)
	}

	return item.ID, nil
}

// listAllItems returns all items from the database using the provided transaction.
func (s *Store) listAllItems(ctx context.Context, tx *sql.Tx) ([]model.Item, error) {
	query := `
		SELECT
			i.id,
			i.list_id,
			i.position,
			i.status,
			i.title,
			i.description,
			i.project_id,
			i.waiting_on,
			i.snoozed,
			i.due,
			i.tags,
			i.modified,
			i.created,
			i.external_id,
			l.external_id AS external_list_id
		FROM items i
		INNER JOIN lists l
		ON i.list_id = l.id
		ORDER BY i.position
	`

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query items: %w", err)
	}

	defer rows.Close()

	var items []model.Item
	for rows.Next() {
		var (
			item     model.Item
			tagsJSON string
		)

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
// It identifies the record via ID or ExternalID.
func (s *Store) UpdateItem(ctx context.Context, item model.Item) error {
	if item.ID == "" && item.ExternalID == nil {
		return errors.New("failed to update item: no internal or external ID provided")
	}

	if !isValidItemStatus(item.Status) {
		return fmt.Errorf("invalid status: %q", item.Status)
	}

	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		return errors.New("item title cannot be empty")
	}

	item.Description = text.MultilineTrim(item.Description)
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
                        external_id = COALESCE(?, external_id)
                WHERE id = ? OR external_id = ?;
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
		item.ExternalID,
	)
	if err != nil {
		return fmt.Errorf("failed to update item %q (ID: %s): %w", item.Title, item.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("item with ID %q or ExternalID %v not found", item.ID, item.ExternalID)
	}

	return nil
}

// updateItemLocation updates the list_id and position of an item.
// It identifies the record via ID or ExternalID.
func (s *Store) updateItemLocation(ctx context.Context, tx *sql.Tx, item model.Item) error {
	if item.ID == "" && item.ExternalID == nil {
		return errors.New("failed to update item location: no internal or external ID provided")
	}

	query := `
		UPDATE items SET
			list_id = ?,
			position = ?
		WHERE id = ? OR external_id = ?;
	`

	res, err := tx.ExecContext(ctx, query,
		item.ListID,
		item.Position,
		item.ID,
		item.ExternalID,
	)
	if err != nil {
		return fmt.Errorf("failed to move item %q (ID: %s): %w", item.Title, item.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("item with ID %q or ExternalID %v not found", item.ID, item.ExternalID)
	}

	return nil
}

// DeleteItem deletes an item from the database.
func (s *Store) DeleteItem(ctx context.Context, item model.Item) error {
	if item.ID == "" && item.ExternalID == nil {
		return errors.New("failed to delete item: no internal or external ID provided")
	}

	query := `DELETE FROM items WHERE id = ? OR external_id = ?;`
	res, err := s.db.ExecContext(ctx, query, item.ID, item.ExternalID)
	if err != nil {
		return fmt.Errorf("failed to delete item %q (ID: %s): %w", item.Title, item.ID, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("item with ID %q or ExternalID %v not found", item.ID, item.ExternalID)
	}

	return nil
}

// deleteListItems hard-deletes all items associated with the given list.
// It is used to clean up orphaned items when a list is soft-deleted (tombstoned).
func (s *Store) deleteListItems(ctx context.Context, tx *sql.Tx, list model.List) error {
	query := `DELETE FROM items WHERE list_id = ?`
	if _, err := tx.ExecContext(ctx, query, list.ID); err != nil {
		return fmt.Errorf("failed to delete items from list %q (ID: %s): %w", list.Name, list.ID, err)
	}

	return nil
}

// isValidListStatus checks if the provided status is a valid enum value for lists.
func isValidListStatus(status model.Status) bool {
	switch status {
	case model.StatusOpen, model.StatusDeleted:
		return true
	default:
		return false
	}
}

// isValidItemStatus checks if the provided status is a valid enum value for items.
func isValidItemStatus(status model.Status) bool {
	switch status {
	case model.StatusNotStarted, model.StatusInProgress, model.StatusDone, model.StatusDeleted:
		return true
	default:
		return false
	}
}
