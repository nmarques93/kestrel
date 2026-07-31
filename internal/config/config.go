// Package config loads Kestrel's runtime configuration from the environment.
package config

import "os"

// Config holds settings needed to start the service. It grows as later
// milestones add the webhook and MCP layers.
type Config struct {
	DatabaseURL string
	HTTPAddr    string
}

// Load reads configuration from environment variables, applying local
// development defaults where it's safe to do so.
func Load() Config {
	return Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://kestrel:kestrel@localhost:55432/kestrel?sslmode=disable"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
