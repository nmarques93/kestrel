package checker

import (
	"context"
	"log"
	"sync"
	"time"
)

// Engine runs the scheduler, the worker pool, and the single writer
// goroutine that drains results into storage.
//
// Every second (configurable via TickEvery) the scheduler asks Targets for
// what's due and dispatches each one into an unbuffered jobs channel. That
// channel has no buffer on purpose: if all workers are busy, the send
// blocks, so a spike of due targets can never open more concurrent checks
// than Workers allows. Each worker runs its check under a context timed out
// to that target's own TimeoutMS, so one slow or hanging target can only
// ever cost one worker, never the others. Results flow into a second
// channel drained by exactly one goroutine, so ResultRecorder never sees
// concurrent calls and doesn't need to synchronize its own writes.
type Engine struct {
	Targets  TargetSource
	Prober   Prober
	Recorder ResultRecorder

	// Workers bounds how many checks can be in flight at once. Defaults to
	// 10 if unset.
	Workers int

	// TickEvery controls how often the scheduler looks for due targets.
	// Defaults to one second if unset.
	TickEvery time.Duration
}

// Run blocks, scheduling and executing checks until ctx is canceled. On a
// clean shutdown it waits for in-flight checks to finish and for the writer
// to drain before returning ctx.Err().
func (e *Engine) Run(ctx context.Context) error {
	workers := e.Workers
	if workers <= 0 {
		workers = 10
	}
	tickEvery := e.TickEvery
	if tickEvery <= 0 {
		tickEvery = time.Second
	}

	jobs := make(chan Target)
	results := make(chan Result)

	var workerWG sync.WaitGroup
	for i := 0; i < workers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			e.work(ctx, jobs, results)
		}()
	}

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		e.write(ctx, results)
	}()

	ticker := time.NewTicker(tickEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(jobs)
			workerWG.Wait()
			close(results)
			<-writerDone
			return ctx.Err()

		case now := <-ticker.C:
			due, err := e.Targets.DueTargets(ctx, now)
			if err != nil {
				log.Printf("checker: list due targets: %v", err)
				continue
			}
			for _, target := range due {
				select {
				case jobs <- target:
				case <-ctx.Done():
					// Let the ctx.Done case above handle shutdown.
				}
			}
		}
	}
}

// work is a single worker's loop: pull a target, check it under its own
// timeout, report the result. It never blocks on anything but its own
// current check and the two channels.
func (e *Engine) work(ctx context.Context, jobs <-chan Target, results chan<- Result) {
	for target := range jobs {
		timeout := time.Duration(target.TimeoutMS) * time.Millisecond
		checkCtx, cancel := context.WithTimeout(ctx, timeout)
		result := e.Prober.Probe(checkCtx, target)
		cancel()

		select {
		case results <- result:
		case <-ctx.Done():
			return
		}
	}
}

// write is the single goroutine allowed to call Recorder.Record, so results
// are persisted one at a time regardless of how many workers found them
// concurrently.
func (e *Engine) write(ctx context.Context, results <-chan Result) {
	for result := range results {
		if err := e.Recorder.Record(ctx, result); err != nil {
			log.Printf("checker: record result for target %d: %v", result.TargetID, err)
		}
	}
}
