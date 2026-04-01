// Package model contains the domain structures for the gtd application.
package model

import "time"

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
