// Command migrate applies Kestrel's database migrations. It embeds the
// migration files so the same binary can run them in any environment
// (local dev, CI, or the deployed container) without a separate goose
// install.
package main

import (
	"database/sql"
	"flag"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/nmarques93/kestrel/internal/config"
	"github.com/nmarques93/kestrel/migrations"
)

func main() {
	direction := flag.String("direction", "up", "migration direction: up or down")
	flag.Parse()

	cfg := config.Load()

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("set dialect: %v", err)
	}

	switch *direction {
	case "up":
		if err := goose.Up(db, "."); err != nil {
			log.Fatalf("migrate up: %v", err)
		}
	case "down":
		if err := goose.Down(db, "."); err != nil {
			log.Fatalf("migrate down: %v", err)
		}
	default:
		log.Fatalf("unknown direction %q (want up or down)", *direction)
	}

	log.Printf("migrate %s: done", *direction)
}
