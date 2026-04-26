// Package main provides the entry point for the gtd CLI application.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/api/tasks/v1"

	"github.com/danrneal/gtd.nvim/internal/app"
	"github.com/danrneal/gtd.nvim/internal/providers/googletasks"
	"github.com/danrneal/gtd.nvim/internal/providers/markdown"
	"github.com/danrneal/gtd.nvim/internal/providers/sqlite"
)

// Config holds all the command-line flag values required to initialize the application.
type Config struct {
	db                      string
	mdFile                  string
	providers               string
	googleTasksPollInterval int
	credsFile               string
	tokenFile               string
}

func main() {
	var cfg Config
	flag.StringVar(&cfg.db, "db", "gtd.db", "Name of the SQLite database.")
	flag.StringVar(&cfg.mdFile, "filename", "gtd.md", "Path to the GTD Markdown file")
	flag.StringVar(
		&cfg.providers,
		"providers",
		"",
		"Comma-separated list of providers to enable. Supported: google_tasks",
	)
	flag.IntVar(
		&cfg.googleTasksPollInterval,
		"google_tasks_poll_interval",
		30,
		"Google Tasks polling interval in seconds",
	)
	flag.StringVar(&cfg.credsFile, "credentials", "credentials.json", "Path to Google credentials file")
	flag.StringVar(&cfg.tokenFile, "token", "token.json", "Path to Google token file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := run(&cfg, logger); err != nil {
		logger.Error("Application terminated unexpectedly", "err", err)
		os.Exit(1)
	}
}

// run encapsulates the application startup and execution lifecycle. It initializes dependencies,
// wires up the synchronization targets, and blocks until the runner finishes or a fatal error occurs.
func run(cfg *Config, logger *slog.Logger) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	sqliteStore, err := sqlite.NewStore(ctx, cfg.db)
	if err != nil {
		return fmt.Errorf("failed to initialize sqlite store: %w", err)
	}

	var syncTargets []*app.SyncTarget

	mdClient := markdown.NewClient(cfg.mdFile)
	mdSyncer := app.NewSyncer(sqliteStore, mdClient)
	mdSyncTarget := &app.SyncTarget{
		Name:    "markdown",
		Syncer:  mdSyncer,
		Watcher: mdClient,
	}

	syncTargets = append(syncTargets, mdSyncTarget)

	for providerName := range strings.SplitSeq(cfg.providers, ",") {
		if providerName == "" {
			continue
		}

		switch providerName {
		case "google_tasks":
			var tasksService *tasks.Service
			tasksService, err = googletasks.NewService(ctx, cfg.credsFile, cfg.tokenFile)
			if err != nil {
				return fmt.Errorf("failed to initialize google tasks service: %w", err)
			}

			pollInterval := time.Duration(cfg.googleTasksPollInterval) * time.Second
			tasksClient := googletasks.NewClient(tasksService, pollInterval)
			tasksSyncer := app.NewSyncer(sqliteStore, tasksClient)
			tasksSyncTarget := &app.SyncTarget{
				Name:    "google_tasks",
				Syncer:  tasksSyncer,
				Watcher: tasksClient,
			}

			syncTargets = append(syncTargets, tasksSyncTarget)
		default:
			return fmt.Errorf("unsupported providers: %q. Supported providers are: google_tasks", providerName)
		}
	}

	runner := app.NewRunner(logger, syncTargets)
	if err = runner.Run(ctx); err != nil {
		return fmt.Errorf("sync loop failed: %w", err)
	}

	return nil
}
