package markdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultDebounceInterval defines how long the file watcher will wait for file system events
// to settle before triggering a sync. This prevents race conditions during editor saves.
const DefaultDebounceInterval = 50 * time.Millisecond

// Watch initializes an fsnotify file watcher on the directory containing the markdown file.
// It returns a channel that emits events when genuine user modifications are detected.
func (c *Client) Watch(ctx context.Context) (<-chan error, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	dir := filepath.Dir(c.filepath)

	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("failed to watch directory %s: %w", dir, err)
	}

	events := make(chan error, 1)

	go func() {
		defer watcher.Close()
		defer close(events)

		c.watchLoop(ctx, watcher, events)
	}()

	return events, nil
}

// watchLoop runs continuously in the background, routing OS events from fsnotify,
// filtering out noise, and pushing valid events down the outbound channel.
func (c *Client) watchLoop(ctx context.Context, watcher *fsnotify.Watcher, events chan<- error) {
	var (
		timer   *time.Timer
		timeout <-chan time.Time
	)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			timer, timeout = c.processEvent(timer, event)
		case <-timeout:
			select {
			case events <- nil:
				// Successfully sent the event
			default:
				// Non-blocking send: drop duplicate burst events if the channel is unread
			}

			timer, timeout = nil, nil
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}

			select {
			case <-ctx.Done():
				return
			case events <- fmt.Errorf("markdown file watcher error: %w", err):
				// Successfully sent the error
			}
		}
	}
}

// processEvent manages the debounce timer state when a valid file modification occurs.
// It safely stops any active timer, drains its channel if necessary, and returns
// a new timer initialized with the debounce interval.
func (c *Client) processEvent(timer *time.Timer, event fsnotify.Event) (*time.Timer, <-chan time.Time) {
	if !c.hasFileChanged(event) {
		if timer == nil {
			return nil, nil
		}

		return timer, timer.C
	}

	if timer != nil {
		if !timer.Stop() {
			<-timer.C
		}

		timer.Reset(DefaultDebounceInterval)

		return timer, timer.C
	}

	timer = time.NewTimer(DefaultDebounceInterval)

	return timer, timer.C
}

// hasFileChanged determines if an fsnotify event represents a genuine user modification
// by verifying the file path, the operation type, and asserting that the file's
// physical modification timestamp has advanced past the Client's last known state.
func (c *Client) hasFileChanged(event fsnotify.Event) bool {
	if filepath.Clean(event.Name) != filepath.Clean(c.filepath) {
		return false
	}

	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Rename) {
		return false
	}

	stat, err := os.Stat(c.filepath)
	if err != nil {
		return false
	}

	c.mu.RLock()
	changed := !stat.ModTime().Before(c.lastModTime)
	c.mu.RUnlock()

	if !changed {
		return false
	}

	c.mu.Lock()
	c.lastModTime = stat.ModTime()
	c.mu.Unlock()

	return true
}
