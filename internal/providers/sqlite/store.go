// Package sqlite implements the SQLite database provider.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/danrneal/gtd.nvim/internal/model"
)

// Store manages the SQLite database connection and executes queries.
type Store struct {
	db *sql.DB
}

// NewStore initializes a new SQLite store.
// It opens the database at dbPath, ensures it is accessible, and creates the necessary schema.
func NewStore(ctx context.Context, dbPath string) (*Store, error) {
	dataSourceName := fmt.Sprintf("%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", dbPath)
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

// Close closes the underlying SQLite database connection, ensuring WAL and SHM files are cleaned up.
func (s *Store) Close() error {
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}

	return nil
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
func (s *Store) CreateList(ctx context.Context, list *model.List) error {
	list.Clean()
	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if list.Status == model.StatusDeleted {
		return errors.New("cannot create a list with status 'deleted'")
	}

	if list.ID == "" {
		list.ID = uuid.NewString()[:8]
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
		list.Status,
		list.Modified,
		list.ExternalID,
	)
	if err != nil {
		return fmt.Errorf("failed to insert list %q: %w", list.Name, err)
	}

	return nil
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

	itemsByListID := make(map[string][]*model.Item, len(items))
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
			list.Items = []*model.Item{}
		}

		lists = append(lists, list)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}

	return lists, nil
}

