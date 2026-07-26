// Package awake holds a wake lock for as long as the process runs, so
// `helikopter` can stand in for `caffeinate -d`.
package awake

import "errors"

// ErrUnsupported means no wake-lock mechanism was found on this system.
var ErrUnsupported = errors.New("no supported keep-awake mechanism")

// Lock is a held wake lock.
type Lock interface {
	// Method names the mechanism, for the status line.
	Method() string
	// Release drops the lock. Safe to call more than once.
	Release()
}

// Options controls how aggressive the lock is.
type Options struct {
	// Display also prevents the screen from sleeping, as `caffeinate -d` does.
	// With it false only idle system sleep is blocked.
	Display bool
}

// Acquire takes a wake lock. On failure it returns a lock that does nothing,
// alongside the error, so callers can carry on and just report it.
func Acquire(o Options) (Lock, error) { return acquire(o) }

type noopLock struct{}

func (noopLock) Method() string { return "none" }
func (noopLock) Release()       {}

// Noop is a lock that holds nothing.
func Noop() Lock { return noopLock{} }
