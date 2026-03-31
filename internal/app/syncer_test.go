package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/danrneal/gtd.nvim/internal/model"
)

// FakeProvider implements RemoteProvider for testing.
type FakeProvider struct {
	Name        string
	Lists       []model.List
	ListCounter int
	ItemCounter int
}

func NewFakeProvider(name string, lists []model.List) *FakeProvider {
	provider := &FakeProvider{
		Name:  name,
		Lists: []model.List{},
	}

	for i, list := range lists {
		provider.ListCounter++
		listID := fmt.Sprintf("%s-list-%d", name, provider.ListCounter)
		if list.ID == "" && name == "store" {
			list.ID = listID
		} else if list.ExternalID == nil && name == "external" {
			list.ExternalID = &listID
		}

		if list.Status == "" {
			list.Status = model.StatusOpen
		}

		list.Position = i
		for j, item := range list.Items {
			provider.ItemCounter++
			itemID := fmt.Sprintf("%s-item-%d", name, provider.ItemCounter)
			if item.ID == "" && name == "store" {
				item.ID = itemID
			} else if item.ExternalID == nil && name == "external" {
				item.ExternalID = &itemID
			}

			item.Position = j
			item.ListID = list.ID
			item.ExternalListID = list.ExternalID
			list.Items[j] = item
		}

		provider.Lists = append(provider.Lists, list)
	}

	return provider
}

func (f *FakeProvider) GetKey(resource model.Resource) string {
	if f.Name != "external" {
		return resource.GetID()
	}

	if extID := resource.GetExternalID(); extID != nil {
		return *extID
	}

	return ""
}

func (f *FakeProvider) SetKey(resource model.Resource, key string) {
	if f.Name != "external" {
		resource.SetID(key)
	} else {
		resource.SetExternalID(key)
	}
}

func (f *FakeProvider) CreateList(_ context.Context, list *model.List) (string, error) {
	if list.Status != "" && list.Status != model.StatusOpen {
		return "", errors.New("new lists must have status 'open'")
	}

	if list.Name == "" {
		return "", errors.New("list name cannot be empty")
	}

	listKey := f.GetKey(list)
	if listKey == "" {
		f.ListCounter++
		listKey = fmt.Sprintf("%s-list-%d", f.Name, f.ListCounter)
		f.SetKey(list, listKey)
	}

	createdList := *list
	if f.Name == "external" {
		createdList.ID = ""
	}

	createdList.Status = model.StatusOpen
	createdList.Items = []model.Item{}
	f.Lists = append(f.Lists, createdList)

	return listKey, nil
}

func (f *FakeProvider) ListLists(_ context.Context) ([]model.List, error) {
	lists := make([]model.List, 0, len(f.Lists))
	for _, list := range f.Lists {
		sort.Slice(list.Items, func(i, j int) bool {
			return list.Items[i].Position < list.Items[j].Position
		})

		items := make([]model.Item, len(list.Items))
		copy(items, list.Items)
		list.Items = items
		lists = append(lists, list)
	}

	sort.Slice(lists, func(i, j int) bool {
		return lists[i].Position < lists[j].Position
	})

	return lists, nil
}

