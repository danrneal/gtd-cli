// Package markdown implements the markdown file provider.
package markdown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/danrneal/gtd.nvim/internal/model"
)

// Client is a markdown file provider client.
type Client struct {
	filepath string
}

// NewClient creates a new markdown client with the given file path.
func NewClient(filepath string) *Client {
	client := &Client{filepath: filepath}

	return client
}

// GetKey extracts the ID from the resource.
func (c *Client) GetKey(resource model.Resource) string {
	if id := resource.GetID(); id != "" {
		return id
	}

	return ""
}

// CreateList creates a new list in the markdown file.
func (c *Client) CreateList(_ context.Context, list *model.List) error {
	list.Clean()
	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if list.Status == model.StatusDeleted {
		return errors.New("cannot create a list with status 'deleted'")
	}

	lists, err := c.readFile()
	if err != nil {
		return err
	}

	lists = append(lists, *list)
	err = c.writeFile(lists)

	return err
}

// ListLists retrieves all lists from the markdown file.
func (c *Client) ListLists(_ context.Context) ([]model.List, error) {
	lists, err := c.readFile()
	return lists, err
}

// UpdateList updates an existing list and its items in the markdown file.
func (c *Client) UpdateList(_ context.Context, list *model.List, _ []*model.Item) error {
	list.Clean()
	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if list.ID == "" {
		return errors.New("failed to update list: missing ID")
	}

	lists, err := c.readFile()
	if err != nil {
		return err
	}

	idx := slices.IndexFunc(lists, func(l model.List) bool {
		return l.ID == list.ID
	})

	if idx == -1 {
		return fmt.Errorf("failed to update list: list %q not found", list.ID)
	}

	lists[idx] = *list
	err = c.writeFile(lists)

	return err
}

// DeleteList removes a list from the markdown file.
func (c *Client) DeleteList(_ context.Context, list *model.List) error {
	if list.ID == "" {
		return errors.New("failed to delete list: missing ID")
	}

	lists, err := c.readFile()
	if err != nil {
		return err
	}

	idx := slices.IndexFunc(lists, func(l model.List) bool {
		return l.ID == list.ID
	})

	if idx == -1 {
		return fmt.Errorf("failed to delete list: list %q not found", list.ID)
	}

	lists = slices.Delete(lists, idx, idx+1)
	err = c.writeFile(lists)

	return err
}

// CreateItem adds a new item to a list in the markdown file.
func (c *Client) CreateItem(_ context.Context, item *model.Item, previousItemID string) error {
	item.Clean()
	if err := item.Validate(); err != nil {
		return fmt.Errorf("invalid item: %w", err)
	}

	if item.Status == model.StatusDeleted {
		return errors.New("cannot create an item with status 'deleted'")
	}

	if item.ListID == "" {
		return errors.New("failed to create item: missing list ID")
	}

	lists, err := c.readFile()
	if err != nil {
		return err
	}

	listIdx := slices.IndexFunc(lists, func(l model.List) bool {
		return l.ID == item.ListID
	})

	if listIdx == -1 {
		return fmt.Errorf("failed to create item: list %q not found", item.ListID)
	}

	list := lists[listIdx]

	prevItemIdx := slices.IndexFunc(list.Items, func(i *model.Item) bool {
		return previousItemID != "" && i.ID == previousItemID
	})

	if prevItemIdx == -1 && previousItemID != "" {
		return fmt.Errorf("state corruption: anchor item %q not found in markdown", previousItemID)
	}

	list.Items = slices.Insert(list.Items, prevItemIdx+1, item)
	lists[listIdx] = list
	err = c.writeFile(lists)

	return err
}

// UpdateItem updates an existing item in the markdown file.
func (c *Client) UpdateItem(_ context.Context, item *model.Item) error {
	item.Clean()
	if err := item.Validate(); err != nil {
		return fmt.Errorf("invalid item: %w", err)
	}

	if item.ID == "" || item.ListID == "" {
		return errors.New("failed to update item: missing identifiers")
	}

	lists, err := c.readFile()
	if err != nil {
		return err
	}

	listIdx := slices.IndexFunc(lists, func(l model.List) bool {
		return l.ID == item.ListID
	})

	if listIdx == -1 {
		return fmt.Errorf("failed to update item: list %q not found", item.ListID)
	}

	list := lists[listIdx]

	itemIdx := slices.IndexFunc(list.Items, func(i *model.Item) bool {
		return i.ID == item.ID
	})

	if itemIdx == -1 {
		return fmt.Errorf("failed to update item: item %q not found", item.ID)
	}

	list.Items[itemIdx] = item
	lists[listIdx] = list
	err = c.writeFile(lists)

	return err
}

// DeleteItem removes an item from the markdown file.
func (c *Client) DeleteItem(_ context.Context, item *model.Item) error {
	if item.ID == "" || item.ListID == "" {
		return errors.New("failed to delete item: missing identifiers")
	}

	lists, err := c.readFile()
	if err != nil {
		return err
	}

	listIdx := slices.IndexFunc(lists, func(l model.List) bool {
		return l.ID == item.ListID
	})

	if listIdx == -1 {
		return fmt.Errorf("failed to delete item: list %q not found", item.ListID)
	}

	list := lists[listIdx]

	itemIdx := slices.IndexFunc(list.Items, func(i *model.Item) bool {
		return i.ID == item.ID
	})

	if itemIdx == -1 {
		return fmt.Errorf("failed to delete item: item %q not found", item.ID)
	}

	list.Items = slices.Delete(list.Items, itemIdx, itemIdx+1)
	lists[listIdx] = list
	err = c.writeFile(lists)

	return err
}

// readFile opens the markdown file, reads its modification time, and parses its contents.
// If the file does not exist, it safely returns an empty slice to allow for bootstrapping.
func (c *Client) readFile() ([]model.List, error) {
	file, err := os.Open(c.filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to open markdown file: %w", err)
	}

	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat markdown file: %w", err)
	}

	lists, err := parse(file, stat.ModTime())
	if err != nil {
		return nil, fmt.Errorf("failed to parse markdown file: %w", err)
	}

	return lists, nil
}

// writeFile renders the provided lists to Markdown and atomically overwrites the file.
func (c *Client) writeFile(lists []model.List) error {
	file, err := os.OpenFile(c.filepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open markdown file for writing: %w", err)
	}

	defer file.Close()

	if err := render(file, lists); err != nil {
		return fmt.Errorf("failed to render markdown file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close markdown file: %w", err)
	}

	return nil
}
