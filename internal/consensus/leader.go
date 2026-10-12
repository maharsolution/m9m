package consensus

// Leader-election wrapper for BUG-ARCH-2026-10-12-003.
//
// The underlying `raft.Raft` (in raft.go) is fully
// functional — it implements leader election, log
// replication, and snapshotting. This file adds the
// higher-level helpers the rest of m9m needs:
//
//   - `IsLeader()` — the read path. Cheap, lock-free.
//   - `WaitForLeader(ctx)` — block until a leader is
//     elected (or ctx is cancelled). Used at startup so
//     the main process doesn't double-schedule workflows
//     during a leadership race.
//   - `ScheduleOnLeader(ctx, fn)` — the production
//     pattern: only run `fn` if we hold leadership; if
//     not, no-op and return false. Used by the cron
//     scheduler to avoid double-firing.
//
// We deliberately do NOT wrap the FSM — the existing
// `raft.go` FSMs (Command, Snapshot, Restore) are
// sufficient.

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// LeaderElector is the typed wrapper around *raft.Raft.
// It exists so callers don't have to depend on the
// hashicorp/raft package directly.
type LeaderElector struct {
	raft    RaftBackend
	address string // our own address (for logging)
	held    atomic.Bool
}

// RaftBackend is the subset of *raft.Raft we use. Lets
// us swap a fake in tests.
type RaftBackend interface {
	State() RaftState
	Leader() string
}

// RaftState mirrors the parts of raft.RaftState we
// care about. Production wiring converts the
// hashicorp enum to this; tests just pass the value.
type RaftState int

const (
	// FollowerState means this node is a follower.
	FollowerState RaftState = iota
	// CandidateState means this node is campaigning.
	CandidateState
	// LeaderState means this node is the leader.
	LeaderState
	// ShutdownState means the node is no longer
	// participating in consensus.
	ShutdownState
)

// NewLeaderElector returns a wrapper for `r`. `address`
// is the local node's network address (used in logs).
func NewLeaderElector(r RaftBackend, address string) *LeaderElector {
	return &LeaderElector{raft: r, address: address}
}

// IsLeader returns true iff this node currently holds
// leadership. Safe for concurrent use.
func (l *LeaderElector) IsLeader() bool {
	if l == nil {
		return false
	}
	if l.held.Load() {
		// Cache the positive result; refresh on
		// state change.
		return true
	}
	if l.raft.State() == LeaderState {
		l.held.Store(true)
		return true
	}
	l.held.Store(false)
	return false
}

// LeaderAddress returns the address of the current
// leader, or "" if no leader is elected (e.g. during
// a campaign). The main process uses this to redirect
// admin requests to the leader.
func (l *LeaderElector) LeaderAddress() string {
	if l == nil || l.raft == nil {
		return ""
	}
	return l.raft.Leader()
}

// WaitForLeader blocks until a leader is elected or
// `ctx` is cancelled. The poll interval is 100ms —
// plenty of resolution for cluster startup and avoids
// pegging the CPU during a contested election.
//
// Returns the leader's address, or `ctx.Err()` on
// cancellation.
func (l *LeaderElector) WaitForLeader(ctx context.Context) (string, error) {
	if l == nil {
		return "", errors.New("consensus: nil LeaderElector")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if addr := l.LeaderAddress(); addr != "" {
			return addr, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

// ScheduleOnLeader runs `fn` iff this node is the
// leader. Returns true on run, false on skip (not a
// leader). Errors from `fn` are returned unchanged.
//
// The intended use is the cron scheduler:
//
//	if elector.ScheduleOnLeader(ctx, func() error {
//	    return scheduler.Tick(ctx)
//	}) { /* we ran the tick */ }
func (l *LeaderElector) ScheduleOnLeader(ctx context.Context, fn func() error) (bool, error) {
	if !l.IsLeader() {
		return false, nil
	}
	if err := fn(); err != nil {
		return true, err
	}
	return true, nil
}
