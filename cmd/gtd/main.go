package main

import (
	"context"
	"flag"
	"log"

	"github.com/danrneal/gtd.nvim/internal/sqlite"
)

var db = flag.String("db", "gtd.db", "Name of the SQLite database.")

func main() {
	flag.Parse()
	ctx := context.Background()

	sqliteStore, err := sqlite.NewStore(ctx, *db)
	if err != nil {
		log.Fatal(err)
	}

	_ = sqliteStore
}
