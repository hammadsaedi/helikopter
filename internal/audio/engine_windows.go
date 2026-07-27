//go:build windows

package audio

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// Windows plays the clip inside this process through winmm, which loops
// natively and needs no helper.
//
// The alternative was launching powershell.exe to drive SoundPlayer, which
// worked but was a poor thing for a downloaded binary to do: spawning a shell
// is a shape security tooling is right to be suspicious of, and it cost a
// whole extra process for a sound that plays itself.
//
// PlaySound only understands WAV, which is what ships and what --sound
// accepts anyway.
var (
	winmm         = syscall.NewLazyDLL("winmm.dll")
	procPlaySound = winmm.NewProc("PlaySoundW")
)

const (
	sndAsync     = 0x0001
	sndLoop      = 0x0008
	sndPurge     = 0x0040
	sndFilename  = 0x00020000
	sndNoDefault = 0x0002
)

type winmmEngine struct {
	mu      sync.Mutex
	playing bool
}

func (e *winmmEngine) name() string { return "winmm" }

func (e *winmmEngine) start(path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.playing {
		return nil
	}

	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, callErr := procPlaySound.Call(
		uintptr(unsafe.Pointer(p)),
		0,
		uintptr(sndFilename|sndAsync|sndLoop|sndNoDefault),
	)
	if r == 0 {
		return fmt.Errorf("PlaySound failed: %v", callErr)
	}
	e.playing = true
	return nil
}

func (e *winmmEngine) stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.playing {
		return
	}
	// A null sound with SND_PURGE stops whatever this process is playing.
	procPlaySound.Call(0, 0, uintptr(sndPurge))
	e.playing = false
}

func newEngine() (engine, error) {
	if err := procPlaySound.Find(); err != nil {
		return nil, ErrNoPlayer
	}
	return &winmmEngine{}, nil
}
