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

	// The width from the original bug report. The fullest hint no longer fits
	// there, so the layout drops to a shorter form — but every key must still
	// be reachable, because a hint that hides a key is worse than no hint.
	at78 := mk(78)
	for _, want := range []string{"q quit", "t theme", "m mute", "w awake",
		"space pause", "i idle"} {
		if !contains(at78, want) {
			t.Errorf("a 78-column status area should show %q, got:\n%s", want, at78)
		}
	}

	// Given room, the fullest form comes back.
	at120 := mk(120)
	for _, want := range []string{"t next theme", "w awake", "+ / - resize"} {
		if !contains(at120, want) {
			t.Errorf("a 120-column status area should show %q, got:\n%s", want, at120)
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

// Pausing has to stop the sound as well as the picture: leaving a chant
// looping over a frozen frame is neither useful nor free.
func TestAudioStateRules(t *testing.T) {
	cases := []struct {
		name                            string
		hasPlayer, silence, muted, susp bool
		wantPlay                        bool
		wantNote                        string
	}{
		{"flying", true, false, false, false, true, "on"},
		{"paused stops the sound", true, false, false, true, false, "paused"},
		{"muted", true, false, true, false, false, "muted"},
		{"muted and paused stays muted", true, false, true, true, false, "muted"},
		{"silenced", true, true, false, false, false, "off"},
		{"silence beats everything", true, true, true, true, false, "off"},
		{"no player", false, false, false, false, false, "unavailable"},
	}
	for _, c := range cases {
		play, note := audioState(c.hasPlayer, c.silence, c.muted, c.susp)
		if play != c.wantPlay || note != c.wantNote {
			t.Errorf("%s: got (%v, %q), want (%v, %q)",
				c.name, play, note, c.wantPlay, c.wantNote)
		}
	}
}

func TestSuspendedCoversPauseAndIdle(t *testing.T) {
	for _, c := range []struct {
		paused, idle, want bool
	}{
		{false, false, false},
		{true, false, true},
		{false, true, true},
		{true, true, true},
	} {
		s := &state{paused: c.paused, idle: c.idle}
		if got := s.suspended(); got != c.want {
			t.Errorf("paused=%v idle=%v: suspended()=%v, want %v",
				c.paused, c.idle, got, c.want)
		}
	}
}

// The status timer must show flight time, not wall-clock time since launch.
// Pausing at 02:15 and resuming used to continue from wherever real time had
// got to, because the display and the animation were reading different clocks.
func TestStatusTimeIsFlightTimeNotWallClock(t *testing.T) {
	th, _ := theme.Get("crimson")
	show := func(clock float64) string {
		s := &state{
			theme: th, mode: theme.ModeNone,
			awakeNote: "on", soundNote: "on", clock: clock,
			screen: render.NewScreen(140, 24, render.StyleHalfBlock, theme.ModeNone),
		}
		return joinLines(s.statusBar(s.flightTime()))
	}

	for clock, want := range map[float64]string{
		0:     "00:00",
		135:   "02:15",
		135.9: "02:15", // truncated toward the second that has elapsed
		3725:  "1:02:05",
	} {
		if got := show(clock); !contains(got, want) {
			t.Errorf("clock=%.1f should display %q, got: %s", clock, want, got)
		}
	}

	// Resuming continues from where it paused rather than jumping: the clock
	// is the only input, so a paused state redraws the same time however long
	// the pause lasted.
	paused := &state{
		theme: th, mode: theme.ModeNone, paused: true,
		awakeNote: "on", soundNote: "paused", clock: 135,
		screen: render.NewScreen(140, 24, render.StyleHalfBlock, theme.ModeNone),
	}
	first := joinLines(paused.statusBar(paused.flightTime()))
	time.Sleep(1100 * time.Millisecond)
	second := joinLines(paused.statusBar(paused.flightTime()))
	if first != second {
		t.Errorf("a paused status line changed over a second of real time:\n%s\n%s", first, second)
	}
	if !contains(first, "02:15") || !contains(first, "paused") {
		t.Errorf("expected a paused line at 02:15, got: %s", first)
	}
}

func TestFlightTimeConversion(t *testing.T) {
	for clock, want := range map[float64]time.Duration{
		0:    0,
		1:    time.Second,
		90.5: 90500 * time.Millisecond,
	} {
		s := &state{clock: clock}
		if got := s.flightTime(); got != want {
			t.Errorf("clock=%v -> %v, want %v", clock, got, want)
		}
	}
}

func joinLines(lines [][]byte) string {
	var out string
	for _, l := range lines {
		out += string(ansi.ReplaceAll(l, nil)) + "\n"
	}
	return out
}
