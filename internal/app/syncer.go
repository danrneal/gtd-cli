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
}

// NewSyncer creates a new Syncer instance with the given local and remote providers.
func NewSyncer(local Provider, remote RemoteProvider) *Syncer {
	syncer := &Syncer{
		local:  local,
		remote: remote,
		getKey: remote.GetKey,
	}

	return syncer
}

// Sync performs a two-way synchronization, first pushing local changes to remote, then pulling remote changes to local.
// It returns true if any changes were pulled from the remote provider.
func (s *Syncer) Sync(ctx context.Context) (bool, error) {
	if _, err := s.Push(ctx); err != nil {
		return false, err
	}

	changed, err := s.Pull(ctx)
	if err != nil {
		return false, err
	}

	return changed, nil
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
	srcCache, err := s.buildResourceCache(ctx, src)
	if err != nil {
		return false, err
	}

	dstCache, err := s.buildResourceCache(ctx, dst)
	if err != nil {
		return false, err
	}

	changed := false
	for _, srcList := range srcCache.lists {
		srcListChanged, err := s.syncSrcList(ctx, src, dst, &srcList, dstCache)
		if err != nil {
			return false, err
		}

		changed = changed || srcListChanged
	}

	for _, dstList := range dstCache.lists {
		pruned, err := s.pruneDstList(ctx, src, dst, &dstList, srcCache)
		if err != nil {
			return false, err
		}

		changed = changed || pruned
	}

	return changed, nil
}

// resourceCache holds a set of Resources from a Provider.
type resourceCache struct {
	lists    []model.List
	listsMap map[string]*model.List
	itemsMap map[string]*model.Item
}

// buildResourceCache retrieves all lists and items from a provider and builds lookup maps.
func (s *Syncer) buildResourceCache(ctx context.Context, p Provider) (*resourceCache, error) {
	lists, err := p.ListLists(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve lists from provider: %w", err)
	}

	listsMap := make(map[string]*model.List, len(lists))
	itemsMap := make(map[string]*model.Item)
	for _, list := range lists {
		for _, item := range list.Items {
			itemKey := s.getKey(item)
			if itemKey == "" {
				continue
			}

			itemsMap[itemKey] = item
		}

		listKey := s.getKey(&list)
		if listKey == "" {
			continue
		}

		listsMap[listKey] = &list
	}

	cache := &resourceCache{
		lists:    lists,
		listsMap: listsMap,
		itemsMap: itemsMap,
	}

	return cache, nil
}

func (s *Syncer) syncSrcList(ctx context.Context, src, dst Provider, srcList *model.List, dstCache *resourceCache) (bool, error) {
	changed := false
	if srcList.Status == model.StatusDeleted {
		return changed, nil
	}

	listKey := s.getKey(srcList)
	dstList, dstListOk := dstCache.listsMap[listKey]
	if !dstListOk {
		if err := s.createList(ctx, src, dst, srcList); err != nil {
			return false, err
		}

		dstList = srcList
		changed = true
	}

	prevItemID := ""
	for _, srcItem := range srcList.Items {
		if srcItem.Status == model.StatusDeleted {
			continue
		}

		srcItem.ListID = srcList.ID
		srcItem.ExternalListID = srcList.ExternalID

		itemKey := s.getKey(srcItem)
		dstItem, dstItemOk := dstCache.itemsMap[itemKey]
		if !dstItemOk {
			if err := s.createItem(ctx, src, dst, srcItem, prevItemID); err != nil {
				return false, err
			}

			changed = true
		} else if srcItem.Modified.After(dstItem.Modified) {
			if err := s.updateItem(ctx, dst, srcItem, dstItem); err != nil {
				return false, err
			}

			changed = true
		}

		prevItemID = itemKey
	}

	if !dstListOk || srcList.Modified.After(dstList.Modified) {
		if err := s.updateList(ctx, dst, srcList, dstList.Items); err != nil {
			return false, err
		}

		changed = true
	}

	return changed, nil
}

