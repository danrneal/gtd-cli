// Package model contains the domain structures for the gtd application.
package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Item represents a single task or action item within a List.
type Item struct {
	ID             string     `json:"id"`
	ListID         string     `json:"listId"`
	Position       int        `json:"position"`
	Status         Status     `json:"status"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	ProjectID      *string    `json:"projectId"`
	WaitingOn      *string    `json:"waitingOn"`
	Snoozed        *time.Time `json:"snoozed"`
	Due            *time.Time `json:"due"`
	Tags           []string   `json:"tags"`
	Modified       time.Time  `json:"modified"`
	Created        time.Time  `json:"created"`
	ExternalID     *string    `json:"externalId"`
	ExternalListID *string    `json:"externalListId"`
}

// GetID returns the internal ID of the item.
func (i *Item) GetID() string {
	return i.ID
}

// GetExternalID returns the external ID of the item.
func (i *Item) GetExternalID() *string {
	return i.ExternalID
}

// Clean formats the item data into its canonical representation.
func (i *Item) Clean() {
	i.Title = strings.TrimSpace(i.Title)
	i.Description = trimDescription(i.Description)
	if i.Status == "" {
		i.Status = StatusNotStarted
	}
}

// Validate checks if the current state of the item satisfies domain invariants.
func (i *Item) Validate() error {
	if i.Title == "" {
		return errors.New("item title cannot be empty")
	}

	if i.ListID == "" && i.ExternalListID == nil {
		return errors.New("item must have an internal or external list ID")
	}

	switch i.Status {
	case StatusNotStarted, StatusInProgress, StatusDone, StatusDeleted:
	default:
		return fmt.Errorf("invalid item status: %q", i.Status)
	}

	return nil
}

// trimDescription strips common leading indentation and trailing whitespace from multiline text.
func trimDescription(description string) string {
	lines := strings.Split(description, "\n")
	baseIndent := ""
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		content := strings.TrimLeft(line, " \t")
		baseIndent = line[:len(line)-len(content)]
		break
	}

	for i, line := range lines {
		line = strings.TrimPrefix(line, baseIndent)
		line = strings.TrimRightFunc(line, unicode.IsSpace)
		lines[i] = line
	}

	description = strings.Join(lines, "\n")
	description = strings.TrimSpace(description)

	return description
}
