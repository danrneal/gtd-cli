package app

import (
	"context"

	"github.com/danrneal/gtd-cli/model"
)

// errorProvider wraps a Provider to inject errors for testing resilience.
type errorProvider struct {
	Provider

	errCreateList error
	errListLists  error
	errUpdateList error
	errDeleteList error
	errCreateItem error
	errUpdateItem error
	errDeleteItem error
}

func (e *errorProvider) GetKey(resource model.Resource) string {
	rp, ok := e.Provider.(RemoteProvider)
	if !ok {
		return ""
	}

	return rp.GetKey(resource)
}

func (e *errorProvider) CreateList(ctx context.Context, list *model.List) error {
	if e.errCreateList != nil {
		err := e.errCreateList
		e.errCreateList = nil

		return err
	}

	return e.Provider.CreateList(ctx, list)
}

func (e *errorProvider) ListLists(ctx context.Context) ([]model.List, error) {
	if e.errListLists != nil {
		err := e.errListLists
		e.errListLists = nil

		return nil, err
	}

	return e.Provider.ListLists(ctx)
}

func (e *errorProvider) UpdateList(ctx context.Context, list, currentList *model.List) error {
	if e.errUpdateList != nil {
		err := e.errUpdateList
		e.errUpdateList = nil

		return err
	}

	return e.Provider.UpdateList(ctx, list, currentList)
}

func (e *errorProvider) DeleteList(ctx context.Context, list *model.List) error {
	if e.errDeleteList != nil {
		err := e.errDeleteList
		e.errDeleteList = nil

		return err
	}

	return e.Provider.DeleteList(ctx, list)
}

func (e *errorProvider) CreateItem(ctx context.Context, item *model.Item, previousItemID string) error {
	if e.errCreateItem != nil {
		err := e.errCreateItem
		e.errCreateItem = nil

		return err
	}

	return e.Provider.CreateItem(ctx, item, previousItemID)
}

func (e *errorProvider) UpdateItem(ctx context.Context, item *model.Item) error {
	if e.errUpdateItem != nil {
		err := e.errUpdateItem
		e.errUpdateItem = nil

		return err
	}

	return e.Provider.UpdateItem(ctx, item)
}

func (e *errorProvider) DeleteItem(ctx context.Context, item *model.Item) error {
	if e.errDeleteItem != nil {
		err := e.errDeleteItem
		e.errDeleteItem = nil

		return err
	}

	return e.Provider.DeleteItem(ctx, item)
}
