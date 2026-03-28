// Package markdown implements the markdown file provider.
package markdown

import (
	"context"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/providers/util/move"
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

// SetKey sets the key on the resource (in-memory).
func (c *Client) SetKey(resource model.Resource, key string) {
	resource.SetID(key)
}

// CreateList creates a new list in the markdown file.
func (c *Client) CreateList(_ context.Context, _ model.List) (string, error) {
	// we might need to itroduce a previousListID param
	return "", nil
}

// ListLists retrieves all lists from the markdown file.
func (c *Client) ListLists(_ context.Context) ([]model.List, error) {
	return nil, nil
}

// UpdateList updates an existing list and its items in the markdown file.
func (c *Client) UpdateList(_ context.Context, _ model.List, _ []model.Item) error {
	return nil
}

// DeleteList removes a list from the markdown file.
func (c *Client) DeleteList(_ context.Context, _ model.List) error {
	return nil
}

// CreateItem adds a new item to a list in the markdown file.
func (c *Client) CreateItem(_ context.Context, _ model.Item, _ string) (string, error) {
	return "", nil
}

//nolint:unused // TODO: implement later
func (c *Client) listItems(_ context.Context, _ model.List) ([]model.Item, error) {
	// Not sure we need this func
	return nil, nil
}

// UpdateItem updates an existing item in the markdown file.
func (c *Client) UpdateItem(_ context.Context, _ model.Item) error {
	return nil
}

//nolint:unused // TODO: implement later
func (c *Client) moveItem(_ context.Context, _ move.Move) error {
	return nil
}

// DeleteItem removes an item from the markdown file.
func (c *Client) DeleteItem(_ context.Context, _ model.Item) error {
	return nil
}
