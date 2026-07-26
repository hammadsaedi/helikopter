//go:build !linux

package awake

import "time"

// Windows has no exec, and its wake lock is a thread flag rather than a
// separate program, so there is nothing to turn into: the caller keeps the
// in-process lock and trims the runtime instead.
func nativeInhibitor(o Options) (string, bool) { return "", false }

func become(o Options, d time.Duration) error { return ErrUnsupported }
