//go:build !windows

package audio

import (
	"os/exec"
	"runtime"
	"sync"
)

// procEngine drives one of the platform's audio commands.
//
// None of them loop, so the process is relaunched each time it reaches the end
// of the clip. Stopping kills it outright rather than muting, which is why a
// paused player costs nothing.
type procEngine struct {
	label string
	bin   string
	args  func(path string) []string

	mu      sync.Mutex
	cmd     *exec.Cmd
	quit    chan struct{}
	done    chan struct{}
	playing bool
}

func (e *procEngine) name() string { return e.label }

func (e *procEngine) start(path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.playing {
		return nil
	}
	e.playing = true
	e.quit = make(chan struct{})
	e.done = make(chan struct{})
	go e.loop(path, e.quit, e.done)
	return nil
}

func (e *procEngine) stop() {
	e.mu.Lock()
	if !e.playing {
		e.mu.Unlock()
		return
	}
	e.playing = false
	quit, done := e.quit, e.done
	close(quit)
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
	e.mu.Unlock()

	<-done
}

func (e *procEngine) loop(path string, quit <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-quit:
			return
		default:
		}

		cmd := exec.Command(e.bin, e.args(path)...)

		e.mu.Lock()
		select {
		case <-quit:
			e.mu.Unlock()
			return
		default:
		}
		if err := cmd.Start(); err != nil {
			e.mu.Unlock()
			return
		}
		e.cmd = cmd
		e.mu.Unlock()

		_ = cmd.Wait()

		e.mu.Lock()
		e.cmd = nil
		e.mu.Unlock()
	}
}

func newEngine() (engine, error) {
	type candidate struct {
		label, bin string
		args       func(string) []string
	}
	one := func(p string) []string { return []string{p} }

	var candidates []candidate
	switch runtime.GOOS {
	case "darwin":
		candidates = []candidate{{"afplay", "afplay", one}}
	default:
		candidates = []candidate{
			{"paplay", "paplay", one},
			{"pw-play", "pw-play", one},
			{"aplay", "aplay", func(p string) []string { return []string{"-q", p} }},
			{"ffplay", "ffplay", func(p string) []string {
				return []string{"-nodisp", "-autoexit", "-loglevel", "quiet", p}
			}},
			{"mpv", "mpv", func(p string) []string {
				return []string{"--no-video", "--really-quiet", p}
			}},
			{"play", "play", func(p string) []string { return []string{"-q", p} }},
		}
	}

	for _, c := range candidates {
		bin, err := exec.LookPath(c.bin)
		if err != nil {
			continue
		}
		return &procEngine{label: c.label, bin: bin, args: c.args}, nil
	}
	return nil, ErrNoPlayer
}
