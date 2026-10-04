// Command migrate applies or rolls back database migrations.
//
//	migrate up     apply all pending migrations
//	migrate down   roll back all migrations (destroys data)
package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"fogline/api/internal/platform/database"
)

func main() {
	_ = godotenv.Load()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate up|down")
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "up":
		err = database.MigrateUp(url)
	case "down":
		err = database.MigrateDown(url)
	default:
		fmt.Fprintln(os.Stderr, "usage: migrate up|down")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	fmt.Println("migrate", os.Args[1], "ok")
}
