// Package sqlite implements the SQLite database provider.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/danrneal/gtd-cli/model"
)

// ErrNotFound is returned when a requested record is not found in the database.
var ErrNotFound = errors.New("record not found")

// Store manages the SQLite database connection and executes queries.
type Store struct {
	db             *sql.DB
	generateListID func() string
	generateItemID func() string
	logger         *slog.Logger
}

// Option defines a functional option for configuring a Store.
type Option func(*Store)

// WithListIDGenerator overrides the default List ID generation function.
// This is primarily used for deterministic testing.
func WithListIDGenerator(fn func() string) Option {
	listIDGeneratorOpt := func(s *Store) {
		s.generateListID = fn
	}

	return listIDGeneratorOpt
}

// WithItemIDGenerator overrides the default Item ID generation function.
// This is primarily used for deterministic testing.
func WithItemIDGenerator(fn func() string) Option {
	itemIDGeneratorOpt := func(s *Store) {
		s.generateItemID = fn
	}

	return itemIDGeneratorOpt
}

// NewStore initializes a new SQLite store, opening the database at dbPath and creating the schema.
// Callers must call Close when done with the returned Store.
func NewStore(ctx context.Context, dbPath string, logger *slog.Logger, opts ...Option) (*Store, error) {
	dataSourceName := fmt.Sprintf("%s?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL", dbPath)
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)

	generateID := func() string {
		return uuid.NewString()[:8]
	}

	store := &Store{
		db:             db,
		generateListID: generateID,
		generateItemID: generateID,
		logger:         logger,
	}

	if err = store.createTables(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	for _, opt := range opts {
		opt(store)
	}

	return store, nil
}

// Close closes the underlying SQLite database connection, ensuring WAL and SHM files are cleaned up.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
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
			project_id TEXT REFERENCES items(id) ON DELETE SET NULL,
			position INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'not_started',
			title TEXT NOT NULL,
			description TEXT,
			project_tag TEXT,
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
		return fmt.Errorf("create tables: %w", err)
	}

	return nil
}

// CreateList inserts a new list into the database, wrapping [model.ErrInvalidResource] if validation fails.
func (s *Store) CreateList(ctx context.Context, list *model.List) error {
	list.Clean()
	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if list.Status == model.StatusDeleted {
		return errors.New("cannot create a list with status 'deleted'")
	}

	if list.ID == "" {
		list.ID = s.generateListID()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer tx.Rollback()

	if err = s.execInsertList(ctx, tx, list); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// execInsertList executes the SQL query to insert a new list record into the database.
// It handles formatting the timestamps and mapping the List struct fields to the query parameters.
func (s *Store) execInsertList(ctx context.Context, tx *sql.Tx, list *model.List) error {
	query := `
		UPDATE lists
		SET position = position + 1
		WHERE position >= ?
	`

	if _, err := tx.ExecContext(ctx, query, list.Position); err != nil {
		return fmt.Errorf("shift list positions: %w", err)
	}

	query = `
		INSERT INTO lists (
			id,
			name,
			position,
			status,
			modified,
			external_id
		) VALUES (?, ?, ?, ?, ?, ?);
	`

	s.logger.InfoContext(ctx, "SQLite: Inserting list", "id", list.ID, "name", list.Name)
	list.Modified = time.Now()
	if _, err := tx.ExecContext(ctx, query,
		list.ID,
		list.Name,
		list.Position,
		list.Status,
		list.Modified,
		list.ExternalID,
	); err != nil {
		return fmt.Errorf("insert list %q: %w", list.Name, err)
	}

	return nil
}

// ListLists returns all lists from the database, populated with their items.
// It uses a read-only transaction to ensure a consistent snapshot of lists and items.
func (s *Store) ListLists(ctx context.Context) ([]model.List, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}

	defer tx.Rollback()

	items, err := listAllItems(ctx, tx)
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
		return nil, fmt.Errorf("query lists: %w", err)
	}

	defer rows.Close()

	var lists []model.List
	i := 0
	for rows.Next() {
		var list model.List
		if err := rows.Scan(
			&list.ID,
			&list.Name,
			&list.Position,
			&list.Status,
			&list.Modified,
			&list.ExternalID,
		); err != nil {
			return nil, fmt.Errorf("scan list: %w", err)
		}

		list.Position = i
		i++

		if items, ok := itemsByListID[list.ID]; ok {
			list.Items = items
		} else {
			list.Items = []*model.Item{}
		}

		lists = append(lists, list)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate list rows: %w", err)
	}

	return lists, nil
}

