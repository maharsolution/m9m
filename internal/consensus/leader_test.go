package consensus

// Tests for BUG-ARCH-2026-10-12-003 — leader-election
// wrapper.

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRaft struct {
	state  RaftState
	leader string
}

func (f *fakeRaft) State() RaftState { return f.state }
func (f *fakeRaft) Leader() string  { return f.leader }

func TestLeaderElector_IsLeader(t *testing.T) {
	cases := []struct {
		name  string
		state RaftState
		want  bool
	}{
		{"leader", LeaderState, true},
		{"follower", FollowerState, false},
		{"candidate", CandidateState, false},
		{"shutdown", ShutdownState, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			le := NewLeaderElector(&fakeRaft{state: c.state}, "self")
			if got := le.IsLeader(); got != c.want {
				t.Errorf("IsLeader = %v, want %v", got, c.want)
			}
		})
	}
}

func TestLeaderElector_NilSafe(t *testing.T) {
	var le *LeaderElector
	if le.IsLeader() {
		t.Error("nil IsLeader = true")
	}
	if le.LeaderAddress() != "" {
		t.Error("nil LeaderAddress != ''")
	}
}

func TestLeaderElector_LeaderAddress(t *testing.T) {
	le := NewLeaderElector(&fakeRaft{state: LeaderState, leader: "node-2:9000"}, "self")
	if got := le.LeaderAddress(); got != "node-2:9000" {
		t.Errorf("LeaderAddress = %q", got)
	}
}

func TestLeaderElector_WaitForLeader(t *testing.T) {
	r := &fakeRaft{state: LeaderState, leader: "node-3:9000"}
	le := NewLeaderElector(r, "self")
	addr, err := le.WaitForLeader(context.Background())
	if err != nil {
		t.Fatalf("WaitForLeader: %v", err)
	}
	if addr != "node-3:9000" {
		t.Errorf("addr = %q", addr)
	}
}

func TestLeaderElector_WaitForLeader_Cancel(t *testing.T) {
	// No leader ever elected; cancel the context.
	r := &fakeRaft{state: FollowerState, leader: ""}
	le := NewLeaderElector(r, "self")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := le.WaitForLeader(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func TestLeaderElector_ScheduleOnLeader_Runs(t *testing.T) {
	le := NewLeaderElector(&fakeRaft{state: LeaderState}, "self")
	called := false
	ran, err := le.ScheduleOnLeader(context.Background(), func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("ScheduleOnLeader: %v", err)
	}
	if !ran {
		t.Error("ran = false on leader")
	}
	if !called {
		t.Error("fn not called")
	}
}

func TestLeaderElector_ScheduleOnLeader_Skips(t *testing.T) {
	le := NewLeaderElector(&fakeRaft{state: FollowerState}, "self")
	called := false
	ran, err := le.ScheduleOnLeader(context.Background(), func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("ScheduleOnLeader: %v", err)
	}
	if ran {
		t.Error("ran = true on follower")
	}
	if called {
		t.Error("fn called on follower")
	}
}

func TestLeaderElector_ScheduleOnLeader_PropagatesError(t *testing.T) {
	le := NewLeaderElector(&fakeRaft{state: LeaderState}, "self")
	want := errors.New("schedule failure")
	ran, err := le.ScheduleOnLeader(context.Background(), func() error {
		return want
	})
	if !ran || !errors.Is(err, want) {
		t.Errorf("ran = %v, err = %v", ran, err)
	}
}
