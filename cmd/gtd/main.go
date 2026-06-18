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

	"github.com/danrneal/gtd-cli/internal/app"
	"github.com/danrneal/gtd-cli/internal/providers/googletasks"
	"github.com/danrneal/gtd-cli/internal/providers/markdown"
	"github.com/danrneal/gtd-cli/internal/providers/sqlite"
)

// Config holds all the command-line flag values required to initialize the application.
type Config struct {
	system                  string
	db                      string
	mdFile                  string
	providers               string
	googleTasksPollInterval int
	credsFile               string
	tokenFile               string
}

func main() {
	var cfg Config
	flag.StringVar(&cfg.system, "system", "gtd", "Base name for system files (e.g., 'personal' yields 'personal.db')")
	flag.StringVar(&cfg.db, "db", "", "Name of the SQLite database (overrides -system)")
	flag.StringVar(&cfg.mdFile, "filename", "", "Path to the GTD Markdown file (overrides -system)")
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
	flag.StringVar(&cfg.credsFile, "credentials", "", "Path to Google credentials file (overrides -system)")
	flag.StringVar(&cfg.tokenFile, "token", "", "Path to Google token file (overrides -system)")
	flag.Parse()

	if cfg.db == "" {
		cfg.db = cfg.system + ".db"
	}

	if cfg.mdFile == "" {
		cfg.mdFile = cfg.system + ".md"
	}

	if cfg.credsFile == "" {
		cfg.credsFile = cfg.system + "_credentials.json"
	}

	if cfg.tokenFile == "" {
		cfg.tokenFile = cfg.system + "_token.json"
	}

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

	sqliteStore, err := sqlite.NewStore(ctx, cfg.db, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize sqlite store: %w", err)
	}
	defer sqliteStore.Close()

	var syncTargets []*app.SyncTarget

	mdClient := markdown.NewClient(cfg.mdFile, logger)
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
			tasksService, err = googletasks.NewService(ctx, cfg.credsFile, cfg.tokenFile, logger)
			if err != nil {
				return fmt.Errorf("failed to initialize google tasks service: %w", err)
			}

			pollInterval := time.Duration(cfg.googleTasksPollInterval) * time.Second
			tasksClient := googletasks.NewClient(tasksService, pollInterval, logger)
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

	runner := app.NewRunner(syncTargets, logger)
	if err = runner.Run(ctx); err != nil {
		return fmt.Errorf("sync loop failed: %w", err)
	}

	return nil
}
