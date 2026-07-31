package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// mcpTokenPrefix makes a raw token recognizable at a glance, the way
// GitHub's ghp_ prefix does for PATs.
const mcpTokenPrefix = "kestrel_"

// CreateMCPToken generates a new random API token and stores its hash. The
// raw token is returned once, here, and never stored — callers must save
// it immediately, since it can't be recovered later.
func (s *Store) CreateMCPToken(ctx context.Context) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := mcpTokenPrefix + hex.EncodeToString(raw)

	if _, err := s.pool.Exec(ctx, `INSERT INTO mcp_tokens (token_hash) VALUES ($1)`, hashToken(token)); err != nil {
		return "", fmt.Errorf("insert token: %w", err)
	}
	return token, nil
}

// AuthenticateMCPToken reports whether token is a currently issued API
// token. A valid token has its last_used_at touched as a side effect.
func (s *Store) AuthenticateMCPToken(ctx context.Context, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE mcp_tokens SET last_used_at = now() WHERE token_hash = $1
	`, hashToken(token))
	if err != nil {
		return false, fmt.Errorf("authenticate token: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
