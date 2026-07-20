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
	WaitingOn      string     `json:"waitingOn"`
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

	if i.Created.IsZero() {
		created := i.Modified.In(time.Local)
		i.Created = time.Date(created.Year(), created.Month(), created.Day(), 0, 0, 0, 0, time.UTC)
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
		// Valid status
	default:
		return fmt.Errorf("invalid item status: %q", i.Status)
	}

	if i.Modified.IsZero() {
		return errors.New("item modified timestamp cannot be zero")
	}

	return nil
}

// Equivalent compares the content of two items, ignoring metadata like Modified timestamps
// and dynamically applying nuanced checks for primary/external IDs.
func (i *Item) Equivalent(other *Item) bool {
	if other == nil {
		return false
	}

	if i.ID != "" && other.ID != "" && i.ID != other.ID {
		return false
	}

	if i.ExternalID != nil && other.ExternalID != nil && *i.ExternalID != *other.ExternalID {
		return false
	}

	if i.Title != other.Title {
		return false
	}

	if i.Description != other.Description {
		return false
	}

	if !equalStatuses(i.Status, other.Status) {
		return false
	}

	if !equalStringPtr(i.ProjectID, other.ProjectID) {
		return false
	}

	if i.WaitingOn != other.WaitingOn {
		return false
	}

	if !equalTimePtr(i.Snoozed, other.Snoozed) {
		return false
	}

	if !equalTimePtr(i.Due, other.Due) {
		return false
	}

	if !equalTags(i.Tags, other.Tags) {
		return false
	}

	if i.WaitingOn != "" && i.Created.Format("2006-01-02") != other.Created.Format("2006-01-02") {
		return false
	}

	return true
}

// equalStringPtr safely compares two string pointers.
func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	return *a == *b
}

// equalTimePtr safely compares two time pointers.
func equalTimePtr(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	return a.Equal(*b)
}

// equalStatuses safely compares two item statuses, accounting for equivalencies
// between Google Tasks and internal representations.
func equalStatuses(a, b Status) bool {
	if a == StatusOpen && (b == StatusNotStarted || b == StatusInProgress) {
		return true
	}

	if b == StatusOpen && (a == StatusNotStarted || a == StatusInProgress) {
		return true
	}

	return a == b
}

// equalTags safely compares two slices of tags.
func equalTags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for idx, tag := range a {
		if tag != b[idx] {
			return false
		}
	}

	return true
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