func (f *FakeProvider) UpdateList(_ context.Context, updatedList *model.List, _ []model.Item) error {
	if updatedList.Status == "" {
		updatedList.Status = model.StatusOpen
	}

	if !isValidListStatus(updatedList.Status) {
		return fmt.Errorf("invalid status: %q", updatedList.Status)
	}

	if updatedList.Name == "" {
		return errors.New("list name cannot be empty")
	}

	if f.Name == "generic" {
		for i, list := range f.Lists {
			if f.GetKey(&list) != "" || list.Name != updatedList.Name {
				continue
			}

			updatedListKey := f.GetKey(updatedList)
			f.SetKey(&list, updatedListKey)
			f.Lists[i] = list
			break
		}
	}

	listItems := []model.Item{}
	for i, updatedItem := range updatedList.Items {
		for j, list := range f.Lists {
			for k, item := range list.Items {
				genericMatch := f.Name == "generic" &&
					f.GetKey(&item) == "" &&
					item.Title == updatedItem.Title

				if !isMatch(&item, &updatedItem) && !genericMatch {
					continue
				}

				item.Position = i
				listItems = append(listItems, item)
				list.Items = append(list.Items[:k], list.Items[k+1:]...)
				f.Lists[j] = list
				break
			}
		}
	}

	for i, list := range f.Lists {
		if !isMatch(&list, updatedList) {
			continue
		}

		list.Position = updatedList.Position
		list.Status = updatedList.Status
		list.Name = updatedList.Name
		list.Modified = updatedList.Modified
		if updatedList.ExternalID != nil {
			list.ExternalID = updatedList.ExternalID
		}

		list.Items = append(list.Items, listItems...)
		if updatedList.Status == model.StatusDeleted {
			list.Items = []model.Item{}
		}

		for j, item := range list.Items {
			if f.Name != "external" {
				item.ListID = list.ID
			}

			item.ExternalListID = list.ExternalID
			list.Items[j] = item
		}

		f.Lists[i] = list

		return nil
	}

	return fmt.Errorf("list not found: %s", updatedList.ID)
}

