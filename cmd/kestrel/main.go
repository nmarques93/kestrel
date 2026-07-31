// Command kestrel runs the uptime monitor's checking engine.
package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/nmarques93/kestrel/internal/checker"
	"github.com/nmarques93/kestrel/internal/config"
	"github.com/nmarques93/kestrel/internal/database"
	"github.com/nmarques93/kestrel/internal/store"
)

func main() {
	cfg := config.Load()

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConnect()

	pool, err := database.NewPool(connectCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	// Ctrl+C or a SIGTERM (e.g. from `docker stop`) cancels this context,
	// which the engine treats as a request to finish in-flight checks and
	// shut down rather than an error.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := store.New(pool)
	engine := &checker.Engine{
		Targets:  s,
		Prober:   checker.NewHTTPProber(),
		Recorder: s,
		Workers:  10,
	}

	log.Println("checker engine started")
	if err := engine.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("checker engine: %v", err)
	}
	log.Println("checker engine stopped")
}
