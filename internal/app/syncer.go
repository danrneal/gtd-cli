package app

import (
	"context"
	"fmt"
	"slices"

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

// Push synchronizes changes from the local provider to the remote provider.
// It returns true if any changes were pushed.
func (s *Syncer) Push(ctx context.Context) error {
	_, err := s.oneWaySync(ctx, s.local, s.remote)
	return err
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
	srcState, err := s.buildProviderState(ctx, src)
	if err != nil {
		return false, err
	}

	dstState, err := s.buildProviderState(ctx, dst)
	if err != nil {
		return false, err
	}

	ss := &syncSession{
		getKey:   s.getKey,
		srcState: srcState,
		dstState: dstState,
	}

	changed := false
	for _, srcList := range srcState.lists {
		srcListChanged, err := ss.syncList(ctx, &srcList)
		changed = changed || srcListChanged
		if err != nil {
			return changed, err
		}
	}

	for _, dstList := range dstState.lists {
		pruned, err := ss.pruneList(ctx, &dstList)
		changed = changed || pruned
		if err != nil {
			return changed, err
		}
	}

	return changed, nil
}

// buildProviderState retrieves all lists and items from a provider and builds lookup maps.
func (s *Syncer) buildProviderState(ctx context.Context, p Provider) (*providerState, error) {
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

	state := &providerState{
		provider: p,
		lists:    lists,
		listsMap: listsMap,
		itemsMap: itemsMap,
	}

	return state, nil
}

// providerState holds a set of Resources from a Provider and the provider itself.
type providerState struct {
	provider Provider
	lists    []model.List
	listsMap map[string]*model.List
	itemsMap map[string]*model.Item
}

// syncSession encapsulates the state required for a single one-way synchronization pass.
type syncSession struct {
	getKey   func(model.Resource) string
	srcState *providerState
	dstState *providerState
}

// syncList processes a single list from the source provider state, creating or updating it and its items
// in the destination provider state as needed. It returns true if any changes were applied.
func (ss *syncSession) syncList(ctx context.Context, srcList *model.List) (bool, error) {
	if srcList.Status == model.StatusDeleted {
		return false, nil
	}

	changed := false
	listKey := ss.getKey(srcList)
	dstList, ok := ss.dstState.listsMap[listKey]
	if !ok {
		if err := ss.createList(ctx, srcList); err != nil {
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
		itemChanged, err := ss.syncItem(ctx, srcItem, prevItemID)
		changed = changed || itemChanged
		if err != nil {
			return changed, err
		}

		prevItemID = ss.getKey(srcItem)
	}

	if !ok || (srcList.Modified.After(dstList.Modified) && !srcList.Equal(dstList)) {
		if err := ss.updateList(ctx, srcList, dstList.Items); err != nil {
			return changed, err
		}

		changed = true
	}

	return changed, nil
}

// syncItem processes a single item from the source provider state, creating or updating it
// in the destination provider state as needed. It returns true if any changes were applied.
func (ss *syncSession) syncItem(ctx context.Context, srcItem *model.Item, prevItemID string) (bool, error) {
	itemKey := ss.getKey(srcItem)
	dstItem, ok := ss.dstState.itemsMap[itemKey]
	if !ok {
		if err := ss.createItem(ctx, srcItem, prevItemID); err != nil {
			return false, err
		}

		return true, nil
	}

	if srcItem.Modified.After(dstItem.Modified) && !srcItem.Equal(dstItem) {
		if err := ss.updateItem(ctx, srcItem, dstItem); err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

// pruneList processes a single list from the destination provider state, deleting it or its items
// if they no longer exist in the source provider state or have been marked as deleted.
// It returns true if any changes were applied.
func (ss *syncSession) pruneList(ctx context.Context, dstList *model.List) (bool, error) {
	if dstList.Status == model.StatusDeleted {
		return false, nil
	}

	listKey := ss.getKey(dstList)
	if listKey == "" {
		return false, nil
	}

	srcList, ok := ss.srcState.listsMap[listKey]
	if !ok || srcList.Status == model.StatusDeleted {
		if err := ss.deleteList(ctx, srcList, dstList); err != nil {
			return false, err
		}

		return true, nil
	}

	pruned := false
	for _, dstItem := range dstList.Items {
		itemPruned, err := ss.pruneItem(ctx, dstItem)
		pruned = pruned || itemPruned
		if err != nil {
			return pruned, err
		}
	}

	return pruned, nil
}

// pruneItem processes a single item from the destination provider state, deleting it
// if it no longer exists in the source provider state or has been marked as deleted.
// It returns true if any changes were applied.
func (ss *syncSession) pruneItem(ctx context.Context, dstItem *model.Item) (bool, error) {
	if dstItem.Status == model.StatusDeleted {
		return false, nil
	}

	itemKey := ss.getKey(dstItem)
	if itemKey == "" {
		return false, nil
	}

	srcItem, ok := ss.srcState.itemsMap[itemKey]
	if !ok || srcItem.Status == model.StatusDeleted {
		if err := ss.deleteItem(ctx, srcItem, dstItem); err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

// createList creates a new list in the destination provider and backfills the external ID
// into the source provider if necessary.
func (ss *syncSession) createList(ctx context.Context, list *model.List) error {
	listKey := ss.getKey(list)
	if err := ss.dstState.provider.CreateList(ctx, list); err != nil {
		return fmt.Errorf("failed to create list %q in destination: %w", list.Name, err)
	}

	if listKey == "" {
		if err := ss.srcState.provider.UpdateList(ctx, list, list.Items); err != nil {
			return fmt.Errorf("failed to backfill external key for list %q in source: %w", list.Name, err)
		}
	}

	return nil
}

// createItem creates a new item in the destination provider and backfills the external ID
// into the source provider if necessary.
func (ss *syncSession) createItem(ctx context.Context, item *model.Item, prevItemID string) error {
	itemKey := ss.getKey(item)
	if err := ss.dstState.provider.CreateItem(ctx, item, prevItemID); err != nil {
		return fmt.Errorf("failed to create item %q in destination: %w", item.Title, err)
	}

	if itemKey == "" {
		if err := ss.srcState.provider.UpdateItem(ctx, item); err != nil {
			return fmt.Errorf("failed to backfill external key for item %q in source: %w", item.Title, err)
		}
	}

	return nil
}

// updateItem updates an existing item in the destination provider, taking care to preserve specific status transitions.
func (ss *syncSession) updateItem(ctx context.Context, srcItem, dstItem *model.Item) error {
	if srcItem.Status == model.StatusNotStarted && dstItem.Status == model.StatusInProgress {
		srcItem.Status = model.StatusInProgress
	}

	if err := ss.dstState.provider.UpdateItem(ctx, srcItem); err != nil {
		return fmt.Errorf("failed to update item %q in destination: %w", srcItem.Title, err)
	}

	return nil
}

// updateList updates an existing list in the destination provider, maintaining its position and metadata.
func (ss *syncSession) updateList(ctx context.Context, list *model.List, currentItems []*model.Item) error {
	syncList := *list
	listItems := slices.Clone(syncList.Items)
	listItems = slices.DeleteFunc(listItems, func(i *model.Item) bool {
		return i.Status == model.StatusDeleted
	})

	syncList.Items = listItems
	if err := ss.dstState.provider.UpdateList(ctx, &syncList, currentItems); err != nil {
		return fmt.Errorf("failed to update list %q in destination: %w", syncList.Name, err)
	}

	return nil
}

// deleteList handles the deletion of a list. It permanently deletes the list if it exists in the source as deleted,
// or marks it deleted in the destination if it is missing from the source.
func (ss *syncSession) deleteList(ctx context.Context, srcList, dstList *model.List) error {
	if srcList == nil {
		dstList.Status = model.StatusDeleted
		if err := ss.dstState.provider.UpdateList(ctx, dstList, dstList.Items); err != nil {
			return fmt.Errorf("failed to mark list %q as deleted in destination: %w", dstList.Name, err)
		}

		return nil
	}

	if err := ss.dstState.provider.DeleteList(ctx, dstList); err != nil {
		return fmt.Errorf("failed to permanently delete list %q from destination: %w", dstList.Name, err)
	}

	if err := ss.srcState.provider.DeleteList(ctx, srcList); err != nil {
		return fmt.Errorf("failed to permanently delete list %q from source: %w", srcList.Name, err)
	}

	return nil
}

// deleteItem handles the deletion of an item. It permanently deletes the item if it exists in the source as deleted,
// or marks it deleted in the destination if it is missing from the source.
func (ss *syncSession) deleteItem(ctx context.Context, srcItem, dstItem *model.Item) error {
	if srcItem == nil {
		dstItem.Status = model.StatusDeleted
		if err := ss.dstState.provider.UpdateItem(ctx, dstItem); err != nil {
			return fmt.Errorf("failed to mark item %q as deleted in destination: %w", dstItem.Title, err)
		}

		return nil
	}

	if err := ss.dstState.provider.DeleteItem(ctx, dstItem); err != nil {
		return fmt.Errorf("failed to permanently delete item %q from destination: %w", dstItem.Title, err)
	}

	if err := ss.srcState.provider.DeleteItem(ctx, srcItem); err != nil {
		return fmt.Errorf("failed to permanently delete item %q from source: %w", srcItem.Title, err)
	}

	return nil
}
