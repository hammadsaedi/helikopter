//go:build darwin

package awake

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

// macOS exposes power management directly through IOKit, so the wake lock is a
// pair of kernel assertions held by this process: no helper process, and the
// kernel drops them however we exit, including kill -9.
//
// The framework is bound at runtime rather than through cgo. That keeps the
// binary on the small pure-Go runtime — the difference is several megabytes of
// resident memory, which matters for a tool whose whole job is to sit still.
const (
	assertSystem  = "PreventUserIdleSystemSleep"
	assertDisplay = "PreventUserIdleDisplaySleep"
	assertionName = "helikopter is flying"

	kCFStringEncodingUTF8 = 0x08000100
	kIOPMAssertionLevelOn = 255
	kIOReturnSuccess      = 0
)

var (
	iokitOnce sync.Once
	iokitErr  error

	cfStringCreateWithCString func(alloc uintptr, s string, enc uint32) uintptr
	cfRelease                 func(ref uintptr)
	assertionCreateWithName   func(t uintptr, level uint32, name uintptr, id *uint32) int32
	assertionRelease          func(id uint32) int32
)

func loadIOKit() error {
	iokitOnce.Do(func() {
		cf, err := purego.Dlopen(
			"/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation",
			purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			iokitErr = fmt.Errorf("CoreFoundation: %w", err)
			return
		}
		io, err := purego.Dlopen(
			"/System/Library/Frameworks/IOKit.framework/IOKit",
			purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			iokitErr = fmt.Errorf("IOKit: %w", err)
			return
		}

		defer func() {
			if r := recover(); r != nil {
				iokitErr = fmt.Errorf("binding power management symbols: %v", r)
			}
		}()
		purego.RegisterLibFunc(&cfStringCreateWithCString, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&assertionCreateWithName, io, "IOPMAssertionCreateWithName")
		purego.RegisterLibFunc(&assertionRelease, io, "IOPMAssertionRelease")
	})
	return iokitErr
}

type darwinLock struct {
	ids    []uint32
	method string
	once   sync.Once
}

func (l *darwinLock) Method() string { return l.method }

func (l *darwinLock) Release() {
	l.once.Do(func() {
		for _, id := range l.ids {
			assertionRelease(id)
		}
		l.ids = nil
	})
}

func assertOne(kind string) (uint32, error) {
	ctype := cfStringCreateWithCString(0, kind, kCFStringEncodingUTF8)
	if ctype == 0 {
		return 0, fmt.Errorf("could not create assertion type string")
	}
	defer cfRelease(ctype)

	cname := cfStringCreateWithCString(0, assertionName, kCFStringEncodingUTF8)
	if cname == 0 {
		return 0, fmt.Errorf("could not create assertion name string")
	}
	defer cfRelease(cname)

	var id uint32
	if r := assertionCreateWithName(ctype, kIOPMAssertionLevelOn, cname, &id); r != kIOReturnSuccess {
		return 0, fmt.Errorf("IOPMAssertionCreateWithName(%s) returned 0x%x", kind, uint32(r))
	}
	return id, nil
}

func acquire(o Options) (Lock, error) {
	if err := loadIOKit(); err != nil {
		return Noop(), err
	}

	id, err := assertOne(assertSystem)
	if err != nil {
		return Noop(), err
	}
	l := &darwinLock{ids: []uint32{id}, method: "power assertion (system)"}

	if o.Display {
		if id, err := assertOne(assertDisplay); err == nil {
			l.ids = append(l.ids, id)
			l.method = "power assertion (system + display)"
		}
	}
	return l, nil
}
