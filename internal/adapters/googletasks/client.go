package googletasks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/danrneal/gtd.nvim/internal/model"
	"google.golang.org/api/tasks/v1"
)

// Client is a wrapper around the Google Tasks service.
type Client struct {
	service *tasks.Service
}

// NewClient returns a new Google Tasks client.
func NewClient(service *tasks.Service) *Client {
	client := &Client{service: service}

	return client
}

// GetAllItems retrieves all tasks from the specified lists and converts them to internal Items.
// It handles fetching, sorting, and parsing metadata from task titles.
func (c *Client) GetAllItems(ctx context.Context, lists []model.List) ([]model.Item, error) {
	var items []model.Item
	for _, list := range lists {
		tasks, err := c.service.Tasks.List(*list.ExternalID).ShowHidden(true).MaxResults(100).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("unable to retrieve tasks for list %q: %w", list.Name, err)
		}

		sort.Slice(tasks.Items, func(i, j int) bool {
			return tasks.Items[i].Position < tasks.Items[j].Position
		})

		for i, task := range tasks.Items {
			waitingFor := list.Name == "Waiting For"
			item := parseTitle(task.Title, waitingFor)
			item.ListID = list.ID
			item.Position = i
			item.Completed = task.Status == "completed"
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
			items = append(items, item)
		}
	}

	return items, nil
}

// parseTitle extracts metadata (project, tags, due date) from the task title string.
// It supports special handling for "Waiting For" lists to extract the waiting-on person.
func parseTitle(title string, waitingFor bool) model.Item {
	var item model.Item
	if waitingFor {
		titleParts := strings.SplitN(title, " - ", 3)
		if len(titleParts) > 1 {
			waitingOn := titleParts[0]
			item.WaitingOn = &waitingOn
			title = titleParts[1]
		}
	}

	var titleParts []string
	titleFields := strings.Fields(title)
	for _, titleField := range titleFields {
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
