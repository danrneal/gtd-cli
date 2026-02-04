package model

import "time"

// Status represents the lifecycle state of an item.
type Status string

const (
	// StatusOpen indicates the item is active (either not started or in progress).
	StatusOpen Status = "open"
	// StatusNotStarted indicates the item has not been started yet.
	StatusNotStarted Status = "not_started"
	// StatusInProgress indicates the item is currently being worked on.
	StatusInProgress Status = "in_progress"
	// StatusDone indicates the item has been completed.
	StatusDone Status = "done"
	// StatusDeleted indicates the item has been deleted.
	StatusDeleted Status = "deleted"
)

// Item represents a single task or action item within a List.
type Item struct {
	ID             string     `json:"id"`
	ListID         string     `json:"list_id"`
	Position       int        `json:"position"`
	Status         Status     `json:"status"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	ProjectID      *string    `json:"project_id"`
	WaitingOn      *string    `json:"waiting_on"`
	Snoozed        *time.Time `json:"snoozed"`
	Due            *time.Time `json:"due"`
	Tags           []string   `json:"tags"`
	Modified       time.Time  `json:"modified"`
	Created        time.Time  `json:"created"`
	ExternalID     *string    `json:"external_id"`
	ExternalListID *string    `json:"external_list_id"`
}

// GetExternalID returns the external ID of the item.
func (i Item) GetExternalID() *string {
	return i.ExternalID
}