// getListID resolves the internal list ID using the provided external ID.
func (s *Store) getListID(ctx context.Context, tx *sql.Tx, externalID *string) (string, error) {
	if externalID == nil {
		return "", errors.New("externalID is nil")
	}

	var id string
	query := `SELECT id FROM lists WHERE external_id = ?`
	row := tx.QueryRowContext(ctx, query, externalID)
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
func (s *Store) UpdateList(ctx context.Context, list *model.List, currentItems []*model.Item) error {
	list.Clean()
	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if list.ID == "" && list.ExternalID == nil {
		return errors.New("failed to update list: no internal or external ID provided")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	if list.ID == "" {
		var listID string
		listID, err = s.getListID(ctx, tx, list.ExternalID)
		if err != nil {
			return err
		}

		list.ID = listID
	}

	query := `
    	UPDATE lists SET
        	name = ?,
            position = ?,
            status = ?,
            modified = ?,
            external_id = COALESCE(?, external_id)
        WHERE id = ?;
    `

	res, err := tx.ExecContext(ctx, query,
		list.Name,
		list.Position,
		list.Status,
		list.Modified,
		list.ExternalID,
		list.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update list %q (ID: %s): %w", list.Name, list.ID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("list with ID %q not found", list.ID)
	}

	if list.Status == model.StatusDeleted {
		if err := s.deleteListItems(ctx, tx, list); err != nil {
			return err
		}
	}

	itemsToMove := calculateItemsToMove(list, currentItems)
	if len(itemsToMove) > 0 {
		if err := s.batchMoveItems(ctx, tx, itemsToMove); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteList deletes a list from the database.
func (s *Store) DeleteList(ctx context.Context, list *model.List) error {
	if err := s.deleteResource(ctx, list); err != nil {
		return fmt.Errorf("failed to delete list %q (ID: %s): %w", list.Name, list.ID, err)
	}

	return nil
}

// CreateItem inserts a new item into the database.
// If item.ListID is empty, it attempts to resolve it using item.ExternalListID.
// The previousItemID parameter is ignored by the SQLite store but kept for interface consistency.
func (s *Store) CreateItem(ctx context.Context, item *model.Item, _ string) error {
	item.Clean()
	if err := item.Validate(); err != nil {
		return fmt.Errorf("invalid item: %w", err)
	}

	if item.Status == model.StatusDeleted {
		return errors.New("cannot create an item with status 'deleted'")
	}

	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	if item.ListID == "" {
		var listID string
		listID, err = s.getListID(ctx, tx, item.ExternalListID)
		if err != nil {
			return err
		}

		item.ListID = listID
	}

	if item.ID == "" {
		item.ID = uuid.NewString()[:8]
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

	_, err = tx.ExecContext(ctx, query,
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
		return fmt.Errorf("failed to insert item %q: %w", item.Title, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// listAllItems returns all items from the database using the provided transaction.
func (s *Store) listAllItems(ctx context.Context, tx *sql.Tx) ([]*model.Item, error) {
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
		ORDER BY
			CASE i.status
				WHEN ? THEN 0
				WHEN ? THEN 1
				WHEN ? THEN 2
				ELSE 3
			END,
			i.position
	`

	rows, err := tx.QueryContext(ctx, query,
		model.StatusInProgress,
		model.StatusNotStarted,
		model.StatusDone,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query items: %w", err)
	}

	defer rows.Close()

	var items []*model.Item
	for rows.Next() {
		var tagsJSON string
		item := &model.Item{}
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

// getItemID resolves the internal item ID using the provided external ID.
func (s *Store) getItemID(ctx context.Context, tx *sql.Tx, externalID *string) (string, error) {
	if externalID == nil {
		return "", errors.New("externalID is nil")
	}

	var id string
	query := `SELECT id FROM items WHERE external_id = ?`
	row := tx.QueryRowContext(ctx, query, externalID)
	err := row.Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("item with external ID %v not found", externalID)
		}

		return "", fmt.Errorf("failed to scan item ID: %w", err)
	}

	return id, nil
}

// UpdateItem updates an existing item in the database.
// It identifies the record via ID or ExternalID.
func (s *Store) UpdateItem(ctx context.Context, item *model.Item) error {
	item.Clean()
	if err := item.Validate(); err != nil {
		return fmt.Errorf("invalid item: %w", err)
	}

	if item.ID == "" && item.ExternalID == nil {
		return errors.New("failed to update item: no internal or external ID provided")
	}

	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	if item.ID == "" {
		var itemID string
		itemID, err = s.getItemID(ctx, tx, item.ExternalID)
		if err != nil {
			return err
		}

		item.ID = itemID
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
        WHERE id = ?;
	`

	res, err := tx.ExecContext(ctx, query,
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
		return fmt.Errorf("failed to update item %q (ID: %s): %w", item.Title, item.ID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("item with ID %q not found", item.ID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// batchMoveItems updates the list_id and position for a batch of items.
// It uses a single prepared statement for efficiency.
func (s *Store) batchMoveItems(ctx context.Context, tx *sql.Tx, items []*model.Item) error {
	query := `
		UPDATE items SET
			list_id = ?,
			position = ?
		WHERE id = ?;
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare batch move item statement: %w", err)
	}

	defer stmt.Close()

	for _, item := range items {
		if item.ID == "" && item.ExternalID == nil {
			return errors.New("failed to update item location: no internal or external ID provided")
		}

		if item.ID == "" {
			itemID, err := s.getItemID(ctx, tx, item.ExternalID)
			if err != nil {
				return err
			}

			item.ID = itemID
		}

		res, err := stmt.ExecContext(ctx,
			item.ListID,
			item.Position,
			item.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to move item %q (ID: %s): %w", item.Title, item.ID, err)
		}

		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}

		if rowsAffected == 0 {
			return fmt.Errorf("item with ID %q not found", item.ID)
		}
	}

	return nil
}

// DeleteItem deletes an item from the database.
func (s *Store) DeleteItem(ctx context.Context, item *model.Item) error {
	if err := s.deleteResource(ctx, item); err != nil {
		return fmt.Errorf("failed to delete item %q (ID: %s): %w", item.Title, item.ID, err)
	}

	return nil
}

// deleteListItems hard-deletes all items associated with the given list.
// It is used to clean up orphaned items when a list is soft-deleted (tombstoned).
func (s *Store) deleteListItems(ctx context.Context, tx *sql.Tx, list *model.List) error {
	query := `DELETE FROM items WHERE list_id = ?`
	if _, err := tx.ExecContext(ctx, query, list.ID); err != nil {
		return fmt.Errorf("failed to delete items from list %q (ID: %s): %w", list.Name, list.ID, err)
	}

	return nil
}

// deleteResource handles the boilerplate of resolving an ID and deleting a record within a transaction.
func (s *Store) deleteResource(ctx context.Context, resource model.Resource) error {
	if resource.GetID() == "" && resource.GetExternalID() == nil {
		return errors.New("failed to delete resource: no internal or external ID provided")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	var (
		query         string
		getResourceID func(context.Context, *sql.Tx, *string) (string, error)
	)

	switch resource.(type) {
	case *model.List:
		query = `DELETE FROM lists WHERE id = ?;`
		getResourceID = s.getListID
	case *model.Item:
		query = `DELETE FROM items WHERE id = ?;`
		getResourceID = s.getItemID
	default:
		return fmt.Errorf("unsupported resource type: %T", resource)
	}

	resourceID := resource.GetID()
	if resourceID == "" {
		resourceID, err = getResourceID(ctx, tx, resource.GetExternalID())
		if err != nil {
			return err
		}
	}

	res, err := tx.ExecContext(ctx, query, resourceID)
	if err != nil {
		return fmt.Errorf("failed to execute delete query for resource ID %q: %w", resourceID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("resource with ID %q not found", resourceID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// calculateItemsToMove compares the incoming list's items against the current database state.
// It returns a slice of items whose Position or ListID have changed and require a database update.
func calculateItemsToMove(list *model.List, currentItems []*model.Item) []*model.Item {
	var itemsToMove []*model.Item
	for i, item := range list.Items {
		if i < len(currentItems) && item.ID == currentItems[i].ID {
			continue
		}

		item.ListID = list.ID
		itemsToMove = append(itemsToMove, item)
	}

	return itemsToMove
}
