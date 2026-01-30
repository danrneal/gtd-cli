package main

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/danrneal/gtd.nvim/internal/adapters/googletasks"
	"github.com/danrneal/gtd.nvim/internal/sqlite"
)

var (
	db        = flag.String("db", "gtd.db", "Name of the SQLite database.")
	adapters  = flag.String("adapters", "", "Comma-separated list of adapters to enable. Supported: google_tasks")
	credsFile = flag.String("credentials", "credentials.json", "Path to Google credentials file")
	tokenFile = flag.String("token", "token.json", "Path to Google token file")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	sqliteStore, err := sqlite.NewStore(ctx, *db)
	if err != nil {
		log.Fatal(err)
	}

	for _, adapter := range strings.Split(*adapters, ",") {
		if adapter == "" {
			continue
		}

		switch adapter {
		case "google_tasks":
			tasksService, err := googletasks.NewService(ctx, *credsFile, *tokenFile)
			if err != nil {
				log.Fatal(err)
			}

			tasksClient := googletasks.NewClient(tasksService)
			_ = tasksClient
		default:
			log.Fatalf("unsupported adapter: %q. Supported adapters are: google_tasks", adapter)
		}
	}

	_ = sqliteStore
}
