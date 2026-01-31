package model

import "time"

type Status string

const (
	StatusOpen       Status = "open"
	StatusNotStarted Status = "not_started"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusDeleted    Status = "deleted"
)

// Item represents a single task or action item within a List.
type Item struct {
	ID          int64      `json:"id"`
	ListID      int64      `json:"list_id"`
	Position    int        `json:"position"`
	Status      Status     `json:"status"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ProjectID   *string    `json:"project_id"`
	WaitingOn   *string    `json:"waiting_on"`
	Snoozed     *time.Time `json:"snoozed"`
	Due         *time.Time `json:"due"`
	Tags        []string   `json:"tags"`
	Modified    time.Time  `json:"modified"`
	Created     time.Time  `json:"created"`
	ExternalID  *string    `json:"external_id"`
}
