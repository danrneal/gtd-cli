package markdown

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/danrneal/gtd.nvim/internal/model"
)

var (
	listRegex = regexp.MustCompile(`^#+\s+(.+?)(?:\s+\(\d+\))?(?:\s+{{([^}]+)}})?$`)
	itemRegex = regexp.MustCompile(`^[*-]\s+\[([ xX-])\]\s+~*(.+?)(?:\s+{{([^}]+)}})?~*$`)
)

// Parse reads Markdown content and converts it into a slice of model.List.
func Parse(reader io.Reader) ([]model.List, error) {
	var (
		lists []model.List
		list  model.List
		item  model.Item
	)

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)
		if matches := listRegex.FindStringSubmatch(trimmedLine); matches != nil {
			if item.Title != "" {
				item.Clean()
				list.Items = append(list.Items, &item)
			}

			if list.Name != "" {
				lists = append(lists, list)
			}

			listName := strings.TrimSpace(matches[1])
			listID := strings.TrimSpace(matches[2])
			list = model.List{
				ID:    listID,
				Name:  listName,
				Items: []*model.Item{},
			}

			continue
		}

		if matches := itemRegex.FindStringSubmatch(trimmedLine); matches != nil {
			if list.Name == "" {
				continue
			}

			if item.Title != "" {
				item.Clean()
				list.Items = append(list.Items, &item)
			}

			itemStatus := model.StatusNotStarted
			switch matches[1] {
			case "-":
				itemStatus = model.StatusInProgress
			case "x", "X":
				itemStatus = model.StatusDone
			}

			itemID := strings.TrimSpace(matches[3])
			item = model.Item{
				ID:     itemID,
				ListID: list.ID,
				Status: itemStatus,
			}

			var titleParts []string
			itemContent := strings.TrimSpace(matches[2])
			for field := range strings.FieldsSeq(itemContent) {
				switch {
				case strings.HasPrefix(field, "+"):
					if len(field) > 1 {
						projectID := field[1:]
						item.ProjectID = &projectID
					}
				case strings.HasPrefix(field, "snoozed:"):
					snoozedStr := strings.TrimPrefix(field, "snoozed:")
					if snoozed, err := time.Parse("2006-01-02", snoozedStr); err == nil {
						item.Snoozed = &snoozed
					}
				case strings.HasPrefix(field, "due:"):
					dueStr := strings.TrimPrefix(field, "due:")
					if due, err := time.Parse("2006-01-02", dueStr); err == nil {
						item.Due = &due
					}
				case strings.HasPrefix(field, "#"):
					if len(field) > 1 {
						item.Tags = append(item.Tags, field[1:])
					}
				default:
					titleParts = append(titleParts, field)
				}
			}

			item.Title = strings.Join(titleParts, " ")

			if list.Name == "Waiting For" {
				parts := strings.Split(item.Title, " - ")
				if len(parts) > 1 {
					waitingOn := strings.TrimSpace(parts[0])
					item.WaitingOn = &waitingOn
					item.Title = strings.TrimSpace(parts[1])
				}
			}

			continue
		}

		if item.Title != "" {
			item.Description += fmt.Sprintln(line)
		}
	}

	if item.Title != "" {
		item.Clean()
		list.Items = append(list.Items, &item)
	}

	if list.Name != "" {
		lists = append(lists, list)
	}

	if err := scanner.Err(); err != nil {
		return lists, fmt.Errorf("failed to scan markdown file: %w", err)
	}

	return lists, nil
}
