package awake

import "time"

// Become replaces this process image with the platform's own inhibitor.
//
// Acquire() holds the lock in a child process, which is right while we are
// animating but costs a whole extra process. Idle mode has nothing left to do
// once the lock is held, so instead of supervising an inhibitor it turns into
// one: after this call there is no Go runtime left, and the footprint is
// exactly the platform tool's own.
//
// It only returns on failure. d is an optional lifetime; zero means forever.
func Become(o Options, d time.Duration) error { return become(o, d) }

// NativeInhibitor names the tool Become would exec into, and reports whether
// it is available, so callers can say what they are about to turn into.
func NativeInhibitor(o Options) (string, bool) { return nativeInhibitor(o) }