// queryList retrieves a list from the database by its internal ID.
// It wraps ErrNotFound if the list does not exist.
func queryList(ctx context.Context, tx *sql.Tx, listID string) (*model.List, error) {
	var list model.List
	query := `
		SELECT
    		id,
            name,
            position,
            status,
            modified,
            external_id
        FROM lists
        WHERE id = ?
	`

	row := tx.QueryRowContext(ctx, query, listID)
	if err := row.Scan(
		&list.ID,
		&list.Name,
		&list.Position,
		&list.Status,
		&list.Modified,
		&list.ExternalID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: list with ID %q", ErrNotFound, listID)
		}

		return nil, fmt.Errorf("scan list: %w", err)
	}

	return &list, nil
}

// queryListID resolves the internal list ID using the provided external ID.
// It wraps ErrNotFound if no matching list exists.
func queryListID(ctx context.Context, tx *sql.Tx, externalID *string) (string, error) {
	var id string
	query := `SELECT id FROM lists WHERE external_id = ?`
	row := tx.QueryRowContext(ctx, query, externalID)
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: list with external ID %q", ErrNotFound, *externalID)
		}

		return "", fmt.Errorf("scan list ID: %w", err)
	}

	return id, nil
}

// UpdateList updates an existing list in the database, identifying it via ID or ExternalID
// and using currentList.Items to compute minimal item position updates.
// It wraps [model.ErrInvalidResource] if validation fails or ErrNotFound if the list or any moved item does not exist.
func (s *Store) UpdateList(ctx context.Context, list, currentList *model.List) error {
	list.Clean()
	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if list.ID == "" && list.ExternalID == nil {
		return errors.New("update list: no internal or external ID provided")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer tx.Rollback()

	if list.ID == "" {
		var listID string
		listID, err = queryListID(ctx, tx, list.ExternalID)
		if err != nil {
			return err
		}

		list.ID = listID
	}

	if err = s.execUpdateList(ctx, tx, list); err != nil {
		return err
	}

	if list.Status == model.StatusDeleted {
		if err := deleteListItems(ctx, tx, list); err != nil {
			return err
		}
	}

	itemsToMove := calculateItemsToMove(list, currentList.Items)
	if err := batchMoveItems(ctx, tx, itemsToMove); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// execUpdateList executes the SQL query to update an existing list record in the database.
// It wraps ErrNotFound if the list does not exist.
func (s *Store) execUpdateList(ctx context.Context, tx *sql.Tx, list *model.List) error {
	query := `
    	UPDATE lists SET
        	name = ?,
            position = ?,
            status = ?,
            modified = ?,
            external_id = COALESCE(?, external_id)
        WHERE id = ?;
    `

	s.logger.InfoContext(ctx, "SQLite: Updating list", "id", list.ID, "name", list.Name)
	list.Modified = time.Now()
	res, err := tx.ExecContext(ctx, query,
		list.Name,
		list.Position,
		list.Status,
		list.Modified,
		list.ExternalID,
		list.ID,
	)
	if err != nil {
		return fmt.Errorf("update list %q (ID: %s): %w", list.Name, list.ID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%w: list with ID %q", ErrNotFound, list.ID)
	}

	return nil
}

// DeleteList deletes a list from the database, wrapping ErrNotFound if it does not exist.
func (s *Store) DeleteList(ctx context.Context, list *model.List) error {
	s.logger.InfoContext(ctx, "SQLite: Deleting list", "id", list.ID, "name", list.Name)
	if err := s.deleteResource(ctx, list); err != nil {
		return fmt.Errorf("delete list %q (ID: %s): %w", list.Name, list.ID, err)
	}

	return nil
}

// CreateItem inserts a new item into the database, resolving item.ExternalListID when item.ListID is empty.
// The previousItemID parameter is ignored by the SQLite store for interface consistency.
// It wraps [model.ErrInvalidResource] if validation fails or ErrNotFound if the target list does not exist.
func (s *Store) CreateItem(ctx context.Context, item *model.Item, _ string) error {
	item.Clean()
	if err := item.Validate(); err != nil {
		return fmt.Errorf("invalid item: %w", err)
	}

	if item.Status == model.StatusDeleted {
		return errors.New("cannot create an item with status 'deleted'")
	}

	if item.ID == "" {
		item.ID = s.generateItemID()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer tx.Rollback()

	if item.ListID == "" {
		var listID string
		listID, err = queryListID(ctx, tx, item.ExternalListID)
		if err != nil {
			return err
		}

		item.ListID = listID
	}

	list, err := queryList(ctx, tx, item.ListID)
	if err != nil {
		return err
	}

	if err = s.execInsertItem(ctx, tx, list, item); err != nil {
		return err
	}

	items, err := listItems(ctx, tx, list.ID)
	if err != nil {
		return err
	}

	list.Items = items

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	currentList := *list
	currentList.Items = slices.Clone(list.Items)
	err = s.UpdateList(ctx, list, &currentList)

	return err
}

// execInsertItem manages the database-level preparation and execution required to insert a new Item.
// It resolves the internal project ID if necessary, sanitizes the item's relationships based on the target list,
// and maps the fields to the SQL parameters.
func (s *Store) execInsertItem(ctx context.Context, tx *sql.Tx, list *model.List, item *model.Item) error {
	tagsJSON, _ := json.Marshal(item.Tags)

	var projectTag *string
	if strings.HasPrefix(list.Name, model.ListProjects) {
		projectTag = item.ProjectTag
	} else if item.ProjectID == nil && (item.ExternalProjectID != nil || item.ProjectTag != nil) {
		if projectID, err := queryProjectID(ctx, tx, item); err == nil {
			item.ProjectID = &projectID
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
	}

	query := `
		UPDATE items
		SET position = position + 1
		WHERE list_id = ? AND position >= ?
	`

	if _, err := tx.ExecContext(ctx, query, item.ListID, item.Position); err != nil {
		return fmt.Errorf("shift item positions: %w", err)
	}

	query = `
        INSERT INTO items (
            id,
            list_id,
			project_id,
            position,
            status,
            title,
            description,
            project_tag,
            waiting_on,
            snoozed,
            due,
            tags,
            modified,
            created,
            external_id
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
    `

	s.logger.InfoContext(ctx, "SQLite: Inserting item", "id", item.ID, "title", item.Title, "listId", item.ListID)
	item.Modified = time.Now()
	_, err := tx.ExecContext(ctx, query,
		item.ID,
		item.ListID,
		item.ProjectID,
		item.Position,
		item.Status,
		item.Title,
		item.Description,
		projectTag,
		item.WaitingOn,
		item.Snoozed,
		item.Due,
		string(tagsJSON),
		item.Modified,
		item.Created,
		item.ExternalID,
	)
	if err != nil {
		return fmt.Errorf("insert item %q: %w", item.Title, err)
	}

	return nil
}

// listAllItems returns all items from the database using the provided transaction.
func listAllItems(ctx context.Context, tx *sql.Tx) ([]*model.Item, error) {
	query := `
		SELECT
			i.id,
			i.list_id,
			i.project_id,
			i.position,
			i.status,
			i.title,
			i.description,
			COALESCE(project.project_tag, i.project_tag) AS project_tag,
			i.waiting_on,
			i.snoozed,
			i.due,
			i.tags,
			i.modified,
			i.created,
			i.external_id,
			l.external_id AS external_list_id,
			project.external_id AS external_project_id
		FROM items i
		INNER JOIN lists l
		ON i.list_id = l.id
		LEFT JOIN items project
		ON i.project_id = project.id
		ORDER BY i.position
	`

	items, err := queryItems(ctx, tx, query)

	return items, err
}

// listItems retrieves all items belonging to a specific list ID, ordered by position.
func listItems(ctx context.Context, tx *sql.Tx, listID string) ([]*model.Item, error) {
	query := `
		SELECT
			i.id,
			i.list_id,
			i.project_id,
			i.position,
			i.status,
			i.title,
			i.description,
			COALESCE(project.project_tag, i.project_tag) AS project_tag,
			i.waiting_on,
			i.snoozed,
			i.due,
			i.tags,
			i.modified,
			i.created,
			i.external_id,
			l.external_id AS external_list_id,
			project.external_id AS external_project_id
		FROM items i
		INNER JOIN lists l
		ON i.list_id = l.id
		LEFT JOIN items project
		ON i.project_id = project.id
		WHERE i.list_id = ?
		ORDER BY i.position
	`

	items, err := queryItems(ctx, tx, query, listID)

	return items, err
}

// queryItems executes the given item SELECT query and scans the resulting rows into Item structs.
func queryItems(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]*model.Item, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}

	defer rows.Close()

	var items []*model.Item
	for rows.Next() {
		var tagsJSON string
		item := &model.Item{}
		if err := rows.Scan(
			&item.ID,
			&item.ListID,
			&item.ProjectID,
			&item.Position,
			&item.Status,
			&item.Title,
			&item.Description,
			&item.ProjectTag,
			&item.WaitingOn,
			&item.Snoozed,
			&item.Due,
			&tagsJSON,
			&item.Modified,
			&item.Created,
			&item.ExternalID,
			&item.ExternalListID,
			&item.ExternalProjectID,
		); err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}

		if tagsJSON != "" {
			if err := json.Unmarshal([]byte(tagsJSON), &item.Tags); err != nil {
				return nil, fmt.Errorf("unmarshal tags for item %s: %w", item.ID, err)
			}
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item rows: %w", err)
	}

	return items, nil
}

// queryItemID resolves the internal item ID using the provided external ID.
// It wraps ErrNotFound if no matching item exists.
func queryItemID(ctx context.Context, tx *sql.Tx, externalID *string) (string, error) {
	var id string
	query := `SELECT id FROM items WHERE external_id = ?`
	row := tx.QueryRowContext(ctx, query, externalID)
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: item with external ID %q", ErrNotFound, *externalID)
		}

		return "", fmt.Errorf("scan item ID: %w", err)
	}

	return id, nil
}

// queryProjectID attempts to resolve the internal database ID for a project.
// It searches first by the item's ExternalProjectID, and falls back to looking up the item's ProjectTag
// among items in the "Projects" list. It returns ErrNotFound if the project cannot be located.
func queryProjectID(ctx context.Context, tx *sql.Tx, item *model.Item) (string, error) {
	if item.ExternalProjectID != nil {
		projectID, err := queryItemID(ctx, tx, item.ExternalProjectID)
		if err != nil {
			return "", err
		}

		return projectID, nil
	}

	if item.ProjectTag != nil {
		var projectID string
		query := `
			SELECT i.id
			FROM items i
			INNER JOIN lists l ON i.list_id = l.id
			WHERE l.name = ?
			AND i.project_tag = ?
		`

		row := tx.QueryRowContext(ctx, query, model.ListProjects, item.ProjectTag)
		if err := row.Scan(&projectID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("%w: item with project tag %q", ErrNotFound, *item.ProjectTag)
			}

			return "", fmt.Errorf("scan project ID: %w", err)
		}

		return projectID, nil
	}

	var projectID string
	query := `
		SELECT i.project_id
		FROM items i
		INNER JOIN items project ON i.project_id = project.id
		WHERE i.id = ?
		AND project.project_tag IS NULL
	`

	row := tx.QueryRowContext(ctx, query, item.ID)
	if err := row.Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: tagless parent project to coalesce", ErrNotFound)
		}

		return "", fmt.Errorf("check current project state: %w", err)
	}

	return projectID, nil
}

