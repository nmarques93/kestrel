package checker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPProber checks a target with an HTTP GET, judging success by whether
// the response status code falls in the target's expected range.
type HTTPProber struct {
	Client *http.Client
}

// NewHTTPProber returns an HTTPProber with a default client. The client has
// no timeout of its own — the per-check timeout comes from the context
// Probe is called with, so one slow target can never hold a worker longer
// than its own configured timeout.
func NewHTTPProber() *HTTPProber {
	return &HTTPProber{Client: &http.Client{}}
}

func (p *HTTPProber) Probe(ctx context.Context, target Target) Result {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return Result{TargetID: target.ID, CheckedAt: start, Success: false, Err: err.Error()}
	}

	resp, err := p.Client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return Result{
			TargetID:  target.ID,
			CheckedAt: start,
			Success:   false,
			LatencyMS: latency.Milliseconds(),
			Err:       err.Error(),
		}
	}
	defer resp.Body.Close()
	// Drain the body so the connection can be reused by the pool.
	_, _ = io.Copy(io.Discard, resp.Body)

	success := int32(resp.StatusCode) >= target.ExpectedStatusMin && int32(resp.StatusCode) < target.ExpectedStatusMax

	result := Result{
		TargetID:   target.ID,
		CheckedAt:  start,
		Success:    success,
		StatusCode: resp.StatusCode,
		LatencyMS:  latency.Milliseconds(),
	}
	if !success {
		result.Err = fmt.Sprintf("unexpected status code %d (want %d-%d)", resp.StatusCode, target.ExpectedStatusMin, target.ExpectedStatusMax-1)
	}
	return result
}
