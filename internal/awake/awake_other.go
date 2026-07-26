//go:build !darwin && !linux && !windows

package awake

func acquire(o Options) (Lock, error) { return Noop(), ErrUnsupported }
