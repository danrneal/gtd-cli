package googletasks

import (
	"context"
	"time"
)

// Watch starts a background goroutine that polls the Google Tasks API on a configured interval.
// It returns a channel that emits nil errors to trigger synchronization, or fatal errors if the watcher crashes.
func (c *Client) Watch(ctx context.Context) (<-chan error, error) {
	events := make(chan error, 1)

	go func() {
		defer close(events)

		ticker := time.NewTicker(c.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				select {
				case events <- nil:
					// Successfully sent the event
				default:
					// Non-blocking send: drop duplicate polling ping if the channel is unread
				}
			}
		}
	}()

	return events, nil
}