// UpdateItem updates an existing item in the database, identifying the record via ID or ExternalID.
// It wraps [model.ErrInvalidResource] if validation fails or ErrNotFound if the item or target list does not exist.
func (s *Store) UpdateItem(ctx context.Context, item *model.Item) error {
	item.Clean()
	if err := item.Validate(); err != nil {
		return fmt.Errorf("invalid item: %w", err)
	}

	if item.ID == "" && item.ExternalID == nil {
		return errors.New("update item: no internal or external ID provided")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer tx.Rollback()

	if item.ID == "" {
		var itemID string
		itemID, err = queryItemID(ctx, tx, item.ExternalID)
		if err != nil {
			return err
		}

		item.ID = itemID
	}

	if item.ListID == "" {
		var listID string
		listID, err = queryListID(ctx, tx, item.ExternalListID)
		if err != nil {
			return err
		}

		item.ListID = listID
	}

	list, err := queryList(ctx, tx, item.ListID)
	if err != nil {
		return err
	}

	if err = s.execUpdateItem(ctx, tx, list, item); err != nil {
		return err
	}

	items, err := listItems(ctx, tx, list.ID)
	if err != nil {
		return err
	}

	list.Items = items

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	currentList := *list
	currentList.Items = slices.Clone(list.Items)
	err = s.UpdateList(ctx, list, &currentList)

	return err
}

// execUpdateItem manages the database-level execution required to update an existing Item.
// It resolves the internal project ID if necessary, enforces relationship constraints based on the target list,
// and wraps ErrNotFound if the item does not exist.
func (s *Store) execUpdateItem(ctx context.Context, tx *sql.Tx, list *model.List, item *model.Item) error {
	tagsJSON, _ := json.Marshal(item.Tags)

	var projectTag *string
	if strings.HasPrefix(list.Name, model.ListProjects) {
		projectTag = item.ProjectTag
		query := `
			UPDATE items
			SET modified = ?
			WHERE
  				project_id = ?
  				AND ? IS NOT (
    				SELECT project_tag
    				FROM items
    				WHERE id = ?
  				);
		`

		if _, err := tx.ExecContext(ctx, query, item.Modified, item.ID, projectTag, item.ID); err != nil {
			return fmt.Errorf("update project next action modified timestamps: %w", err)
		}
	} else if item.ProjectID == nil &&
		(item.ExternalProjectID != nil || item.ProjectTag != nil || item.ExternalID == nil) {
		if projectID, err := queryProjectID(ctx, tx, item); err == nil {
			item.ProjectID = &projectID
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
	}

	query := `
        UPDATE items SET
			project_id = ?,
            status = ?,
            title = ?,
            description = ?,
            project_tag = COALESCE(?, project_tag),
            waiting_on = ?,
            snoozed = ?,
            due = ?,
            tags = ?,
            modified = ?,
            created = ?,
            external_id = COALESCE(?, external_id)
        WHERE id = ?;
    `

	s.logger.InfoContext(ctx, "SQLite: Updating item", "id", item.ID, "title", item.Title, "status", item.Status)
	item.Modified = time.Now()
	res, err := tx.ExecContext(ctx, query,
		item.ProjectID,
		item.Status,
		item.Title,
		item.Description,
		projectTag,
		item.WaitingOn,
		item.Snoozed,
		item.Due,
		string(tagsJSON),
		item.Modified,
		item.Created,
		item.ExternalID,
		item.ID,
	)
	if err != nil {
		return fmt.Errorf("update item %q (ID: %s): %w", item.Title, item.ID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%w: item with ID %q", ErrNotFound, item.ID)
	}

	return nil
}

// batchMoveItems updates the list_id and position for a batch of items using a single prepared statement.
// It wraps ErrNotFound if any moved item does not exist.
func batchMoveItems(ctx context.Context, tx *sql.Tx, items []*model.Item) error {
	if len(items) == 0 {
		return nil
	}

	query := `
		UPDATE items SET
			list_id = ?,
			position = ?
		WHERE id = ?;
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare batch move item statement: %w", err)
	}

	defer stmt.Close()

	for _, item := range items {
		if item.ID == "" && item.ExternalID == nil {
			return errors.New("update item location: no internal or external ID provided")
		}

		if item.ID == "" {
			itemID, err := queryItemID(ctx, tx, item.ExternalID)
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
			return fmt.Errorf("move item %q (ID: %s): %w", item.Title, item.ID, err)
		}

		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("check rows affected: %w", err)
		}

		if rowsAffected == 0 {
			return fmt.Errorf("%w: item with ID %q", ErrNotFound, item.ID)
		}
	}

	return nil
}

// DeleteItem deletes an item from the database, wrapping ErrNotFound if it does not exist.
func (s *Store) DeleteItem(ctx context.Context, item *model.Item) error {
	s.logger.InfoContext(ctx, "SQLite: Deleting item", "id", item.ID, "title", item.Title)
	if err := s.deleteResource(ctx, item); err != nil {
		return fmt.Errorf("delete item %q (ID: %s): %w", item.Title, item.ID, err)
	}

	return nil
}

// deleteListItems hard-deletes all items associated with the given list.
// It is used to clean up orphaned items when a list is soft-deleted (tombstoned).
func deleteListItems(ctx context.Context, tx *sql.Tx, list *model.List) error {
	query := `DELETE FROM items WHERE list_id = ?`
	if _, err := tx.ExecContext(ctx, query, list.ID); err != nil {
		return fmt.Errorf("delete items from list %q (ID: %s): %w", list.Name, list.ID, err)
	}

	return nil
}

// deleteResource resolves a resource ID and deletes its record within a transaction.
// It wraps ErrNotFound if the record does not exist.
func (s *Store) deleteResource(ctx context.Context, resource model.Resource) error {
	if resource.GetID() == "" && resource.GetExternalID() == nil {
		return errors.New("delete resource: no internal or external ID provided")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer tx.Rollback()

	var (
		query           string
		queryResourceID func(context.Context, *sql.Tx, *string) (string, error)
	)

	switch resource.(type) {
	case *model.List:
		query = `DELETE FROM lists WHERE id = ?;`
		queryResourceID = queryListID
	case *model.Item:
		query = `DELETE FROM items WHERE id = ?;`
		queryResourceID = queryItemID
	}

	resourceID := resource.GetID()
	if resourceID == "" {
		resourceID, err = queryResourceID(ctx, tx, resource.GetExternalID())
		if err != nil {
			return err
		}
	}

	if _, ok := resource.(*model.Item); ok {
		projQuery := `UPDATE items SET modified = ? WHERE project_id = ?`
		if _, err = tx.ExecContext(ctx, projQuery, time.Now(), resourceID); err != nil {
			return fmt.Errorf("update project next action modified timestamps: %w", err)
		}
	}

	res, err := tx.ExecContext(ctx, query, resourceID)
	if err != nil {
		return fmt.Errorf("execute delete query for resource ID %q: %w", resourceID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("%w: resource with ID %q", ErrNotFound, resourceID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
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
