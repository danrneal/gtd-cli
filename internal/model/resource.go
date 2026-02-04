package model

// Resource is an interface for objects that have an external ID.
type Resource interface {
	// GetExternalID returns the external ID of the resource.
	GetExternalID() *string
}
