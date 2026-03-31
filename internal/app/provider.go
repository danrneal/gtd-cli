// Package app defines the core interfaces for the gtd application.
package app

import (
	"context"

	"github.com/danrneal/gtd.nvim/internal/model"
)

// Provider defines the interface for task management systems (e.g. SQLite, Google Tasks, Neovim)
// to interact with the core application logic.
// It acts as an abstraction layer for task persistence and synchronization services.
type Provider interface {
	CreateList(ctx context.Context, list *model.List) (string, error)
	ListLists(ctx context.Context) ([]model.List, error)
	UpdateList(ctx context.Context, list *model.List, currentItems []*model.Item) error
	DeleteList(ctx context.Context, list *model.List) error

	CreateItem(ctx context.Context, item *model.Item, previousItemID string) (string, error)
	UpdateItem(ctx context.Context, item *model.Item) error
	DeleteItem(ctx context.Context, item *model.Item) error
}

// RemoteProvider extends Provider with capabilities for managing external keys (Get/Set),
// typically required for synchronizing with external services.
type RemoteProvider interface {
	Provider
	GetKey(resource model.Resource) string
	SetKey(resource model.Resource, key string)
}
