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

// Resource is an interface for domain objects (like Lists and Items) that can be
// identified and linked across different providers via an External ID.
type Resource interface {
	GetExternalID() *string
	SetExternalID(string)
}
