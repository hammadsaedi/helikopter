package audio

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// ErrNoPlayer means no usable playback mechanism was found.
var ErrNoPlayer = errors.New("no audio player found")

// Player owns the rendered WAV and the engine playing it.
//
// Pausing stops playback outright rather than turning the volume down, so a
// paused player costs nothing at all.
type Player struct {
	path   string
	source Source

	mu      sync.Mutex
	eng     engine
	playing bool
	once    sync.Once
}

// New renders the soundtrack to a temp WAV and prepares a player without
// starting playback.
func New(cfg Config) (*Player, error) {
	eng, err := newEngine()
	if err != nil {
		return nil, err
	}

	data, source, err := Render(cfg)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "helikopter-")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "helikopter.wav")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	return &Player{path: path, source: source, eng: eng}, nil
}

// Method names the playback mechanism in use.
func (p *Player) Method() string { return p.eng.name() }

// Source reports where the audio came from.
func (p *Player) Source() Source { return p.source }

// Playing reports whether sound is currently running.
func (p *Player) Playing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// Start begins playback.
func (p *Player) Start() { p.Resume() }

// Resume starts playback, from the beginning of the loop.
func (p *Player) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.playing {
		return
	}
	if err := p.eng.start(p.path); err != nil {
		return
	}
	p.playing = true
}

// Pause stops the sound without discarding the rendered audio, so it can be
// resumed later.
func (p *Player) Pause() {
	p.mu.Lock()
	if !p.playing {
		p.mu.Unlock()
		return
	}
	p.playing = false
	p.mu.Unlock()

	p.eng.stop()
}

// Toggle flips between playing and paused and reports the new state.
func (p *Player) Toggle() bool {
	if p.Playing() {
		p.Pause()
		return false
	}
	p.Resume()
	return p.Playing()
}

// Stop ends playback for good and removes the temp file.
func (p *Player) Stop() {
	p.once.Do(func() {
		p.Pause()
		os.RemoveAll(filepath.Dir(p.path))
	})
}
