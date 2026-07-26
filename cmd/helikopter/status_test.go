package main

import (
	"regexp"
	"testing"
	"time"

	"github.com/hammadsaedi/helikopter/internal/render"
	"github.com/hammadsaedi/helikopter/internal/theme"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func visibleWidth(b []byte) int {
	return len([]rune(string(ansi.ReplaceAll(b, nil))))
}

// A status line one character too wide wraps, and a wrap inside the alternate
// screen scrolls everything and corrupts every later differential update. It
// must fit at every terminal width, in every state.
func TestStatusBarNeverExceedsTerminalWidth(t *testing.T) {
	th, err := theme.Get("crimson")
	if err != nil {
		t.Fatal(err)
	}

	notes := []string{"on", "off", "muted", "unavailable",
		"power assertion (system + display)", "systemd-inhibit"}
	durations := []time.Duration{0, 59 * time.Second, 90 * time.Minute, 26 * time.Hour}

	for cols := 2; cols <= 400; cols++ {
		for _, note := range notes {
			for _, d := range durations {
				for _, mode := range []theme.Mode{theme.ModeTrue, theme.Mode256, theme.ModeNone} {
					for _, flag := range []struct{ idle, paused bool }{
						{false, false}, {true, false}, {false, true},
					} {
						s := &state{
							theme:     th,
							mode:      mode,
							awakeNote: note,
							soundNote: note,
							idle:      flag.idle,
							paused:    flag.paused,
							screen:    render.NewScreen(cols, 10, render.StyleHalfBlock, mode),
						}
						// NewScreen enforces a floor, so measure against the
						// width it actually settled on.
						max := s.screen.Cols - 1
						for i, line := range s.statusBar(d) {
							if got := visibleWidth(line); got > max {
								t.Fatalf("cols=%d row=%d note=%q idle=%v paused=%v: %d wide, max %d",
									cols, i, note, flag.idle, flag.paused, got, max)
							}
						}
					}
				}
			}
		}
	}
}

// Narrowing the terminal should shed detail, not the name.
func TestStatusBarDegradesGracefully(t *testing.T) {
	th, _ := theme.Get("crimson")
	mk := func(cols int) string {
		s := &state{
			theme: th, mode: theme.ModeNone,
			awakeNote: "on", soundNote: "on",
			screen: render.NewScreen(cols, 24, render.StyleHalfBlock, theme.ModeNone),
		}
		var all string
		for _, line := range s.statusBar(90 * time.Second) {
			all += string(ansi.ReplaceAll(line, nil)) + "\n"
		}
		return all
	}

	wide := mk(140)
	for _, want := range []string{"helikopter", "crimson", "flying", "awake", "sound", "quit"} {
		if !contains(wide, want) {
			t.Errorf("a 140-column status bar should mention %q: %q", want, wide)
		}
	}

	if narrow := mk(30); !contains(narrow, "helikopter") {
		t.Errorf("even a 30-column status bar should name the program: %q", narrow)
	}

	// The width from the bug report: the full key hint must survive there.
	for _, want := range []string{"q quit", "t next theme", "m mute",
		"space pause", "i idle", "+ / - resize"} {
		if !contains(mk(78), want) {
			t.Errorf("a 78-column status area should show %q, got:\n%s", want, mk(78))
		}
	}
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
