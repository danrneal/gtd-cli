package markdown

import (
	"fmt"
	"io"
	"strings"

	"github.com/danrneal/gtd-cli/model"
)

// render writes a slice of model.List to the provided [io.Writer] in Markdown format.
func render(writer io.Writer, lists []model.List) error {
	buf := strings.Builder{}
	for _, list := range lists {
		if err := renderList(&buf, &list); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprint(writer, buf.String()); err != nil {
		return fmt.Errorf("failed to write to writer: %w", err)
	}

	return nil
}

// renderList formats a single list and its items into the provided string builder.
func renderList(buf *strings.Builder, list *model.List) error {
	list.Clean()

	listParts := []string{"#", list.Name}

	count := fmt.Sprintf("(%d)", len(list.Items))
	listParts = append(listParts, count)

	if list.ID != "" {
		listID := fmt.Sprintf("{{%s}}", list.ID)
		listParts = append(listParts, listID)
	}

	listTitle := strings.Join(listParts, " ")
	buf.WriteString(listTitle)
	buf.WriteString("\n")

	for _, item := range list.Items {
		if err := renderItem(buf, item); err != nil {
			return err
		}
	}

	buf.WriteString("\n")

	return nil
}

// renderItem formats a single item, including its status and metadata, into the provided string builder.
func renderItem(buf *strings.Builder, item *model.Item) error {
	item.Clean()

	title := renderTitle(item)

	var itemStatus string
	switch item.Status {
	case model.StatusNotStarted:
		itemStatus = " "
	case model.StatusInProgress:
		itemStatus = "-"
	case model.StatusDone:
		itemStatus = "x"
		title = fmt.Sprintf("~~%s~~", title)
	default:
		return fmt.Errorf("render received invalid status %q on item %s", item.Status, item.ID)
	}

	status := fmt.Sprintf("* [%s] ", itemStatus)
	buf.WriteString(status)
	buf.WriteString(title)

	if item.ID != "" {
		itemID := fmt.Sprintf(" {{%s}}", item.ID)
		buf.WriteString(itemID)
	}

	buf.WriteString("\n")

	if item.Description != "" {
		description := strings.ReplaceAll(item.Description, "\n", "\n    ")

		buf.WriteString("    ")
		buf.WriteString(description)
		buf.WriteString("\n")
	}

	return nil
}

// renderTitle constructs the task title string by appending all relevant metadata fields.
func renderTitle(item *model.Item) string {
	titleParts := []string{item.Title}

	if item.ProjectID != nil {
		projectID := fmt.Sprintf("+%s", *item.ProjectID)
		titleParts = append(titleParts, projectID)
	}

	if item.Due != nil {
		due := fmt.Sprintf("due:%s", item.Due.Format("2006-01-02"))
		titleParts = append(titleParts, due)
	}

	if item.Snoozed != nil {
		snoozed := fmt.Sprintf("snoozed:%s", item.Snoozed.Format("2006-01-02"))
		titleParts = append(titleParts, snoozed)
	}

	for _, tag := range item.Tags {
		tag = fmt.Sprintf("#%s", tag)
		titleParts = append(titleParts, tag)
	}

	title := strings.Join(titleParts, " ")

	if item.WaitingOn != "" {
		created := item.Created.Format("2006-01-02")
		title = fmt.Sprintf("%s - %s - %s", item.WaitingOn, title, created)
	}

	return title
}
