package app

import (
	"context"

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
		return false, err
	}

	dstLists, err := dst.ListLists(ctx)
	if err != nil {
		return false, err
	}

	srcListsMap, srcItemsMap := s.createResourceMaps(srcLists)
	dstListsMap, dstItemsMap := s.createResourceMaps(dstLists)

	updated := false
	for _, srcList := range srcLists {
		if srcList.Status == model.StatusDeleted {
			continue
		}

		listKey := s.getKey(&srcList)
		dstList, ok := dstListsMap[listKey]
		if !ok {
			dstListKey, err := dst.CreateList(ctx, srcList)
			if err != nil {
				return false, err
			}

			if listKey == "" {
				s.setKey(&srcList, dstListKey)
				if err := src.UpdateList(ctx, srcList, srcList.Items); err != nil {
					return false, err
				}
			}

			dstList = srcList
			dstList.Modified = dstList.Modified.Add(-1)
			updated = true
		}

		prevItemID := ""
		for i, srcItem := range srcList.Items {
			if srcItem.Status == model.StatusDeleted {
				continue
			}

			itemKey := s.getKey(&srcItem)
			if _, ok := dstItemsMap[itemKey]; !ok {
				if srcItem.Status == model.StatusOpen {
					srcItem.Status = model.StatusNotStarted
				}

				srcItem.ListID = srcList.ID
				srcItem.ExternalListID = srcList.ExternalID
				dstItemKey, err := dst.CreateItem(ctx, srcItem, prevItemID)
				if err != nil {
					return false, err
				}

				if itemKey == "" {
					s.setKey(&srcItem, dstItemKey)
					if err := src.UpdateItem(ctx, srcItem); err != nil {
						return false, err
					}
				}

				srcList.Items[i] = srcItem
				updated = true
			}

			prevItemID = itemKey
		}

		if srcList.Modified.After(dstList.Modified) {
			if err := dst.UpdateList(ctx, srcList, dstList.Items); err != nil {
				return false, err
			}

			updated = true
		}

		for _, srcItem := range srcList.Items {
			if srcItem.Status == model.StatusDeleted {
				continue
			}

			itemKey := s.getKey(&srcItem)
			dstItem, ok := dstItemsMap[itemKey]
			if ok && srcItem.Modified.After(dstItem.Modified) {
				if srcItem.Status == model.StatusOpen {
					srcItem.Status = model.StatusNotStarted
					if dstItem.Status == model.StatusInProgress {
						srcItem.Status = model.StatusInProgress
					}
				}

				srcItem.ListID = srcList.ID
				if err := dst.UpdateItem(ctx, srcItem); err != nil {
					return false, err
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
				return false, err
			}

			updated = true
			continue
		} else if srcList.Status == model.StatusDeleted {
			if err := dst.DeleteList(ctx, dstList); err != nil {
				return false, err
			}

			if err := src.DeleteList(ctx, srcList); err != nil {
				return false, err
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
					return false, err
				}

				updated = true
			} else if srcItem.Status == model.StatusDeleted {
				if err := dst.DeleteItem(ctx, dstItem); err != nil {
					return false, err
				}

				if err := src.DeleteItem(ctx, srcItem); err != nil {
					return false, err
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