func (f *FakeProvider) DeleteList(_ context.Context, deletedList *model.List) error {
	for i, list := range f.Lists {
		if isMatch(&list, deletedList) {
			f.Lists = append(f.Lists[:i], f.Lists[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("list not found: %s", deletedList.Name)
}

func (f *FakeProvider) CreateItem(_ context.Context, item *model.Item, _ string) (string, error) {
	if !isValidItemStatus(item.Status) {
		return "", fmt.Errorf("invalid status: %q", item.Status)
	}

	if item.Title == "" {
		return "", errors.New("item title cannot be empty")
	}

	itemKey := f.GetKey(item)
	if itemKey == "" {
		f.ItemCounter++
		itemKey = fmt.Sprintf("%s-item-%d", f.Name, f.ItemCounter)
		f.SetKey(item, itemKey)
	}

	for i, list := range f.Lists {
		if !isParent(&list, item) {
			continue
		}

		createdItem := *item
		if f.Name == "external" {
			createdItem.ID = ""
		}

		createdItem.ListID = list.ID
		createdItem.ExternalListID = list.ExternalID
		list.Items = append(list.Items, createdItem)
		f.Lists[i] = list

		return itemKey, nil
	}

	return "", fmt.Errorf("list ID and external list ID not found: %s, %v", item.ListID, item.ExternalListID)
}

func (f *FakeProvider) UpdateItem(_ context.Context, updatedItem *model.Item) error {
	if !isValidItemStatus(updatedItem.Status) {
		return fmt.Errorf("invalid status: %q", updatedItem.Status)
	}

	if updatedItem.Title == "" {
		return errors.New("item title cannot be empty")
	}

	if f.Name == "generic" {
		for i, list := range f.Lists {
			for j, item := range list.Items {
				if f.GetKey(&item) != "" || item.Title != updatedItem.Title {
					continue
				}

				updatedItemKey := f.GetKey(updatedItem)
				f.SetKey(&item, updatedItemKey)
				list.Items[j] = item
				f.Lists[i] = list
				break
			}
		}
	}

	for i, list := range f.Lists {
		for j, item := range list.Items {
			if !isMatch(&item, updatedItem) {
				continue
			}

			if !isParent(&list, updatedItem) {
				return fmt.Errorf(
					"item parent mismatch: item %s belongs to list %s (ID=%s, ExtID=%v), "+
						"but update request specifies parent ID=%s, ExtID=%v",
					updatedItem.Title,
					list.Name,
					list.ID,
					list.ExternalID,
					updatedItem.ListID,
					updatedItem.ExternalListID,
				)
			}

			item.Status = updatedItem.Status
			item.Title = updatedItem.Title
			item.Description = updatedItem.Description
			item.ProjectID = updatedItem.ProjectID
			item.WaitingOn = updatedItem.WaitingOn
			item.Snoozed = updatedItem.Snoozed
			item.Due = updatedItem.Due
			item.Tags = updatedItem.Tags
			item.Modified = updatedItem.Modified
			if updatedItem.ExternalID != nil {
				item.ExternalID = updatedItem.ExternalID
			}

			if updatedItem.ExternalListID != nil {
				item.ExternalListID = updatedItem.ExternalListID
			}

			list.Items[j] = item
			f.Lists[i] = list

			return nil
		}
	}

	return fmt.Errorf("item not found: %s", updatedItem.ID)
}

func (f *FakeProvider) DeleteItem(_ context.Context, deletedItem *model.Item) error {
	for i, list := range f.Lists {
		for j, item := range list.Items {
			if !isMatch(&item, deletedItem) {
				continue
			}

			list.Items = append(list.Items[:j], list.Items[j+1:]...)
			f.Lists[i] = list

			return nil
		}
	}

	return fmt.Errorf("item not found: %s", deletedItem.ID)
}

func isValidListStatus(status model.Status) bool {
	switch status {
	case model.StatusOpen, model.StatusDeleted:
		return true
	default:
		return false
	}
}

func isValidItemStatus(status model.Status) bool {
	switch status {
	case model.StatusNotStarted, model.StatusInProgress, model.StatusDone, model.StatusDeleted:
		return true
	default:
		return false
	}
}

func isMatch(a, b model.Resource) bool {
	if a.GetID() != "" && a.GetID() == b.GetID() {
		return true
	}

	if a.GetExternalID() == nil || b.GetExternalID() == nil {
		return false
	}

	if *a.GetExternalID() == *b.GetExternalID() {
		return true
	}

	return false
}

func isParent(list *model.List, item *model.Item) bool {
	if list.ID != "" && list.ID == item.ListID {
		return true
	}

	if list.ExternalID == nil || item.ExternalListID == nil {
		return false
	}

	if *list.ExternalID == *item.ExternalListID {
		return true
	}

	return false
}

func TestOneWaySync(t *testing.T) {
	t.Parallel()
	baseTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		src          *FakeProvider
		dst          *FakeProvider
		wantSrcLists []model.List
		wantDstLists []model.List
		wantUpdated  bool
	}{
		{
			name: "create list (push)",
			src: NewFakeProvider("store", []model.List{
				{Name: "L1"},
			}),
			dst: NewFakeProvider("generic", []model.List{}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list (pull)",
			src: NewFakeProvider("generic", []model.List{
				{Name: "L1"},
			}),
			dst: NewFakeProvider("store", []model.List{}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list external (push)",
			src: NewFakeProvider("store", []model.List{
				{Name: "L1"},
			}),
			dst: NewFakeProvider("external", []model.List{}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list external (pull)",
			src: NewFakeProvider("external", []model.List{
				{Name: "L1"},
			}),
			dst: NewFakeProvider("store", []model.List{}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusNotStarted,
						},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:   "store-list-1",
					Name: "L1",
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:   "store-list-1",
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusNotStarted,
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{Name: "L1"},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusNotStarted,
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{Name: "L1"},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-item-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-item-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusNotStarted,
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-item-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-item-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create deleted item (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusDeleted,
						},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:   "store-list-1",
					Name: "L1",
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusDeleted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
				},
			},
			wantUpdated: false,
		},
		{
			name: "create deleted item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusDeleted,
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{Name: "L1"},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantUpdated: false,
		},
		{
			name: "create list and create item (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusNotStarted,
						},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and create item (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusNotStarted,
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusNotStarted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and create item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusNotStarted,
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-item-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-item-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and create item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusNotStarted,
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalID:     stringPtr("external-item-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalID:     stringPtr("external-item-1"),
							ExternalListID: stringPtr("external-list-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and move item (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:     "L1",
					Modified: baseTime.Add(1),
					Items:    []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{
							ID:    "store-item-1",
							Title: "I1",
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 1,
					Status:   model.StatusOpen,
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and move item (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Modified: baseTime.Add(1),
					Items:    []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							ID:    "store-item-1",
							Title: "I1",
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Position: 0,
					Status:   model.StatusOpen,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Position: 1,
					Status:   model.StatusOpen,
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and move item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					Modified:   baseTime.Add(1),
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							Title:          "I1",
							Position:       0,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list and move item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name:     "L1",
					Modified: baseTime.Add(1),
					Items:    []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Modified:   baseTime,
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							Title:          "I1",
							Position:       0,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Position:   0,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Position:   1,
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list create item and move item (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:     "L1",
					Modified: baseTime.Add(1),
					Items:    []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							ID:     "store-item-2",
							Title:  "I2",
							Status: model.StatusNotStarted,
						},
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusInProgress,
						},
						{
							ID:     "store-item-3",
							Title:  "I3",
							Status: model.StatusInProgress,
						},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusInProgress,
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list create item and move item (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Modified: baseTime.Add(1),
					Items:    []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							Title:  "I2",
							Status: model.StatusNotStarted,
						},
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusInProgress,
						},
						{
							Title:  "I3",
							Status: model.StatusInProgress,
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusInProgress,
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							Status:   model.StatusNotStarted,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							Status:   model.StatusInProgress,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list create item and move item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					Modified:   baseTime.Add(1),
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							ID:     "store-item-2",
							Title:  "I2",
							Status: model.StatusNotStarted,
						},
						{
							ID:         "store-item-1",
							Title:      "I1",
							Status:     model.StatusInProgress,
							ExternalID: stringPtr("external-item-1"),
						},
						{
							ID:     "store-item-3",
							Title:  "I3",
							Status: model.StatusInProgress,
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusInProgress,
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       1,
							Status:         model.StatusInProgress,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							ID:             "store-item-3",
							Title:          "I3",
							Position:       2,
							Status:         model.StatusInProgress,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-3"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							Title:          "I2",
							Position:       0,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							Title:          "I1",
							Position:       1,
							Status:         model.StatusInProgress,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							Title:          "I3",
							Position:       2,
							Status:         model.StatusInProgress,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-3"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "create list create item and move item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name:     "L1",
					Modified: baseTime.Add(1),
					Items:    []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							Title:      "I2",
							Status:     model.StatusNotStarted,
							ExternalID: stringPtr("external-item-2"),
						},
						{
							Title:      "I1",
							Status:     model.StatusInProgress,
							ExternalID: stringPtr("external-item-1"),
						},
						{
							Title:      "I3",
							Status:     model.StatusInProgress,
							ExternalID: stringPtr("external-item-3"),
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Modified:   baseTime,
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
							Status:     model.StatusInProgress,
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							Title:          "I2",
							Position:       0,
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							Title:          "I1",
							Position:       1,
							Status:         model.StatusInProgress,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							Title:          "I3",
							Position:       2,
							Status:         model.StatusInProgress,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-3"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       0,
							Status:         model.StatusNotStarted,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       1,
							Status:         model.StatusInProgress,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							ID:             "store-item-3",
							Title:          "I3",
							Position:       2,
							Status:         model.StatusInProgress,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-3"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:     "L1 Updated",
					Modified: baseTime.Add(1),
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1 Original",
					Modified: baseTime,
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1 Updated",
					Modified: baseTime.Add(1),
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:     "L1 Original",
					Modified: baseTime,
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1 Updated",
					ExternalID: stringPtr("external-list-1"),
					Modified:   baseTime.Add(1),
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name:     "L1 Original",
					Modified: baseTime,
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantUpdated: true,
		},
		{
			name: "update list external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name:     "L1 Updated",
					Modified: baseTime.Add(1),
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1 Original",
					ExternalID: stringPtr("external-list-1"),
					Modified:   baseTime,
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:    "I1 Updated",
							Status:   model.StatusDone,
							Modified: baseTime.Add(1),
						},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:   "store-list-1",
					Name: "L1",
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1 Original",
							Status:   model.StatusNotStarted,
							ListID:   "store-list-1",
							Modified: baseTime,
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusDone,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusDone,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:   "store-list-1",
					Name: "L1",
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1 Updated",
							Status:   model.StatusInProgress,
							ListID:   "store-list-1",
							Modified: baseTime.Add(1),
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:    "I1 Original",
							Status:   model.StatusDone,
							Modified: baseTime,
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusInProgress,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1 Updated",
							Status: model.StatusInProgress,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1 Updated",
							Status:     model.StatusDone,
							Modified:   baseTime.Add(1),
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:    "I1 Original",
							Status:   model.StatusNotStarted,
							Modified: baseTime,
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Updated",
							Status:         model.StatusDone,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusDone,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "update item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:    "I1 Updated",
							Status:   model.StatusNotStarted,
							Modified: baseTime.Add(1),
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1 Original",
							Status:     model.StatusDone,
							Modified:   baseTime,
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I1 Updated",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1 Updated",
							Status:         model.StatusNotStarted,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "no status update item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1",
							Status:     model.StatusInProgress,
							Modified:   baseTime.Add(1),
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Modified: baseTime,
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusInProgress,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I1",
							Status:         model.StatusInProgress,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "no status update item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:    "I1",
							Status:   model.StatusNotStarted,
							Modified: baseTime.Add(1),
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1",
							Status:     model.StatusInProgress,
							Modified:   baseTime,
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I1",
							Status:         model.StatusNotStarted,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusInProgress,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "reorder items (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:     "L1 Updated",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{Title: "I1"},
						{Title: "I2"},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1 Original",
					Modified: baseTime,
					Items: []model.Item{
						{
							ID:    "store-item-2",
							Title: "I2",
						},
						{
							ID:    "store-item-1",
							Title: "I1",
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "reorder items (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1 Updated",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							ID:    "store-item-2",
							Title: "I2",
						},
						{
							ID:    "store-item-1",
							Title: "I1",
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:     "L1 Original",
					Modified: baseTime,
					Items: []model.Item{
						{Title: "I1"},
						{Title: "I2"},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1 Updated",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							ListID:   "store-list-1",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							ListID:   "store-list-1",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "reorder items external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1 Updated",
					Modified:   baseTime.Add(1),
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
						{
							Title:      "I2",
							ExternalID: stringPtr("external-item-2"),
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name:     "L1 Original",
					Modified: baseTime,
					Items: []model.Item{
						{
							Title:      "I2",
							ExternalID: stringPtr("external-item-2"),
						},
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-2"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I1",
							Position:       0,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							Title:          "I2",
							Position:       1,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-2"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "reorder items external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name:     "L1 Updated",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							Title:      "I2",
							ExternalID: stringPtr("external-item-2"),
						},
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1 Original",
					Modified:   baseTime,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
						{
							Title:      "I2",
							ExternalID: stringPtr("external-item-2"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:          "I2",
							Position:       0,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							Title:          "I1",
							Position:       1,
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1 Updated",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       0,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       1,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "move item (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:     "L1",
					Modified: baseTime.Add(1),
					Items:    []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{Title: "I1"},
						{Title: "I2"},
						{Title: "I3"},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{
							ID:    "store-item-2",
							Title: "I2",
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Modified: baseTime,
					Items: []model.Item{
						{
							ID:    "store-item-1",
							Title: "I1",
						},
						{
							ID:    "store-item-3",
							Title: "I3",
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 0,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 1,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "move item (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Modified: baseTime.Add(1),
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							ID:    "store-item-2",
							Title: "I2",
						},
						{
							ID:    "store-item-1",
							Title: "I1",
						},
						{
							ID:    "store-item-3",
							Title: "I3",
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{Title: "I1"},
					},
				},
				{
					Name:     "L2",
					Modified: baseTime,
					Items: []model.Item{
						{Title: "I2"},
						{Title: "I3"},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusOpen,
					Position: 0,
					Items:    []model.Item{},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:       "store-item-2",
							Title:    "I2",
							Position: 0,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-1",
							Title:    "I1",
							Position: 1,
							ListID:   "store-list-2",
						},
						{
							ID:       "store-item-3",
							Title:    "I3",
							Position: 2,
							ListID:   "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "move item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Modified:   baseTime.Add(1),
					Items:      []model.Item{},
				},
				{
					Name:       "L2",
					ExternalID: stringPtr("external-list-2"),
					Modified:   baseTime.Add(1),
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
						{
							Title:      "I2",
							ExternalID: stringPtr("external-item-2"),
						},
						{
							Title:      "I3",
							ExternalID: stringPtr("external-item-3"),
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{
							Title:      "I2",
							ExternalID: stringPtr("external-item-2"),
						},
					},
				},
				{
					Name:     "L2",
					Modified: baseTime,
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
						{
							Title:      "I3",
							ExternalID: stringPtr("external-item-3"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       0,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       1,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							ID:             "store-item-3",
							Title:          "I3",
							Position:       2,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-3"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							Title:          "I1",
							Position:       0,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							Title:          "I2",
							Position:       1,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							Title:          "I3",
							Position:       2,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-3"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "move item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name:     "L1",
					Modified: baseTime.Add(1),
					Items:    []model.Item{},
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							Title:      "I2",
							ExternalID: stringPtr("external-item-2"),
						},
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
						{
							Title:      "I3",
							ExternalID: stringPtr("external-item-3"),
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Modified:   baseTime,
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
				{
					Name:       "L2",
					ExternalID: stringPtr("external-list-2"),
					Modified:   baseTime,
					Items: []model.Item{
						{
							Title:      "I2",
							ExternalID: stringPtr("external-item-2"),
						},
						{
							Title:      "I3",
							ExternalID: stringPtr("external-item-3"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							Title:          "I2",
							Position:       0,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							Title:          "I1",
							Position:       1,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							Title:          "I3",
							Position:       2,
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-3"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					Position:   0,
					ExternalID: stringPtr("external-list-1"),
					Items:      []model.Item{},
				},
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							ID:             "store-item-2",
							Title:          "I2",
							Position:       0,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-2"),
						},
						{
							ID:             "store-item-1",
							Title:          "I1",
							Position:       1,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
						{
							ID:             "store-item-3",
							Title:          "I3",
							Position:       2,
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-3"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:   "L1",
					Status: model.StatusDeleted,
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:   "store-list-1",
					Name: "L1",
					Items: []model.Item{
						{
							ID:    "store-item-1",
							Title: "I1",
						},
					},
				},
			}),
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{},
			wantUpdated:  true,
		},
		{
			name: "delete list (pull)",
			src:  NewFakeProvider("generic", []model.List{}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusDeleted,
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusDeleted,
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{},
			wantUpdated:  true,
		},
		{
			name: "delete list external (pull)",
			src:  NewFakeProvider("external", []model.List{}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusDeleted,
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete already deleted list (pull)",
			src:  NewFakeProvider("generic", []model.List{}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:   "L1",
					Status: model.StatusDeleted,
				},
			}),
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusDeleted,
				},
			},
			wantUpdated: false,
		},
		{
			name: "delete already deleted list external (pull)",
			src:  NewFakeProvider("external", []model.List{}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					Status:     model.StatusDeleted,
					ExternalID: stringPtr("external-list-1"),
				},
			}),
			wantSrcLists: []model.List{},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusDeleted,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantUpdated: false,
		},
		{
			name: "delete item (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusDeleted,
						},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:   "store-list-1",
					Name: "L1",
					Items: []model.Item{
						{
							ID:    "store-item-1",
							Title: "I1",
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete item (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:   "store-list-1",
					Name: "L1",
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							ListID: "store-list-1",
							Status: model.StatusDeleted,
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
							Status:     model.StatusDeleted,
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusOpen,
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusOpen,
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{Name: "L1"},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusOpen,
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusOpen,
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
							Status:         model.StatusDeleted,
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete already deleted item (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:   "store-list-1",
					Name: "L1",
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name: "L1",
					Items: []model.Item{
						{
							Title:  "I1",
							Status: model.StatusDeleted,
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
				},
			},
			wantDstLists: []model.List{
				{
					ID:     "store-list-1",
					Name:   "L1",
					Status: model.StatusOpen,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							Status: model.StatusDeleted,
							ListID: "store-list-1",
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "delete already deleted item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{Name: "L1"},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1",
							Status:     model.StatusDeleted,
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-1",
					Name:       "L1",
					Status:     model.StatusOpen,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							Status:         model.StatusDeleted,
							ListID:         "store-list-1",
							ExternalListID: stringPtr("external-list-1"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "delete list move item (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:     "L1",
					Status:   model.StatusDeleted,
					Modified: baseTime.Add(1),
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			dst: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{
							ID:    "store-item-1",
							Title: "I1",
						},
					},
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Modified: baseTime,
					Items:    []model.Item{},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							ListID: "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							ListID: "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list move item (pull)",
			src: NewFakeProvider("generic", []model.List{
				{
					ID:       "store-list-1",
					Name:     "L1",
					Status:   model.StatusDeleted,
					Modified: baseTime.Add(1),
				},
				{
					ID:       "store-list-2",
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{
							ID:    "store-item-1",
							Title: "I1",
						},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{Title: "I1"},
					},
				},
				{
					Name:     "L2",
					Modified: baseTime,
					Items:    []model.Item{},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							ListID: "store-list-2",
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:       "store-list-2",
					Name:     "L2",
					Status:   model.StatusOpen,
					Position: 1,
					Items: []model.Item{
						{
							ID:     "store-item-1",
							Title:  "I1",
							ListID: "store-list-2",
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list move item external (push)",
			src: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					ExternalID: stringPtr("external-list-1"),
					Status:     model.StatusDeleted,
					Modified:   baseTime.Add(1),
				},
				{
					Name:       "L2",
					ExternalID: stringPtr("external-list-2"),
					Modified:   baseTime.Add(1),
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
			}),
			dst: NewFakeProvider("external", []model.List{
				{
					Name:     "L1",
					Modified: baseTime,
					Items: []model.Item{
						{Title: "I1"},
					},
				},
				{
					Name:     "L2",
					Modified: baseTime,
					Items:    []model.Item{},
				},
			}),
			wantSrcLists: []model.List{
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							Title:          "I1",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "delete list move item external (pull)",
			src: NewFakeProvider("external", []model.List{
				{
					Name:     "L1",
					Status:   model.StatusDeleted,
					Modified: baseTime.Add(1),
				},
				{
					Name:     "L2",
					Modified: baseTime.Add(1),
					Items: []model.Item{
						{Title: "I1"},
					},
				},
			}),
			dst: NewFakeProvider("store", []model.List{
				{
					Name:       "L1",
					Modified:   baseTime,
					ExternalID: stringPtr("external-list-1"),
					Items: []model.Item{
						{
							Title:      "I1",
							ExternalID: stringPtr("external-item-1"),
						},
					},
				},
				{
					Name:       "L2",
					Modified:   baseTime,
					ExternalID: stringPtr("external-list-2"),
					Items:      []model.Item{},
				},
			}),
			wantSrcLists: []model.List{
				{
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							Title:          "I1",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantDstLists: []model.List{
				{
					ID:         "store-list-2",
					Name:       "L2",
					Status:     model.StatusOpen,
					Position:   1,
					ExternalID: stringPtr("external-list-2"),
					Items: []model.Item{
						{
							ID:             "store-item-1",
							Title:          "I1",
							ListID:         "store-list-2",
							ExternalListID: stringPtr("external-list-2"),
							ExternalID:     stringPtr("external-item-1"),
						},
					},
				},
			},
			wantUpdated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var s *Syncer
			switch {
			case tt.src.Name == "store":
				s = NewSyncer(tt.src, tt.dst)
			case tt.dst.Name == "store":
				s = NewSyncer(tt.dst, tt.src)
			default:
				t.Fatalf("test must have at least one 'store' provider")
			}

			updated, err := s.oneWaySync(context.Background(), tt.src, tt.dst)
			if err != nil {
				t.Fatalf("oneWaySync failed: %v", err)
			}

			if updated != tt.wantUpdated {
				t.Errorf("updated = %v, want %v", updated, tt.wantUpdated)
			}

			opts := []cmp.Option{
				cmpopts.IgnoreFields(model.List{}, "Modified"),
				cmpopts.IgnoreFields(model.Item{}, "Modified", "Created"),
				cmpopts.EquateEmpty(),
			}

			gotSrcLists, _ := tt.src.ListLists(context.Background())
			if diff := cmp.Diff(tt.wantSrcLists, gotSrcLists, opts...); diff != "" {
				t.Errorf("Source state mismatch (-want +got):\n%s", diff)
			}

			gotDstLists, _ := tt.dst.ListLists(context.Background())
			if diff := cmp.Diff(tt.wantDstLists, gotDstLists, opts...); diff != "" {
				t.Errorf("Destination state mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
