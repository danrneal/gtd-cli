package app

import (
	"context"

	"github.com/danrneal/gtd.nvim/internal/model"
)

type Syncer struct {
	local  Provider
	remote Provider
}

func NewSyncer(local Provider, remote Provider) *Syncer {
	syncer := &Syncer{
		local:  local,
		remote: remote,
	}

	return syncer
}

func (s *Syncer) Sync(ctx context.Context) (bool, error) {
	updated, err := s.Push(ctx)
	if err != nil {
		return false, err
	}

	updated, err = s.Pull(ctx)
	if err != nil {
		return false, err
	}

	return updated, nil
}

func (s *Syncer) Push(ctx context.Context) (bool, error) {
	localLists, err := s.local.ListLists(ctx)
	if err != nil {
		return false, err
	}

	localListsByExtID := make(map[string]model.List)
	localItemsByExtID := make(map[string]model.Item)
	for _, localList := range localLists {
		if localList.ExternalID == nil {
			continue
		}

		localListsByExtID[*localList.ExternalID] = localList
		for _, localItem := range localList.Items {
			if localItem.ExternalID == nil {
				continue
			}

			localItemsByExtID[*localItem.ExternalID] = localItem
		}
	}

	remoteLists, err := s.remote.ListLists(ctx)
	if err != nil {
		return false, err
	}

	remoteListsByExtID := make(map[string]model.List)
	remoteItemsByExtID := make(map[string]model.Item)
	for _, remoteList := range remoteLists {
		remoteListsByExtID[*remoteList.ExternalID] = remoteList
		for _, remoteItem := range remoteList.Items {
			remoteItemsByExtID[*remoteItem.ExternalID] = remoteItem
		}
	}

	updated := false
	for _, remoteList := range remoteLists {
		/* Deletion logic deferred - model.List needs deleted/status field
		localList, ok := localListsByExtID[*remoteList.ExternalID]
		if ok && localList.deleted {
			if err := s.remote.DeleteList(ctx, *remoteList.ExternalID); err != nil {
				return false, err
			}

			if err := s.local.DeleteList(ctx, localList.ID); err != nil {
				return false, err
			}

			updated = true
			continue
		}
		*/

		for _, remoteItem := range remoteList.Items {
			localItem, ok := localItemsByExtID[*remoteItem.ExternalID]
			if ok && localItem.Status == model.StatusDeleted {
				if err := s.remote.DeleteItem(ctx, remoteItem); err != nil {
					return false, err
				}

				if err := s.local.DeleteItem(ctx, localItem); err != nil {
					return false, err
				}

				updated = true
			}
		}
	}

	for _, localList := range localLists {
		if localList.ExternalID == nil {
			extID, err := s.remote.CreateList(ctx, localList)
			if err != nil {
				return false, err
			}

			localList.ExternalID = &extID
			if err := s.local.UpdateList(ctx, localList, nil); err != nil {
				return false, err
			}

			updated = true
		}

		prevItemID := ""
		for _, localItem := range localList.Items {
			if localItem.ExternalID == nil {
				extID, err := s.remote.CreateItem(ctx, localItem, prevItemID)
				if err != nil {
					return false, err
				}

				localItem.ExternalID = &extID
				if err := s.local.UpdateItem(ctx, localItem); err != nil {
					return false, err
				}

				updated = true
			}

			prevItemID = *localItem.ExternalID
		}

		remoteList := remoteListsByExtID[*localList.ExternalID]
		if localList.Modified.After(remoteList.Modified) {
			if err := s.remote.UpdateList(ctx, localList, remoteList.Items); err != nil {
				return false, err
			}

			updated = true
		}

		for _, localItem := range localList.Items {
			remoteItem, ok := remoteItemsByExtID[*localItem.ExternalID]
			if ok && localItem.Modified.After(remoteItem.Modified) {
				if err := s.remote.UpdateItem(ctx, localItem); err != nil {
					return false, err
				}

				updated = true
			}
		}
	}

	return updated, nil
}

func (s *Syncer) Pull(ctx context.Context) (bool, error) {
	remoteLists, err := s.remote.ListLists(ctx)
	if err != nil {
		return false, err
	}

	remoteListsByExtID := make(map[string]model.List)
	remoteItemsByExtID := make(map[string]model.Item)
	for _, remoteList := range remoteLists {
		remoteListsByExtID[*remoteList.ExternalID] = remoteList
		for _, remoteItem := range remoteList.Items {
			remoteItemsByExtID[*remoteItem.ExternalID] = remoteItem
		}
	}

	localLists, err := s.local.ListLists(ctx)
	if err != nil {
		return false, err
	}

	localListsByExtID := make(map[string]model.List)
	localItemsByExtID := make(map[string]model.Item)
	for _, localList := range localLists {
		if localList.ExternalID == nil {
			continue
		}

		localListsByExtID[*localList.ExternalID] = localList
		for _, localItem := range localList.Items {
			if localItem.ExternalID == nil {
				continue
			}

			localItemsByExtID[*localItem.ExternalID] = localItem
		}
	}

	updated := false
	for _, localList := range localLists {
		if localList.ExternalID != nil {
			if _, ok := remoteListsByExtID[*localList.ExternalID]; !ok {
				if err := s.local.DeleteList(ctx, localList.ID); err != nil {
					return false, err
				}

				updated = true
				continue
			}
		}

		for _, localItem := range localList.Items {
			if localItem.ExternalID != nil {
				if _, ok := remoteItemsByExtID[*localItem.ExternalID]; !ok {
					if err := s.local.DeleteItem(ctx, localItem); err != nil {
						return false, err
					}

					updated = true
				}
			}
		}
	}

	for _, remoteList := range remoteLists {
		localList, ok := localListsByExtID[*remoteList.ExternalID]
		if !ok {
			localListID, err := s.local.CreateList(ctx, remoteList)
			if err != nil {
				return false, err
			}

			remoteList.ID = localListID
			updated = true
		} else {
			remoteList.ID = localList.ID
		}

		for _, remoteItem := range remoteList.Items {
			if _, ok := localItemsByExtID[*remoteItem.ExternalID]; !ok {
				remoteItem.ListID = remoteList.ID
				if _, err := s.local.CreateItem(ctx, remoteItem, ""); err != nil {
					return false, err
				}

				updated = true
			}
		}

		if ok && remoteList.Modified.After(localList.Modified) {
			if err := s.local.UpdateList(ctx, remoteList, localList.Items); err != nil {
				return false, err
			}

			updated = true
		}

		for _, remoteItem := range remoteList.Items {
			localItem, ok := localItemsByExtID[*remoteItem.ExternalID]
			if ok && remoteItem.Modified.After(localItem.Modified) {
				if err := s.local.UpdateItem(ctx, remoteItem); err != nil {
					return false, err
				}

				updated = true
			}
		}
	}

	return updated, nil
}