func (s *Syncer) pruneDstList(ctx context.Context, src, dst Provider, dstList *model.List, srcCache *resourceCache) (bool, error) {
	pruned := false
	if dstList.Status == model.StatusDeleted {
		return pruned, nil
	}

	listKey := s.getKey(dstList)
	if listKey == "" {
		return pruned, nil
	}

	srcList, ok := srcCache.listsMap[listKey]
	if !ok || srcList.Status == model.StatusDeleted {
		if err := s.deleteList(ctx, src, dst, srcList, dstList); err != nil {
			return false, err
		}

		pruned = true

		return pruned, nil
	}

	for _, dstItem := range dstList.Items {
		if dstItem.Status == model.StatusDeleted {
			continue
		}

		itemKey := s.getKey(dstItem)
		if itemKey == "" {
			continue
		}

		srcItem, ok := srcCache.itemsMap[itemKey]
		if !ok || srcItem.Status == model.StatusDeleted {
			if err := s.deleteItem(ctx, src, dst, srcItem, dstItem); err != nil {
				return false, err
			}

			pruned = true
		}
	}

	return pruned, nil
}

func (s *Syncer) createList(ctx context.Context, src, dst Provider, list *model.List) error {
	listKey := s.getKey(list)
	if err := dst.CreateList(ctx, list); err != nil {
		return fmt.Errorf("failed to create list %q in destination: %w", list.Name, err)
	}

	if listKey == "" {
		if err := src.UpdateList(ctx, list, list.Items); err != nil {
			return fmt.Errorf("failed to backfill external key for list %q in source: %w", list.Name, err)
		}
	}

	return nil
}

func (s *Syncer) createItem(ctx context.Context, src, dst Provider, item *model.Item, prevItemID string) error {
	itemKey := s.getKey(item)
	if err := dst.CreateItem(ctx, item, prevItemID); err != nil {
		return fmt.Errorf("failed to create item %q in destination: %w", item.Title, err)
	}

	if itemKey == "" {
		if err := src.UpdateItem(ctx, item); err != nil {
			return fmt.Errorf("failed to backfill external key for item %q in source: %w", item.Title, err)
		}
	}

	return nil
}

func (s *Syncer) updateItem(ctx context.Context, dst Provider, srcItem, dstItem *model.Item) error {
	if srcItem.Status == model.StatusNotStarted && dstItem.Status == model.StatusInProgress {
		srcItem.Status = model.StatusInProgress
	}

	if err := dst.UpdateItem(ctx, srcItem); err != nil {
		return fmt.Errorf("failed to update item %q in destination: %w", srcItem.Title, err)
	}

	return nil
}

func (s *Syncer) updateList(ctx context.Context, dst Provider, list *model.List, currentItems []*model.Item) error {
	if err := dst.UpdateList(ctx, list, currentItems); err != nil {
		return fmt.Errorf("failed to update list %q in destination: %w", list.Name, err)
	}

	return nil
}

func (s *Syncer) deleteList(ctx context.Context, src, dst Provider, srcList, dstList *model.List) error {
	if srcList == nil {
		dstList.Status = model.StatusDeleted
		if err := dst.UpdateList(ctx, dstList, dstList.Items); err != nil {
			return fmt.Errorf("failed to mark list %q as deleted in destination: %w", dstList.Name, err)
		}

		return nil
	}

	if err := dst.DeleteList(ctx, dstList); err != nil {
		return fmt.Errorf("failed to permanently delete list %q from destination: %w", dstList.Name, err)
	}

	if err := src.DeleteList(ctx, srcList); err != nil {
		return fmt.Errorf("failed to permanently delete list %q from source: %w", srcList.Name, err)
	}

	return nil
}

func (s *Syncer) deleteItem(ctx context.Context, src, dst Provider, srcItem, dstItem *model.Item) error {
	if srcItem == nil {
		dstItem.Status = model.StatusDeleted
		if err := dst.UpdateItem(ctx, dstItem); err != nil {
			return fmt.Errorf("failed to mark item %q as deleted in destination: %w", dstItem.Title, err)
		}

		return nil
	}

	if err := dst.DeleteItem(ctx, dstItem); err != nil {
		return fmt.Errorf("failed to permanently delete item %q from destination: %w", dstItem.Title, err)
	}

	if err := src.DeleteItem(ctx, srcItem); err != nil {
		return fmt.Errorf("failed to permanently delete item %q from source: %w", srcItem.Title, err)
	}

	return nil
}
