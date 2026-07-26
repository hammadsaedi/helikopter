package audio

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
)

// ErrNoPlayer means no usable audio command was found.
var ErrNoPlayer = errors.New("no audio player found")

// Player owns the temp WAV file and the process playing it. Pausing kills the
// child rather than muting it, so a paused player costs nothing.
type Player struct {
	path    string
	backend backend

	mu   sync.Mutex
	cmd  *exec.Cmd
	want atomic.Bool
	wake chan struct{}
	quit chan struct{}
	done chan struct{}
	once sync.Once
}

type backend struct {
	name string
	bin  string
	// args builds the command line. loops reports whether the backend repeats
	// on its own; if not, the player restarts it when it exits.
	args  func(path string) []string
	loops bool
}

// New renders the soundtrack to a temp WAV and prepares a player without
// starting playback.
func New(cfg Config) (*Player, error) {
	b, err := findBackend()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "helikopter-")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "helikopter.wav")
	if err := os.WriteFile(path, Render(cfg), 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &Player{
		path:    path,
		backend: b,
		wake:    make(chan struct{}, 1),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}, nil
}

// Method names the backend in use.
func (p *Player) Method() string { return p.backend.name }

// Playing reports whether sound is currently running.
func (p *Player) Playing() bool { return p.want.Load() }

// Start begins playback.
func (p *Player) Start() {
	p.want.Store(true)
	go p.loop()
}

// Pause stops the sound without discarding the rendered audio.
func (p *Player) Pause() {
	p.want.Store(false)
	p.kill()
}

// Resume restarts playback after a Pause.
func (p *Player) Resume() {
	p.want.Store(true)
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Toggle flips between playing and paused and reports the new state.
func (p *Player) Toggle() bool {
	if p.want.Load() {
		p.Pause()
		return false
	}
	p.Resume()
	return true
}

// Stop ends playback for good and removes the temp file.
func (p *Player) Stop() {
	p.once.Do(func() {
		p.want.Store(false)
		close(p.quit)
		p.kill()
		<-p.done
		os.RemoveAll(filepath.Dir(p.path))
	})
}

func (p *Player) kill() {
	p.mu.Lock()
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	p.mu.Unlock()
}

func (p *Player) loop() {
	defer close(p.done)
	for {
		select {
		case <-p.quit:
			return
		default:
		}

		// Paused: block until woken or stopped. No polling, no CPU.
		if !p.want.Load() {
			select {
			case <-p.wake:
				continue
			case <-p.quit:
				return
			}
		}

		cmd := exec.Command(p.backend.bin, p.backend.args(p.path)...)
		p.mu.Lock()
		if !p.want.Load() {
			p.mu.Unlock()
			continue
		}
		if err := cmd.Start(); err != nil {
			p.mu.Unlock()
			return
		}
		p.cmd = cmd
		p.mu.Unlock()

		_ = cmd.Wait()

		p.mu.Lock()
		p.cmd = nil
		p.mu.Unlock()

		if p.backend.loops && p.want.Load() {
			// A looping backend only exits because we killed it.
			select {
			case <-p.quit:
				return
			default:
			}
		}
	}
}

func findBackend() (backend, error) {
	var candidates []backend

	switch runtime.GOOS {
	case "darwin":
		candidates = []backend{
			{name: "afplay", bin: "afplay", args: func(p string) []string { return []string{p} }},
		}
	case "windows":
		// SoundPlayer.PlayLooping loops natively; the sleep keeps the host
		// process, and so the sound, alive until we kill it.
		ps := func(p string) []string {
			return []string{
				"-NoProfile", "-NonInteractive", "-Command",
				`$p = New-Object System.Media.SoundPlayer '` + p + `'; ` +
					`$p.PlayLooping(); while ($true) { Start-Sleep -Seconds 3600 }`,
			}
		}
		candidates = []backend{
			{name: "powershell", bin: "powershell.exe", args: ps, loops: true},
			{name: "pwsh", bin: "pwsh", args: ps, loops: true},
		}
	default:
		candidates = []backend{
			{name: "paplay", bin: "paplay", args: func(p string) []string { return []string{p} }},
			{name: "pw-play", bin: "pw-play", args: func(p string) []string { return []string{p} }},
			{name: "aplay", bin: "aplay", args: func(p string) []string { return []string{"-q", p} }},
			{name: "ffplay", bin: "ffplay", args: func(p string) []string {
				return []string{"-nodisp", "-autoexit", "-loglevel", "quiet", p}
			}},
			{name: "mpv", bin: "mpv", args: func(p string) []string {
				return []string{"--no-video", "--really-quiet", p}
			}},
			{name: "play", bin: "play", args: func(p string) []string { return []string{"-q", p} }},
		}
	}

	for _, c := range candidates {
		if bin, err := exec.LookPath(c.bin); err == nil {
			c.bin = bin
			return c, nil
		}
	}
	return backend{}, ErrNoPlayer
}
