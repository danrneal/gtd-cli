package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// Runner orchestrates state synchronization across multiple SyncTargets.
type Runner struct {
	logger  *slog.Logger
	targets []*SyncTarget
	events  chan syncEvent
}

// NewRunner creates a new Runner instance with the provided logger and configuration targets.
func NewRunner(logger *slog.Logger, targets []*SyncTarget) *Runner {
	runner := &Runner{
		logger:  logger,
		targets: targets,
		events:  make(chan syncEvent, len(targets)),
	}

	return runner
}

// SyncTarget groups a Syncer and Watcher together with internal state tracking for retries.
type SyncTarget struct {
	Name           string
	Syncer         *Syncer
	Watcher        Watcher
	needsPushRetry bool
	needsPullRetry bool
}

// Watcher defines the interface for detecting changes in a provider.
type Watcher interface {
	Watch(ctx context.Context) (<-chan error, error)
}

// syncEvent is used internally to pass watcher events or fatal errors to the main loop.
type syncEvent struct {
	target *SyncTarget
	err    error
}

// Run starts the main event loop for the Runner. It initializes watchers for all targets
// and blocks until the context is canceled or a fatal error occurs in a watcher.
func (r *Runner) Run(ctx context.Context) error {
	for _, target := range r.targets {
		if err := r.startWatcher(ctx, target); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("sync loop canceled: %w", ctx.Err())
		case event := <-r.events:
			if event.err != nil {
				return fmt.Errorf("fatal error in %s watcher: %w", event.target.Name, event.err)
			}

			r.syncTargets(ctx, event)
		}
	}
}

// startWatcher initializes and runs a background goroutine that listens for events
// from the given target's Watcher, forwarding them to the runner's event channel.
func (r *Runner) startWatcher(ctx context.Context, target *SyncTarget) error {
	events, err := target.Watcher.Watch(ctx)
	if err != nil {
		return fmt.Errorf("failed to start watcher for %s: %w", target.Name, err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-events:
				if !ok {
					err = errors.New("watcher channel closed unexpectedly")
				}

				event := syncEvent{
					target: target,
					err:    err,
				}

				select {
				case <-ctx.Done():
					return
				case r.events <- event:
					if !ok {
						return
					}
				}
			}
		}
	}()

	return nil
}

// syncTargets orchestrates the pulling and pushing of state across all targets
// in response to an event. It cross-pollinates changes while respecting retry states.
func (r *Runner) syncTargets(ctx context.Context, event syncEvent) {
	changed := false
	for _, target := range r.targets {
		if target != event.target && !target.needsPullRetry {
			continue
		}

		pulled := r.pull(ctx, target)
		changed = changed || pulled
	}

	for _, target := range r.targets {
		if !changed && !target.needsPushRetry {
			continue
		}

		r.push(ctx, target)
	}
}

// pull attempts to synchronize state from the given target to the local store.
// It returns true if changes were successfully pulled, and sets retry flags on failure.
func (r *Runner) pull(ctx context.Context, target *SyncTarget) bool {
	pulled, err := target.Syncer.Pull(ctx)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to pull", "syncTarget", target.Name, "err", err)
		target.needsPullRetry = true

		return false
	}

	target.needsPullRetry = false

	return pulled
}

// push attempts to synchronize state from the local store to the given target.
// It avoids pushing if the target needs a pull retry, and updates retry flags appropriately.
func (r *Runner) push(ctx context.Context, target *SyncTarget) {
	if target.needsPullRetry {
		return
	}

	if err := target.Syncer.Push(ctx); err != nil {
		r.logger.ErrorContext(ctx, "Failed to push", "syncTarget", target.Name, "err", err)
		target.needsPushRetry = true

		return
	}

	target.needsPushRetry = false
}
