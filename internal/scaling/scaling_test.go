package scaling

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestInProcessBus_EnqueueAndWait(t *testing.T) {
	enq, w := NewInProcessBus(4)
	defer enq.Close()
	defer w.Close()
	job := &Job{JobID: "j-1", WorkflowID: "wf-1"}
	if err := enq.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := w.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got.JobID != "j-1" {
		t.Errorf("JobID = %q, want j-1", got.JobID)
	}
}

func TestInProcessBus_ResultRoundTrip(t *testing.T) {
	enq, w := NewInProcessBus(4)
	defer enq.Close()
	defer w.Close()
	_ = enq.Enqueue(context.Background(), &Job{JobID: "j-2"})
	_, _ = w.Wait(context.Background())
	if err := w.Result(context.Background(), &JobResult{JobID: "j-2", Success: true}); err != nil {
		t.Fatalf("Result: %v", err)
	}
}

func TestInProcessBus_CloseUnblocksWait(t *testing.T) {
	_, w := NewInProcessBus(1)
	w.Close()
	_, err := w.Wait(context.Background())
	if !errors.Is(err, ErrWorkerClosed) {
		t.Errorf("Wait after close error = %v, want ErrWorkerClosed", err)
	}
}

func TestInProcessBus_CloseRejectsEnqueue(t *testing.T) {
	enq, _ := NewInProcessBus(1)
	enq.Close()
	if err := enq.Enqueue(context.Background(), &Job{JobID: "x"}); err == nil {
		t.Fatal("expected error after close")
	}
}

type noopExecutor struct{ calls int32 }

func (e *noopExecutor) Execute(ctx context.Context, job *Job) (*JobResult, error) {
	atomic.AddInt32(&e.calls, 1)
	return &JobResult{JobID: job.JobID, Success: true, StartedAt: time.Now(), EndedAt: time.Now()}, nil
}

func TestRunner_StartStop(t *testing.T) {
	enq, w := NewInProcessBus(8)
	exec := &noopExecutor{}
	r := &Runner{Worker: w, Executor: exec, Concurrency: 2}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for i := 0; i < 5; i++ {
		_ = enq.Enqueue(ctx, &Job{JobID: string(rune('a' + i))})
	}
	// Wait for the workers to drain.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&exec.calls) < 5 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&exec.calls); got != 5 {
		t.Errorf("executor calls = %d, want 5", got)
	}
	cancel()
	r.Stop()
}

func TestRunner_StartValidates(t *testing.T) {
	if err := (&Runner{}).Start(context.Background()); err == nil {
		t.Fatal("expected error on nil Worker")
	}
	w := &inProcessBus{}
	if err := (&Runner{Worker: w}).Start(context.Background()); err == nil {
		t.Fatal("expected error on nil Executor")
	}
}
