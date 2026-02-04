package app

import (
	"context"

	"github.com/danrneal/gtd.nvim/internal/model"
)

// Provider defines the interface for task management systems (e.g. SQLite, Google Tasks, Neovim)
// to interact with the core application logic.
// It acts as an abstraction layer for task persistence and synchronization services.
type Provider interface {
	CreateList(ctx context.Context, list model.List) (model.List, error)
	ListLists(ctx context.Context) ([]model.List, error)
	UpdateList(ctx context.Context, list model.List, currentItems []model.Item) error
	DeleteList(ctx context.Context, listID string) error

	CreateItem(ctx context.Context, item model.Item, previousItemID string) (model.Item, error)
	UpdateItem(ctx context.Context, item model.Item) error
	DeleteItem(ctx context.Context, item model.Item) error
}

// RemoteProvider extends Provider with the ability to generate/extract external keys.
// This is typically implemented by external services like Google Tasks.
type RemoteProvider interface {
	Provider
	GetKey(resource model.Resource) string
}
