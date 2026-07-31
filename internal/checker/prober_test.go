package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProberSuccessWithinExpectedRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	prober := NewHTTPProber()
	target := Target{ID: 1, URL: server.URL, ExpectedStatusMin: 200, ExpectedStatusMax: 300, TimeoutMS: 1000}

	result := prober.Probe(context.Background(), target)

	if !result.Success {
		t.Errorf("Success = false, want true (err: %q)", result.Err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
}

func TestHTTPProberStatusOutsideExpectedRangeIsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	prober := NewHTTPProber()
	target := Target{ID: 1, URL: server.URL, ExpectedStatusMin: 200, ExpectedStatusMax: 300, TimeoutMS: 1000}

	result := prober.Probe(context.Background(), target)

	if result.Success {
		t.Error("Success = true, want false for a 500 response")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", result.StatusCode)
	}
	if result.Err == "" {
		t.Error("Err is empty, want an explanation of the unexpected status code")
	}
}

func TestHTTPProberRespectsContextTimeout(t *testing.T) {
	unblock := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-unblock
	}))
	defer func() {
		close(unblock)
		server.Close()
	}()

	prober := NewHTTPProber()
	target := Target{ID: 1, URL: server.URL, ExpectedStatusMin: 200, ExpectedStatusMax: 300}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := prober.Probe(ctx, target)
	elapsed := time.Since(start)

	if result.Success {
		t.Error("Success = true, want false when the request times out")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Probe took %v, want it to return promptly after the 20ms context timeout", elapsed)
	}
}

func TestHTTPProberConnectionErrorIsFailure(t *testing.T) {
	prober := NewHTTPProber()
	// Nothing listens here; the connection should be refused immediately.
	target := Target{ID: 1, URL: "http://127.0.0.1:1", ExpectedStatusMin: 200, ExpectedStatusMax: 300, TimeoutMS: 1000}

	result := prober.Probe(context.Background(), target)

	if result.Success {
		t.Error("Success = true, want false for a connection error")
	}
	if result.Err == "" {
		t.Error("Err is empty, want a connection error message")
	}
}
