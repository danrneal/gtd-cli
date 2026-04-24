package markdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

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

func (c *Client) watchLoop(ctx context.Context, watcher *fsnotify.Watcher, events chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if !c.hasFileChanged(event) {
				continue
			}

			select {
			case events <- nil:
				// Successfully sent the event
			default:
				// Non-blocking send: drop duplicate burst events if the channel is unread
			}
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
	changed := stat.ModTime().After(c.lastModTime)
	c.mu.RUnlock()

	if !changed {
		return false
	}

	c.mu.Lock()
	c.lastModTime = stat.ModTime()
	c.mu.Unlock()

	return true
}
