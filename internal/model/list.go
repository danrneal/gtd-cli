package model

import "time"

// List represents a named collection of tasks (Items).
type List struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Position   int       `json:"position"`
	Status     Status    `json:"status"`
	Modified   time.Time `json:"modified"`
	ExternalID *string   `json:"external_id"`
	Items      []Item    `json:"items"`
}

// GetID returns the internal ID of the list.
func (l *List) GetID() string {
	return l.ID
}

// SetID sets the internal ID of the list.
func (l *List) SetID(id string) {
	l.ID = id
}

// GetExternalID returns the external ID of the list.
func (l *List) GetExternalID() *string {
	return l.ExternalID
}

// SetExternalID sets the external ID of the list.
func (l *List) SetExternalID(externalID string) {
	l.ExternalID = &externalID
}
