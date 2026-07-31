// Package config loads Kestrel's runtime configuration from the environment.
package config

import "os"

// Config holds settings needed to start the service. It grows as later
// milestones add the MCP layer.
type Config struct {
	DatabaseURL string
	HTTPAddr    string
	// WebhookURL is where DOWN/recovery notifications are POSTed. Empty
	// disables webhook notifications entirely.
	WebhookURL string
}

// Load reads configuration from environment variables, applying local
// development defaults where it's safe to do so.
func Load() Config {
	return Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://kestrel:kestrel@localhost:55432/kestrel?sslmode=disable"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":8080"),
		WebhookURL:  getEnv("WEBHOOK_URL", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
