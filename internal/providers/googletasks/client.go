// Package googletasks implements the Google Tasks provider.
package googletasks

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/tasks/v1"

	"github.com/danrneal/gtd.nvim/internal/model"
	"github.com/danrneal/gtd.nvim/internal/providers/util/reorder"
)

// Client is a wrapper around the Google Tasks service.
type Client struct {
	service *tasks.Service
}

const (
	maxTaskResults    = 100
	statusNeedsAction = "needsAction"
	statusCompleted   = "completed"
)

// NewClient returns a new Google Tasks client.
func NewClient(service *tasks.Service) *Client {
	client := &Client{service: service}

	return client
}

// GetKey extracts the external ID from the resource.
func (c *Client) GetKey(resource model.Resource) string {
	if extID := resource.GetExternalID(); extID != nil {
		return *extID
	}

	return ""
}

// CreateList creates a new task list on the Google Tasks service.
func (c *Client) CreateList(ctx context.Context, list *model.List) error {
	list.Clean()
	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if list.Status != model.StatusOpen {
		return errors.New("new lists must have status 'open'")
	}

	tasklist := &tasks.TaskList{
		Title: list.Name,
	}

	taskList, err := c.service.Tasklists.Insert(tasklist).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to create tasklist %s: %w", tasklist.Title, err)
	}

	list.ExternalID = &taskList.Id

	return nil
}

// ListLists retrieves all task lists from Google Tasks.
func (c *Client) ListLists(ctx context.Context) ([]model.List, error) {
	resp, err := c.service.Tasklists.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve tasklists: %w", err)
	}

	var lists []model.List
	for i, tasklist := range resp.Items {
		list := model.List{
			Name:       tasklist.Title,
			Position:   i,
			Status:     model.StatusOpen,
			ExternalID: &tasklist.Id,
		}

		if tasklist.Updated != "" {
			if updated, err := time.Parse(time.RFC3339, tasklist.Updated); err == nil {
				list.Modified = updated
			}
		}

		items, err := c.listItems(ctx, &list)
		if err != nil {
			return nil, err
		}

		list.Items = items
		lists = append(lists, list)
	}

	return lists, nil
}

// UpdateList updates an existing task list on Google Tasks.
func (c *Client) UpdateList(ctx context.Context, list *model.List, currentItems []*model.Item) error {
	list.Clean()
	if err := list.Validate(); err != nil {
		return fmt.Errorf("invalid list: %w", err)
	}

	if list.ExternalID == nil {
		return errors.New("failed to update list: missing external ID")
	}

	tasklist := &tasks.TaskList{
		Title: list.Name,
	}

	if _, err := c.service.Tasklists.Patch(*list.ExternalID, tasklist).Context(ctx).Do(); err != nil {
		return fmt.Errorf("failed to update tasklist %s: %w", tasklist.Title, err)
	}

	moves := reorder.CalculateMoves(list, currentItems)
	for _, move := range moves {
		if err := c.moveItem(ctx, move); err != nil {
			return err
		}
	}

	return nil
}

// DeleteList deletes a task list from Google Tasks.
func (c *Client) DeleteList(ctx context.Context, list *model.List) error {
	if list.ExternalID == nil {
		return errors.New("failed to delete list: missing external ID")
	}

	if err := c.service.Tasklists.Delete(*list.ExternalID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("failed to delete tasklist: %w", err)
	}

	return nil
}

// CreateItem creates a new task in the specified Google Task list.
// If previousItemID is provided, the task is inserted after that item.
// It renders the item's title to include metadata (project, tags, due date) compatible with the parser.
func (c *Client) CreateItem(ctx context.Context, item *model.Item, previousItemID string) error {
	title := renderTitle(item)
	status := statusNeedsAction
	if item.Status == model.StatusDone {
		status = statusCompleted
	}

	var due string
	if item.Snoozed != nil {
		due = item.Snoozed.Format(time.RFC3339)
	}

	task := &tasks.Task{
		Title:  title,
		Notes:  item.Description,
		Status: status,
		Due:    due,
	}

	tasksInsertCall := c.service.Tasks.Insert(*item.ExternalListID, task)
	if previousItemID != "" {
		tasksInsertCall.Previous(previousItemID)
	}

	task, err := tasksInsertCall.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to insert task: %w", err)
	}

	item.ExternalID = &task.Id

	return nil
}

