package model

// Status represents the lifecycle state of a resource.
type Status string

const (
	// StatusOpen indicates the resource is active.
	StatusOpen Status = "open"
	// StatusNotStarted indicates the item has not been started yet.
	StatusNotStarted Status = "not_started"
	// StatusInProgress indicates the item is currently being worked on.
	StatusInProgress Status = "in_progress"
	// StatusDone indicates the item has been completed.
	StatusDone Status = "done"
	// StatusDeleted indicates the resource has been deleted.
	StatusDeleted Status = "deleted"
)

// Resource is an interface for objects that have an external ID.
type Resource interface {
	// GetExternalID returns the external ID of the resource.
	GetExternalID() *string
}
