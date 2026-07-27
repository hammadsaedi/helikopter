package main

import (
	"errors"
	"testing"

	"github.com/hammadsaedi/helikopter/internal/awake"
)

type fakeLock struct {
	released *int
}

func (f fakeLock) Method() string { return "fake" }
func (f fakeLock) Release()       { *f.released++ }

func TestSetAwakeTakesAndReleasesTheLock(t *testing.T) {
	released, acquired := 0, 0
	s := &state{acquire: func(awake.Options) (awake.Lock, error) {
		acquired++
		return fakeLock{&released}, nil
	}}

	s.setAwake(true)
	if !s.wantAwake || s.lock == nil || acquired != 1 {
		t.Fatalf("expected a held lock: want=%v lock=%v acquired=%d", s.wantAwake, s.lock, acquired)
	}
	if s.awakeNote != "on" {
		t.Errorf("awakeNote = %q, want \"on\"", s.awakeNote)
	}

	// Letting the machine sleep has to actually release it; leaving it held
	// while reporting "off" would be a lie.
	s.setAwake(false)
	if s.wantAwake || s.lock != nil {
		t.Fatal("lock should be gone")
	}
	if released != 1 {
		t.Errorf("Release called %d times, want 1", released)
	}
	if s.awakeNote != "off" {
		t.Errorf("awakeNote = %q, want \"off\"", s.awakeNote)
	}
}

func TestToggleAwakeGoesBothWays(t *testing.T) {
	released, acquired := 0, 0
	s := &state{acquire: func(awake.Options) (awake.Lock, error) {
		acquired++
		return fakeLock{&released}, nil
	}}

	s.setAwake(true)
	for i := 0; i < 3; i++ {
		s.toggleAwake()
		if s.wantAwake {
			t.Fatalf("round %d: expected released", i)
		}
		s.toggleAwake()
		if !s.wantAwake || s.lock == nil {
			t.Fatalf("round %d: expected held again", i)
		}
	}
	if acquired != 4 {
		t.Errorf("acquired %d times, want 4", acquired)
	}
	if released != 3 {
		t.Errorf("released %d times, want 3", released)
	}
}

// Toggling on must not leak the previous lock.
func TestSetAwakeTwiceReleasesTheOldLock(t *testing.T) {
	released := 0
	s := &state{acquire: func(awake.Options) (awake.Lock, error) {
		return fakeLock{&released}, nil
	}}
	s.setAwake(true)
	s.setAwake(true)
	if released != 1 {
		t.Errorf("the first lock should have been released, got %d releases", released)
	}
	s.releaseAwake()
	if released != 2 {
		t.Errorf("releaseAwake should drop the held lock, got %d releases", released)
	}
}

func TestSetAwakeReportsWhenItCannotHold(t *testing.T) {
	s := &state{acquire: func(awake.Options) (awake.Lock, error) {
		return nil, errors.New("no mechanism")
	}}
	s.setAwake(true)
	if s.wantAwake {
		t.Error("wantAwake should be false when the lock could not be taken")
	}
	if s.lock != nil {
		t.Error("no lock should be recorded")
	}
	if s.awakeNote != "unavailable" {
		t.Errorf("awakeNote = %q, want \"unavailable\"", s.awakeNote)
	}
}

// --no-awake starts without the lock, and w can still turn it on afterwards.
func TestStartingWithoutTheLockCanStillEnableIt(t *testing.T) {
	released := 0
	s := &state{acquire: func(awake.Options) (awake.Lock, error) {
		return fakeLock{&released}, nil
	}}
	s.setAwake(false)
	if s.awakeNote != "off" || s.lock != nil {
		t.Fatal("should start with nothing held")
	}
	s.toggleAwake()
	if !s.wantAwake || s.lock == nil || s.awakeNote != "on" {
		t.Error("toggling should take the lock even when it started off")
	}
}

func TestReleaseAwakeIsSafeWithNothingHeld(t *testing.T) {
	s := &state{}
	s.releaseAwake()
	s.releaseAwake()
}
