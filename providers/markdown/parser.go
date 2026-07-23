package markdown

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/danrneal/gtd-cli/model"
)

var (
	// Matches Markdown headers: # List Name OR # List Name (1) OR # List Name {{list-id}}.
	listRegex = regexp.MustCompile(`^#+\s+(.+?)(?:\s+\(\d+\))?(?:\s+{{([^}]+)}})?$`)
	// Matches Markdown list items: * [ ] Item Title OR - [x] ~~Item Title~~ OR * [ ] Item Title {{item-id}}.
	itemRegex = regexp.MustCompile(`^[*-]\s+\[(.)\]\s+~*(.+?)~*(?:\s+{{([^}]+)}})?$`)
)

// parse reads Markdown content and converts it into a slice of model.List.
func parse(reader io.Reader, modified time.Time) ([]model.List, error) {
	p := parser{}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)
		if matches := listRegex.FindStringSubmatch(trimmedLine); matches != nil {
			p.flushList(modified)

			list := &model.List{
				ID:    strings.TrimSpace(matches[2]),
				Name:  matches[1],
				Items: []*model.Item{},
			}

			p.list = list
			continue
		}

		if matches := itemRegex.FindStringSubmatch(trimmedLine); matches != nil {
			if p.list == nil {
				continue
			}

			p.flushItem(modified)

			var item *model.Item
			itemContent := matches[2]
			if strings.HasPrefix(p.list.Name, model.ListWaitingFor) {
				item = parseWaitingForItemContent(itemContent)
			} else {
				item = parseItemContent(itemContent)
			}

			item.ID = strings.TrimSpace(matches[3])
			item.ListID = p.list.ID
			switch matches[1] {
			case " ":
				item.Status = model.StatusNotStarted
			case "x", "X":
				item.Status = model.StatusDone
			default:
				item.Status = model.StatusInProgress
			}

			p.item = item
			continue
		}

		if p.item != nil {
			p.item.Description += fmt.Sprintln(line)
		}
	}

	p.flushList(modified)

	if err := scanner.Err(); err != nil {
		return p.lists, fmt.Errorf("failed to scan markdown file: %w", err)
	}

	return p.lists, nil
}

// parser maintains the running state during the Markdown parsing process.
type parser struct {
	lists []model.List
	list  *model.List
	item  *model.Item
}

// flushItem finalizes the current item and appends it to the active list.
func (p *parser) flushItem(modified time.Time) {
	if p.list == nil || p.item == nil {
		return
	}

	p.item.Position = len(p.list.Items)
	p.item.Modified = modified
	p.item.Clean()
	p.list.Items = append(p.list.Items, p.item)
	p.item = nil
}

// flushList finalizes the current list, including its active item, and appends it to the master slice.
func (p *parser) flushList(modified time.Time) {
	if p.list == nil {
		return
	}

	p.flushItem(modified)
	p.list.Position = len(p.lists)
	p.list.Modified = modified
	p.list.Clean()
	p.lists = append(p.lists, *p.list)
}

// parseWaitingForItemContent extracts the delegated person from a "Waiting For" item and parses the remaining content.
func parseWaitingForItemContent(content string) *model.Item {
	var waitingOn string
	parts := strings.Split(content, " - ")
	if len(parts) > 1 {
		waitingOn = strings.TrimSpace(parts[0])
		content = parts[1]
	}

	item := parseItemContent(content)
	if waitingOn != "" {
		item.WaitingOn = waitingOn
	}

	if parts[len(parts)-1] != content {
		created := strings.TrimSpace(parts[len(parts)-1])
		if created, err := time.Parse("2006-01-02", created); err == nil {
			item.Created = created
		}
	}

	return item
}

// parseItemContent extracts metadata such as projects, tags, and dates from the raw item string.
func parseItemContent(content string) *model.Item {
	item := &model.Item{}

	var titleParts []string
	for field := range strings.FieldsSeq(content) {
		switch {
		case strings.HasPrefix(field, "+") && len(field) > 1:
			projectID := field[1:]
			item.ProjectID = &projectID
		case strings.HasPrefix(field, "snoozed:"):
			snoozed := strings.TrimPrefix(field, "snoozed:")
			if snoozed, err := time.Parse("2006-01-02", snoozed); err == nil {
				item.Snoozed = &snoozed
			} else {
				titleParts = append(titleParts, field)
			}
		case strings.HasPrefix(field, "due:"):
			due := strings.TrimPrefix(field, "due:")
			if due, err := time.Parse("2006-01-02", due); err == nil {
				item.Due = &due
			} else {
				titleParts = append(titleParts, field)
			}
		case strings.HasPrefix(field, "#") && len(field) > 1:
			item.Tags = append(item.Tags, field[1:])
		default:
			titleParts = append(titleParts, field)
		}
	}

	item.Title = strings.Join(titleParts, " ")

	return item
}
