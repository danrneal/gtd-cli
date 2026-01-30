package googletasks

import "google.golang.org/api/tasks/v1"

// Client is a wrapper around the Google Tasks service.
type Client struct {
	service *tasks.Service
}

// NewClient returns a new Google Tasks client.
func NewClient(service *tasks.Service) *Client {
	client := &Client{service: service}

	return client
}
