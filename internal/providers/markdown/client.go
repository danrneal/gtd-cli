package markdown

import (
	"context"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/providers/common"
)

type Client struct {
	filepath string
}

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

func (c *Client) CreateList(ctx context.Context, list model.List) (string, error) {
	// we might need to itroduce a previousListID param
	return "", nil
}

func (c *Client) ListLists(ctx context.Context) ([]model.List, error) {
	return nil, nil
}

func (c *Client) UpdateList(ctx context.Context, list model.List, currentItems []model.Item) error {
	return nil
}

func (c *Client) DeleteList(ctx context.Context, list model.List) error {
	return nil
}

func (c *Client) CreateItem(ctx context.Context, item model.Item, previousItemID string) (string, error) {
	return "", nil
}

//nolint:unused // TODO: implement later
func (c *Client) listItems(ctx context.Context, list model.List) ([]model.Item, error) {
	// Not sure we need this func
	return nil, nil
}

func (c *Client) UpdateItem(ctx context.Context, item model.Item) error {
	return nil
}

//nolint:unused // TODO: implement later
func (c *Client) moveItem(ctx context.Context, move common.Move) error {
	return nil
}

func (c *Client) DeleteItem(ctx context.Context, item model.Item) error {
	return nil
}
