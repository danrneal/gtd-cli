package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// List represents a named collection of tasks (Items).
type List struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Position   int       `json:"position"`
	Status     Status    `json:"status"`
	Modified   time.Time `json:"modified"`
	ExternalID *string   `json:"externalId"`
	Items      []*Item   `json:"items"`
}

// GetID returns the internal ID of the list.
func (l *List) GetID() string {
	return l.ID
}

// GetExternalID returns the external ID of the list.
func (l *List) GetExternalID() *string {
	return l.ExternalID
}

// Clean formats the list data into its canonical representation.
func (l *List) Clean() {
	l.Name = strings.TrimSpace(l.Name)
	if l.Status == "" {
		l.Status = StatusOpen
	}
}

// Validate checks if the current state of the list satisfies domain invariants.
func (l *List) Validate() error {
	if l.Name == "" {
		return errors.New("list name cannot be empty")
	}

	switch l.Status {
	case StatusOpen, StatusDeleted:
	default:
		return fmt.Errorf("invalid list status: %q", l.Status)
	}

	if l.Modified.IsZero() {
		return errors.New("list modified timestamp cannot be zero")
	}

	return nil
}

// Equal compares the content of two lists, ignoring metadata like Modified timestamps
// and dynamically applying nuanced checks for primary/external IDs.
func (l *List) Equal(other *List) bool {
	if other == nil {
		return false
	}

	if l.ID != "" && other.ID != "" && l.ID != other.ID {
		return false
	}

	if l.ExternalID != nil && other.ExternalID != nil && *l.ExternalID != *other.ExternalID {
		return false
	}

	if l.Name != other.Name {
		return false
	}

	if l.Status != other.Status {
		return false
	}

	if l.Position != other.Position {
		return false
	}

	if len(l.Items) != len(other.Items) {
		return false
	}

	for i, item := range l.Items {
		otherItem := other.Items[i]

		if item.Position != otherItem.Position {
			return false
		}

		if item.ID != "" && otherItem.ID != "" && item.ID != otherItem.ID {
			return false
		}

		if item.ExternalID != nil && otherItem.ExternalID != nil && *item.ExternalID != *otherItem.ExternalID {
			return false
		}
	}

	return true
}
