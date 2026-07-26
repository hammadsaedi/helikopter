//go:build windows

package awake

import (
	"runtime"
	"sync"
	"syscall"
)

const (
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
	esContinuous      = 0x80000000
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
)

// The execution state is per-thread, so the flags have to be set and later
// cleared on the same locked OS thread. A dedicated goroutine owns it and
// waits for the release signal.
type winLock struct {
	done   chan struct{}
	method string
	once   sync.Once
}

func (w *winLock) Method() string { return w.method }

func (w *winLock) Release() { w.once.Do(func() { close(w.done) }) }

func acquire(o Options) (Lock, error) {
	flags := uintptr(esContinuous | esSystemRequired)
	method := "SetThreadExecutionState(system)"
	if o.Display {
		flags |= esDisplayRequired
		method = "SetThreadExecutionState(system+display)"
	}

	l := &winLock{done: make(chan struct{}), method: method}
	ready := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		r, _, err := procSetThreadExecutionState.Call(flags)
		if r == 0 {
			ready <- err
			return
		}
		ready <- nil

		<-l.done
		procSetThreadExecutionState.Call(uintptr(esContinuous))
	}()

	if err := <-ready; err != nil {
		return Noop(), err
	}
	return l, nil
}
