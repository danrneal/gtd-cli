package model

import "time"

// List represents a named collection of tasks (Items).
type List struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Position   int       `json:"position"`
	Modified   time.Time `json:"modified"`
	ExternalID *string   `json:"external_id"`
	Items      []Item    `json:"items"`
}

// GetExternalID returns the external ID of the list.
func (l List) GetExternalID() *string {
	return l.ExternalID
}
