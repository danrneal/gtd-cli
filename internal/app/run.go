package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

type Watcher interface {
	Watch(ctx context.Context) (<-chan error, error)
}

type SyncNode struct {
	Name           string
	Syncer         *Syncer
	Watcher        Watcher
	needsPushRetry bool
	needsPullRetry bool
}

type nodeEvent struct {
	idx int
	err error
}

func Run(ctx context.Context, logger *slog.Logger, syncNodes []*SyncNode) error {
	nodeEvents := make(chan nodeEvent)

	for i, syncNode := range syncNodes {
		eventsChan, err := syncNode.Watcher.Watch(ctx)
		if err != nil {
			return fmt.Errorf("failed to start watcher for %s: %w", syncNode.Name, err)
		}

		go func(idx int, nodeChan <-chan error) {
			for {
				select {
				case <-ctx.Done():
					return
				case err, ok := <-nodeChan:
					if !ok {
						err = errors.New("watcher channel closed unexpectedly")
					}

					event := nodeEvent{
						idx: idx,
						err: err,
					}

					select {
					case <-ctx.Done():
						return
					case nodeEvents <- event:
						if !ok {
							return
						}
					}
				}
			}
		}(i, eventsChan)
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("sync loop canceled: %w", ctx.Err())
		case event := <-nodeEvents:
			if event.err != nil {
				nodeName := syncNodes[event.idx].Name
				return fmt.Errorf("fatal error in %s watcher: %w", nodeName, event.err)
			}

			changed := false
			for i, node := range syncNodes {
				if i != event.idx && !node.needsPullRetry {
					continue
				}

				pulled, err := node.Syncer.Pull(ctx)
				if err != nil {
					logger.ErrorContext(ctx, "Failed to pull", "node", node.Name, "err", err)
					node.needsPullRetry = true
				} else {
					node.needsPullRetry = false
					changed = changed || pulled
				}
			}

			for _, node := range syncNodes {
				if node.needsPullRetry {
					continue
				}

				if !changed && !node.needsPushRetry {
					continue
				}

				if err := node.Syncer.Push(ctx); err != nil {
					logger.ErrorContext(ctx, "Failed to push", "node", node.Name, "err", err)
					node.needsPushRetry = true
				} else {
					node.needsPushRetry = false
				}
			}
		}
	}
}
