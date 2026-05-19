package app

import (
	"context"

	"github.com/danrneal/gtd.nvim/internal/model"
)

// FakeRemoteProvider wraps a RemoteProvider to inject errors for testing resilience.
type FakeRemoteProvider struct {
	RemoteProvider

	ErrNextRead  error
	ErrNextWrite error
}

func (f *FakeRemoteProvider) CreateList(ctx context.Context, list *model.List) error {
	if f.ErrNextWrite != nil {
		err := f.ErrNextWrite
		f.ErrNextWrite = nil

		return err
	}

	return f.RemoteProvider.CreateList(ctx, list)
}

func (f *FakeRemoteProvider) ListLists(ctx context.Context) ([]model.List, error) {
	if f.ErrNextRead != nil {
		err := f.ErrNextRead
		f.ErrNextRead = nil

		return nil, err
	}

	return f.RemoteProvider.ListLists(ctx)
}

func (f *FakeRemoteProvider) UpdateList(ctx context.Context, list, currentList *model.List) error {
	if f.ErrNextWrite != nil {
		err := f.ErrNextWrite
		f.ErrNextWrite = nil

		return err
	}

	return f.RemoteProvider.UpdateList(ctx, list, currentList)
}

func (f *FakeRemoteProvider) DeleteList(ctx context.Context, list *model.List) error {
	if f.ErrNextWrite != nil {
		err := f.ErrNextWrite
		f.ErrNextWrite = nil

		return err
	}

	return f.RemoteProvider.DeleteList(ctx, list)
}

func (f *FakeRemoteProvider) CreateItem(ctx context.Context, item *model.Item, previousItemID string) error {
	if f.ErrNextWrite != nil {
		err := f.ErrNextWrite
		f.ErrNextWrite = nil

		return err
	}

	return f.RemoteProvider.CreateItem(ctx, item, previousItemID)
}

func (f *FakeRemoteProvider) UpdateItem(ctx context.Context, item *model.Item) error {
	if f.ErrNextWrite != nil {
		err := f.ErrNextWrite
		f.ErrNextWrite = nil

		return err
	}

	return f.RemoteProvider.UpdateItem(ctx, item)
}

func (f *FakeRemoteProvider) DeleteItem(ctx context.Context, item *model.Item) error {
	if f.ErrNextWrite != nil {
		err := f.ErrNextWrite
		f.ErrNextWrite = nil

		return err
	}

	return f.RemoteProvider.DeleteItem(ctx, item)
}
