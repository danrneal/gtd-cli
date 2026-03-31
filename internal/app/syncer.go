package app

import (
	"context"
	"fmt"

	"github.com/danrneal/gtd.nvim/internal/model"
)

// Syncer manages the synchronization of items and lists between a local provider and a remote provider.
type Syncer struct {
	local  Provider
	remote RemoteProvider
	getKey func(model.Resource) string
	setKey func(model.Resource, string)
}

// NewSyncer creates a new Syncer instance with the given local and remote providers.
func NewSyncer(local Provider, remote RemoteProvider) *Syncer {
	syncer := &Syncer{
		local:  local,
		remote: remote,
		getKey: remote.GetKey,
		setKey: remote.SetKey,
	}

	return syncer
}

// Sync performs a two-way synchronization, first pushing local changes to remote, then pulling remote changes to local.
// It returns true if any changes were pulled from the remote provider.
func (s *Syncer) Sync(ctx context.Context) (bool, error) {
	if _, err := s.Push(ctx); err != nil {
		return false, err
	}

	updated, err := s.Pull(ctx)
	if err != nil {
		return false, err
	}

	return updated, nil
}

// Push synchronizes changes from the local provider to the remote provider.
// It returns true if any changes were pushed.
func (s *Syncer) Push(ctx context.Context) (bool, error) {
	return s.oneWaySync(ctx, s.local, s.remote)
}

// Pull synchronizes changes from the remote provider to the local provider.
// It returns true if any changes were pulled.
func (s *Syncer) Pull(ctx context.Context) (bool, error) {
	return s.oneWaySync(ctx, s.remote, s.local)
}

// oneWaySync performs a unidirectional synchronization from the source provider to the destination provider.
// It handles creation, updates, and deletions of lists and items based on modification timestamps and status.
// It returns true if any changes were applied to the destination.
func (s *Syncer) oneWaySync(ctx context.Context, src, dst Provider) (bool, error) {
	srcLists, err := src.ListLists(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve lists from source provider: %w", err)
	}

	dstLists, err := dst.ListLists(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to retrieve lists from destination provider: %w", err)
	}

	srcListsMap, srcItemsMap := s.createResourceMaps(srcLists)
	dstListsMap, dstItemsMap := s.createResourceMaps(dstLists)

	updated := false
	for _, srcList := range srcLists {
		if srcList.Status == model.StatusDeleted {
			continue
		}

		isNewList := false
		listKey := s.getKey(&srcList)
		dstList, ok := dstListsMap[listKey]
		if !ok {
			dstListKey, err := dst.CreateList(ctx, srcList)
			if err != nil {
				return false, fmt.Errorf("failed to create list %q in destination: %w", srcList.Name, err)
			}

			if listKey == "" {
				s.setKey(&srcList, dstListKey)
				if err := src.UpdateList(ctx, srcList, srcList.Items); err != nil {
					return false, fmt.Errorf(
						"failed to backfill external key for list %q in source: %w",
						srcList.Name,
						err,
					)
				}
			}

			dstList = srcList
			isNewList = true
			updated = true
		}

		prevItemID := ""
		for i, srcItem := range srcList.Items {
			if srcItem.Status == model.StatusDeleted {
				continue
			}

			itemKey := s.getKey(&srcItem)
			if _, ok := dstItemsMap[itemKey]; !ok {
				srcItem.ListID = srcList.ID
				srcItem.ExternalListID = srcList.ExternalID
				dstItemKey, err := dst.CreateItem(ctx, srcItem, prevItemID)
				if err != nil {
					return false, fmt.Errorf("failed to create item %q in destination: %w", srcItem.Title, err)
				}

				if itemKey == "" {
					s.setKey(&srcItem, dstItemKey)
					if err := src.UpdateItem(ctx, srcItem); err != nil {
						return false, fmt.Errorf(
							"failed to backfill external key for item %q in source: %w",
							srcItem.Title,
							err,
						)
					}
				}

				srcList.Items[i] = srcItem
				updated = true
			}

			prevItemID = itemKey
		}

		if isNewList || srcList.Modified.After(dstList.Modified) {
			if err := dst.UpdateList(ctx, srcList, dstList.Items); err != nil {
				return false, fmt.Errorf("failed to update list %q in destination: %w", srcList.Name, err)
			}

			updated = true
		}

		for _, srcItem := range srcList.Items {
			if srcItem.Status == model.StatusDeleted {
				continue
			}

			itemKey := s.getKey(&srcItem)
			dstItem, ok := dstItemsMap[itemKey]
			//nolint:revive // Complex logic pending refactor
			if ok && srcItem.Modified.After(dstItem.Modified) {
				if srcItem.Status == model.StatusNotStarted && dstItem.Status == model.StatusInProgress {
					srcItem.Status = model.StatusInProgress
				}

				srcItem.ListID = srcList.ID
				if err := dst.UpdateItem(ctx, srcItem); err != nil {
					return false, fmt.Errorf("failed to update item %q in destination: %w", srcItem.Title, err)
				}

				updated = true
			}
		}
	}

	for _, dstList := range dstLists {
		if dstList.Status == model.StatusDeleted {
			continue
		}

		listKey := s.getKey(&dstList)
		if listKey == "" {
			continue
		}

		if srcList, ok := srcListsMap[listKey]; !ok {
			dstList.Status = model.StatusDeleted
			if err := dst.UpdateList(ctx, dstList, dstList.Items); err != nil {
				return false, fmt.Errorf("failed to mark list %q as deleted in destination: %w", dstList.Name, err)
			}

			updated = true
			continue
		} else if srcList.Status == model.StatusDeleted {
			if err := dst.DeleteList(ctx, dstList); err != nil {
				return false, fmt.Errorf("failed to permanently delete list %q from destination: %w", dstList.Name, err)
			}

			if err := src.DeleteList(ctx, srcList); err != nil {
				return false, fmt.Errorf("failed to permanently delete list %q from source: %w", srcList.Name, err)
			}

			updated = true
			continue
		}

		for _, dstItem := range dstList.Items {
			if dstItem.Status == model.StatusDeleted {
				continue
			}

			itemKey := s.getKey(&dstItem)
			if itemKey == "" {
				continue
			}

			if srcItem, ok := srcItemsMap[itemKey]; !ok {
				dstItem.Status = model.StatusDeleted
				if err := dst.UpdateItem(ctx, dstItem); err != nil {
					return false, fmt.Errorf("failed to mark item %q as deleted in destination: %w", dstItem.Title, err)
				}

				updated = true
			} else if srcItem.Status == model.StatusDeleted {
				if err := dst.DeleteItem(ctx, dstItem); err != nil {
					return false, fmt.Errorf(
						"failed to permanently delete item %q from destination: %w",
						dstItem.Title,
						err,
					)
				}

				if err := src.DeleteItem(ctx, srcItem); err != nil {
					return false, fmt.Errorf("failed to permanently delete item %q from source: %w", srcItem.Title, err)
				}

				updated = true
			}
		}
	}

	return updated, nil
}

// createResourceMaps creates lookup maps for lists and items from a slice of lists, keyed by their sync identifier.
func (s *Syncer) createResourceMaps(lists []model.List) (map[string]model.List, map[string]model.Item) {
	listsMap := make(map[string]model.List)
	itemsMap := make(map[string]model.Item)
	for _, list := range lists {
		for _, item := range list.Items {
			itemKey := s.getKey(&item)
			if itemKey == "" {
				continue
			}

			itemsMap[itemKey] = item
		}

		listKey := s.getKey(&list)
		if listKey == "" {
			continue
		}

		listsMap[listKey] = list
	}

	return listsMap, itemsMap
}
