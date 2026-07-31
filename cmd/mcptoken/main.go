// Command mcptoken issues a new API token for the MCP server. The raw
// token is printed once and never stored — save it immediately.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nmarques93/kestrel/internal/config"
	"github.com/nmarques93/kestrel/internal/database"
	"github.com/nmarques93/kestrel/internal/store"
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

	token, err := store.New(pool).CreateMCPToken(ctx)
	if err != nil {
		log.Fatalf("create token: %v", err)
	}

	fmt.Println(token)
	fmt.Println()
	fmt.Println("Save this token now — it will not be shown again.")
	fmt.Println("Use it as: Authorization: Bearer <token> against POST /mcp")
}
