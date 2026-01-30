package model

import "time"

// Item represents a single task or action item within a List.
type Item struct {
	ID          int64      `json:"id"`
	ListID      int64      `json:"list_id"`
	Position    int        `json:"position"`
	Completed   bool       `json:"completed"`
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
