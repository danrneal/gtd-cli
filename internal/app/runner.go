package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// Runner orchestrates state synchronization across multiple SyncNodes.
type Runner struct {
	logger     *slog.Logger
	syncNodes  []*SyncNode
	nodeEvents chan nodeEvent
}

// NewRunner creates a new Runner instance with the provided logger and configuration nodes.
func NewRunner(logger *slog.Logger, syncNodes []*SyncNode) *Runner {
	runner := &Runner{
		logger:     logger,
		syncNodes:  syncNodes,
		nodeEvents: make(chan nodeEvent),
	}

	return runner
}

// SyncNode groups a Syncer and Watcher together with internal state tracking for retries.
type SyncNode struct {
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

// nodeEvent is used internally to pass watcher events or fatal errors to the main loop.
type nodeEvent struct {
	node *SyncNode
	err  error
}

func (r *Runner) Run(ctx context.Context) error {
	for _, syncNode := range r.syncNodes {
		if err := r.startWatcher(ctx, syncNode); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("sync loop canceled: %w", ctx.Err())
		case event := <-r.nodeEvents:
			if event.err != nil {
				return fmt.Errorf("fatal error in %s watcher: %w", event.node.Name, event.err)
			}

			changed := false
			for _, syncNode := range r.syncNodes {
				if syncNode != event.node && !syncNode.needsPullRetry {
					continue
				}

				pulled := r.pull(ctx, syncNode)
				changed = changed || pulled
			}

			for _, syncNode := range r.syncNodes {
				if !changed && !syncNode.needsPushRetry {
					continue
				}

				r.push(ctx, syncNode)
			}
		}
	}
}

func (r *Runner) startWatcher(ctx context.Context, syncNode *SyncNode) error {
	eventsChan, err := syncNode.Watcher.Watch(ctx)
	if err != nil {
		return fmt.Errorf("failed to start watcher for %s: %w", syncNode.Name, err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-eventsChan:
				if !ok {
					err = errors.New("watcher channel closed unexpectedly")
				}

				event := nodeEvent{
					node: syncNode,
					err:  err,
				}

				select {
				case <-ctx.Done():
					return
				case r.nodeEvents <- event:
					if !ok {
						return
					}
				}
			}
		}
	}()

	return nil
}

func (r *Runner) pull(ctx context.Context, syncNode *SyncNode) bool {
	pulled, err := syncNode.Syncer.Pull(ctx)
	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to pull", "syncNode", syncNode.Name, "err", err)
		syncNode.needsPullRetry = true

		return false
	}

	syncNode.needsPullRetry = false

	return pulled
}

func (r *Runner) push(ctx context.Context, syncNode *SyncNode) {
	if syncNode.needsPullRetry {
		return
	}

	if err := syncNode.Syncer.Push(ctx); err != nil {
		r.logger.ErrorContext(ctx, "Failed to push", "syncNode", syncNode.Name, "err", err)
		syncNode.needsPushRetry = true

		return
	}

	syncNode.needsPushRetry = false
}