// listItems retrieves all tasks from the specified list and converts them to internal Items.
// It handles fetching, sorting, and parsing metadata from task titles.
func (c *Client) listItems(ctx context.Context, list *model.List) ([]*model.Item, error) {
	resp, err := c.service.Tasks.List(*list.ExternalID).ShowHidden(true).MaxResults(maxTaskResults).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve tasks for list %q: %w", list.Name, err)
	}

	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].Position < resp.Items[j].Position
	})

	var items []*model.Item
	for i, task := range resp.Items {
		var item model.Item
		if list.Name == "Waiting For" {
			item = parseWaitingForTitle(task.Title)
		} else {
			item = parseTitle(task.Title)
		}

		item.ListID = list.ID
		item.Position = i
		if task.Status == statusCompleted {
			item.Status = model.StatusDone
		} else {
			item.Status = model.StatusNotStarted
		}

		item.Description = task.Notes
		if task.Due != "" {
			if due, err := time.Parse(time.RFC3339, task.Due); err == nil {
				item.Snoozed = &due
			}
		}

		if task.Updated != "" {
			if updated, err := time.Parse(time.RFC3339, task.Updated); err == nil {
				item.Modified = updated
			}
		}

		externalID := task.Id
		item.ExternalID = &externalID
		item.ExternalListID = list.ExternalID
		items = append(items, &item)
	}

	return items, nil
}

// UpdateItem updates an existing task in the specified Google Task list.
func (c *Client) UpdateItem(ctx context.Context, item *model.Item) error {
	title := renderTitle(item)
	status := statusNeedsAction
	if item.Status == model.StatusDone {
		status = statusCompleted
	}

	var due string
	if item.Snoozed != nil {
		due = item.Snoozed.Format(time.RFC3339)
	}

	task := &tasks.Task{
		Title:  title,
		Notes:  item.Description,
		Status: status,
		Due:    due,
	}

	if _, err := c.service.Tasks.Patch(*item.ExternalListID, *item.ExternalID, task).Context(ctx).Do(); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	return nil
}

// moveItem moves a task to a new position, potentially in a different list.
func (c *Client) moveItem(ctx context.Context, move reorder.Move) error {
	tasksMoveCall := c.service.Tasks.Move(move.SourceListID, move.ItemID)
	if move.PreviousItemID != "" {
		tasksMoveCall.Previous(move.PreviousItemID)
	}

	if move.DestinationListID != "" {
		tasksMoveCall.DestinationTasklist(move.DestinationListID)
	}

	if _, err := tasksMoveCall.Context(ctx).Do(); err != nil {
		return fmt.Errorf("failed to move task: %w", err)
	}

	return nil
}

// DeleteItem deletes a task from the specified Google Task list.
func (c *Client) DeleteItem(ctx context.Context, item *model.Item) error {
	if err := c.service.Tasks.Delete(*item.ExternalListID, *item.ExternalID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	return nil
}

// renderTitle constructs the task title by combining it with metadata (project, tags, due date, waiting on).
func renderTitle(item *model.Item) string {
	titleParts := []string{item.Title}
	if item.ProjectID != nil {
		projectIDStr := fmt.Sprintf("+%s", *item.ProjectID)
		titleParts = append(titleParts, projectIDStr)
	}

	if item.Due != nil {
		dueStr := fmt.Sprintf("due:%s", item.Due.Format("2006-01-02"))
		titleParts = append(titleParts, dueStr)
	}

	for _, tag := range item.Tags {
		tagStr := fmt.Sprintf("#%s", tag)
		titleParts = append(titleParts, tagStr)
	}

	title := strings.Join(titleParts, " ")

	if item.WaitingOn != nil {
		createdStr := item.Created.Format("Jan 2")
		title = fmt.Sprintf("%s - %s - %s", *item.WaitingOn, title, createdStr)
	}

	return title
}

// parseWaitingForTitle extracts the waiting-on person from the title and then
// delegates to parseTitle for the rest of the metadata.
func parseWaitingForTitle(title string) model.Item {
	var waitingOn string
	titleParts := strings.SplitN(title, " - ", 3)
	if len(titleParts) > 1 {
		waitingOn = titleParts[0]
		title = titleParts[1]
	}

	item := parseTitle(title)
	if waitingOn != "" {
		item.WaitingOn = &waitingOn
	}

	return item
}

// parseTitle extracts metadata (project, tags, due date) from the task title string.
func parseTitle(title string) model.Item {
	var item model.Item

	var titleParts []string
	titleFields := strings.FieldsSeq(title)
	for titleField := range titleFields {
		switch {
		case strings.HasPrefix(titleField, "+"):
			if len(titleField) > 1 {
				projectID := titleField[1:]
				item.ProjectID = &projectID
			}
		case strings.HasPrefix(titleField, "due:"):
			dueStr := strings.TrimPrefix(titleField, "due:")
			if due, err := time.Parse("2006-01-02", dueStr); err == nil {
				item.Due = &due
			}
		case strings.HasPrefix(titleField, "#"):
			if len(titleField) > 1 {
				item.Tags = append(item.Tags, titleField[1:])
			}
		default:
			titleParts = append(titleParts, strings.TrimSpace(titleField))
		}
	}

	item.Title = strings.Join(titleParts, " ")

	return item
}
