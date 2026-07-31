package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSenderSucceedsFirstAttempt(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)

		var got Event
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if got.Type != "down" || got.TargetName != "example" {
			t.Errorf("got event %+v, want type=down target_name=example", got)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := &Sender{URL: server.URL, BaseDelay: time.Millisecond}
	err := sender.Notify(context.Background(), Event{Type: "down", TargetName: "example"})
	if err != nil {
		t.Fatalf("Notify() error = %v, want nil", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("server called %d times, want exactly 1", calls)
	}
}

func TestSenderRetriesThenSucceeds(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := &Sender{URL: server.URL, BaseDelay: time.Millisecond, MaxAttempts: 5}
	err := sender.Notify(context.Background(), Event{Type: "up"})
	if err != nil {
		t.Fatalf("Notify() error = %v, want nil (should have succeeded on 3rd attempt)", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("server called %d times, want exactly 3", calls)
	}
}

func TestSenderGivesUpAfterMaxAttempts(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	sender := &Sender{URL: server.URL, BaseDelay: time.Millisecond, MaxAttempts: 3}
	err := sender.Notify(context.Background(), Event{Type: "down"})
	if err == nil {
		t.Fatal("Notify() error = nil, want an error after exhausting all attempts")
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("server called %d times, want exactly 3 (MaxAttempts)", calls)
	}
}

func TestSenderBackoffDoubles(t *testing.T) {
	var timestamps []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timestamps = append(timestamps, time.Now())
		if len(timestamps) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := &Sender{URL: server.URL, BaseDelay: 20 * time.Millisecond, MaxAttempts: 5}
	if err := sender.Notify(context.Background(), Event{}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if len(timestamps) != 3 {
		t.Fatalf("got %d attempts, want 3", len(timestamps))
	}

	firstGap := timestamps[1].Sub(timestamps[0])
	secondGap := timestamps[2].Sub(timestamps[1])
	// secondGap should be roughly double firstGap (20ms then 40ms); allow
	// generous slack for scheduler jitter in CI.
	if secondGap < firstGap+10*time.Millisecond {
		t.Errorf("backoff didn't grow: first gap %v, second gap %v", firstGap, secondGap)
	}
}

func TestSenderRespectsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	sender := &Sender{URL: server.URL, BaseDelay: time.Hour, MaxAttempts: 5} // huge delay: only cancellation should stop it

	done := make(chan error, 1)
	go func() { done <- sender.Notify(ctx, Event{}) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Notify() error = nil, want an error after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Notify did not return promptly after context cancellation")
	}
}

func TestSenderConnectionErrorIsRetried(t *testing.T) {
	sender := &Sender{URL: "http://127.0.0.1:1/", BaseDelay: time.Millisecond, MaxAttempts: 2}
	err := sender.Notify(context.Background(), Event{})
	if err == nil {
		t.Fatal("Notify() error = nil, want an error for a refused connection")
	}
}
