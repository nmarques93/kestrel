// Command kestrel runs the uptime monitor service.
package main

import (
	"context"
	"log"
	"time"

	"github.com/nmarques93/kestrel/internal/config"
	"github.com/nmarques93/kestrel/internal/database"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	log.Println("connected to database, exiting")
}
