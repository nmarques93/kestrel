// Command kestrel runs the uptime monitor: the checking engine and the
// HTTP API/status page, side by side in one process.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nmarques93/kestrel/internal/api"
	"github.com/nmarques93/kestrel/internal/checker"
	"github.com/nmarques93/kestrel/internal/config"
	"github.com/nmarques93/kestrel/internal/database"
	"github.com/nmarques93/kestrel/internal/store"
	"github.com/nmarques93/kestrel/internal/webhook"
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
	// which both the engine and the HTTP server treat as a shutdown
	// request rather than an error.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := store.New(pool)
	if cfg.WebhookURL != "" {
		s.SetNotifier(&webhook.Sender{URL: cfg.WebhookURL, Client: &http.Client{Timeout: 10 * time.Second}})
		log.Printf("webhook notifications enabled: %s", cfg.WebhookURL)
	}
	engine := &checker.Engine{
		Targets:  s,
		Prober:   checker.NewHTTPProber(),
		Recorder: s,
		Workers:  10,
	}
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: api.NewServer(s),
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		log.Println("checker engine started")
		if err := engine.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("checker engine: %v", err)
		}
		log.Println("checker engine stopped")
	}()

	go func() {
		defer wg.Done()
		log.Printf("http server listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("http server shutdown: %v", err)
	}

	wg.Wait()
	log.Println("kestrel stopped")
}
