package audio

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newFakePlayer builds a player whose "backend" appends a line to a log each
// time it is launched, so the test can count launches.
func newFakePlayer(t *testing.T) (*Player, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in backend is a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "launches")
	wav := filepath.Join(dir, "helikopter.wav")
	data, _, err := Render(Config{Music: false, Rotor: false, Volume: 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wav, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return &Player{
		path: wav,
		backend: backend{
			name: "fake",
			bin:  "/bin/sh",
			args: func(p string) []string {
				return []string{"-c", "echo launched >> " + log + "; sleep 30"}
			},
		},
		wake: make(chan struct{}, 1),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}, log
}

func launches(t *testing.T, log string) int {
	t.Helper()
	b, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "launched")
}

// waitFor polls until cond holds or the deadline passes.
func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

// Resuming has to start the audio again from the beginning rather than
// carrying on from wherever it was cut off, so the chant is never rejoined
// mid-phrase.
func TestResumeRestartsPlaybackFromTheBeginning(t *testing.T) {
	p, log := newFakePlayer(t)
	defer p.Stop()

	p.Start()
	if !waitFor(t, 2*time.Second, func() bool { return launches(t, log) == 1 }) {
		t.Fatalf("player did not launch: %d launches", launches(t, log))
	}

	p.Pause()
	if !waitFor(t, 2*time.Second, func() bool { return !p.Playing() }) {
		t.Fatal("player did not pause")
	}
	// Paused means the process is gone, not merely silent.
	before := launches(t, log)

	time.Sleep(200 * time.Millisecond)
	if got := launches(t, log); got != before {
		t.Errorf("a paused player kept relaunching: %d then %d", before, got)
	}

	p.Resume()
	if !waitFor(t, 2*time.Second, func() bool { return launches(t, log) == before+1 }) {
		t.Fatalf("resume did not start a fresh process: %d launches", launches(t, log))
	}
}

func TestToggleReportsAndFlipsState(t *testing.T) {
	p, _ := newFakePlayer(t)
	defer p.Stop()

	p.Start()
	if !p.Playing() {
		t.Fatal("player should be playing after Start")
	}
	if on := p.Toggle(); on || p.Playing() {
		t.Error("first toggle should pause")
	}
	if on := p.Toggle(); !on || !p.Playing() {
		t.Error("second toggle should resume")
	}
}
