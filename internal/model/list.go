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

	return nil
}
