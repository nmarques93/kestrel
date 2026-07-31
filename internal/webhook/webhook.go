// Package webhook delivers incident notifications to a single configured
// URL, retrying failed deliveries with exponential backoff.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Event is the JSON payload POSTed on a DOWN or recovery transition.
type Event struct {
	Type       string     `json:"type"` // "down" or "up"
	TargetID   int64      `json:"target_id"`
	TargetName string     `json:"target_name"`
	TargetURL  string     `json:"target_url"`
	IncidentID int64      `json:"incident_id"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Cause      *string    `json:"cause,omitempty"`
}

// Notifier delivers a single event. Implementations are called from a
// background goroutine, so a slow or failing delivery never blocks the
// checker engine's result writer.
type Notifier interface {
	Notify(ctx context.Context, event Event) error
}

// Sender POSTs events to a fixed URL, retrying with exponential backoff on
// failure (a non-2xx response or a transport error).
type Sender struct {
	URL         string
	Client      *http.Client
	MaxAttempts int           // default 4 (1 initial try + 3 retries)
	BaseDelay   time.Duration // default 1s, doubling each retry
}

var _ Notifier = (*Sender)(nil)

func (s *Sender) Notify(ctx context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal webhook event: %w", err)
	}

	maxAttempts := s.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 4
	}
	delay := s.BaseDelay
	if delay <= 0 {
		delay = time.Second
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return fmt.Errorf("webhook delivery cancelled after %d attempt(s): %w", attempt-1, ctx.Err())
			}
			delay *= 2
		}

		lastErr = deliver(ctx, client, s.URL, body)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("webhook delivery failed after %d attempts: %w", maxAttempts, lastErr)
}

func deliver(ctx context.Context, client *http.Client, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
