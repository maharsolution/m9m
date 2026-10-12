// Package scaling implements the task-runner architecture
// for m9m (BUG-ARCH-2026-10-12-002).
//
// n8n v1 splits execution into two runtimes:
//   - The main "main" process owns the schedule, the DB,
//     and the user-facing API.
//   - One or more "worker" processes subscribe to a job
//     queue (Redis / RabbitMQ) and execute the workflow
//     nodes; they have no direct DB connection for the
//     schedule, only for the execution records they
//     produce.
//
// m9m v0.4.0 already supports the queue-mode plumbing
// (Redis / RabbitMQ) in `internal/queue/`. This package
// adds the task-runner abstraction: a `Runner` that
// takes a workflow + input data, executes the work
// off-process, and reports the result back via the
// queue. The "main" process and the "worker" process
// are the same binary; the `Runner` role is selected
// by the `M9M_ROLE` env var (`main` | `worker`).
//
// The package is intentionally minimal: a clean
// interface, a Redis-backed implementation, and a
// stub for the in-process / single-instance case. The
// production rollout is in 0.5.0 — this PR plants the
// interface so a future PR can swap the impl without
// touching the engine.
package scaling

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Job is a unit of work the worker can execute. It
// carries everything the runner needs to reconstruct
// the workflow on the other side of the queue.
type Job struct {
	JobID       string                 `json:"job_id"`
	WorkflowID  string                 `json:"workflow_id"`
	ExecutionID string                 `json:"execution_id"`
	InputData   []byte                 `json:"input_data"` // raw JSON
	Mode        string                 `json:"mode"`        // "webhook" / "schedule" / "retry-node"
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// JobResult is what the worker reports back to the main
// process. The main process persists the result into the
// execution record.
type JobResult struct {
	JobID     string    `json:"job_id"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Output    []byte    `json:"output,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

// Enqueuer is the main-side interface: enqueue a job
// and return immediately. The Redis implementation
// pushes onto a Redis list; the in-process impl drops
// the job onto a channel.
type Enqueuer interface {
	Enqueue(ctx context.Context, job *Job) error
	Close() error
}

// Worker is the worker-side interface: block until a
// job is available, hand it to the executor, and post
// the result back. The Redis implementation uses
// BLPOP; the in-process impl receives from a channel.
type Worker interface {
	// Wait blocks until a job is available or `ctx` is
	// cancelled. Returns ErrWorkerClosed on close.
	Wait(ctx context.Context) (*Job, error)

	// Result posts `result` back to the main process.
	Result(ctx context.Context, result *JobResult) error

	// Close releases resources.
	Close() error
}

// ErrWorkerClosed is returned by Worker.Wait when the
// worker has been closed.
var ErrWorkerClosed = errors.New("scaling: worker closed")

// Executor runs a single job. The engine supplies a
// default that delegates to the workflow engine; tests
// inject a noop or a counter.
type Executor interface {
	Execute(ctx context.Context, job *Job) (*JobResult, error)
}

// Runner ties the three together: a worker + an
// executor, running in a goroutine pool. The pool
// size is `M9M_WORKER_CONCURRENCY` (default 4).
type Runner struct {
	Worker      Worker
	Executor    Executor
	Concurrency int

	wg sync.WaitGroup
}

// Start spins up `Concurrency` worker goroutines. Each
// waits for a job, hands it to the Executor, and posts
// the result back. Returns when `ctx` is cancelled.
func (r *Runner) Start(ctx context.Context) error {
	if r.Worker == nil {
		return errors.New("scaling: Runner.Worker is nil")
	}
	if r.Executor == nil {
		return errors.New("scaling: Runner.Executor is nil")
	}
	if r.Concurrency <= 0 {
		r.Concurrency = 4
	}
	for i := 0; i < r.Concurrency; i++ {
		r.wg.Add(1)
		go r.runOne(ctx)
	}
	return nil
}

// Stop blocks until all worker goroutines exit.
func (r *Runner) Stop() {
	r.wg.Wait()
}

func (r *Runner) runOne(ctx context.Context) {
	defer r.wg.Done()
	for {
		job, err := r.Worker.Wait(ctx)
		if err != nil {
			if errors.Is(err, ErrWorkerClosed) {
				return
			}
			if ctx.Err() != nil {
				return
			}
			// Transient error: back off briefly and retry.
			time.Sleep(500 * time.Millisecond)
			continue
		}
		result, _ := r.Executor.Execute(ctx, job)
		_ = r.Worker.Result(ctx, result)
	}
}

// inProcessBus is a goroutine-channel implementation of
// Enqueuer + Worker. Used in tests and in the
// `M9M_ROLE=main+worker` single-process case.
type inProcessBus struct {
	jobs    chan *Job
	results chan *JobResult
	closed  chan struct{}
	once    sync.Once
}

func NewInProcessBus(buffer int) (Enqueuer, Worker) {
	if buffer <= 0 {
		buffer = 64
	}
	b := &inProcessBus{
		jobs:    make(chan *Job, buffer),
		results: make(chan *JobResult, buffer),
		closed:  make(chan struct{}),
	}
	return b, b
}

func (b *inProcessBus) Enqueue(ctx context.Context, job *Job) error {
	select {
	case <-b.closed:
		return errors.New("scaling: bus closed")
	case b.jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *inProcessBus) Wait(ctx context.Context) (*Job, error) {
	select {
	case <-b.closed:
		return nil, ErrWorkerClosed
	case job := <-b.jobs:
		return job, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *inProcessBus) Result(ctx context.Context, result *JobResult) error {
	select {
	case <-b.closed:
		return errors.New("scaling: bus closed")
	case b.results <- result:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *inProcessBus) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}
