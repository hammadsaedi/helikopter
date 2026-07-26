package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/hammadsaedi/helikopter/internal/awake"

	"github.com/hammadsaedi/helikopter/internal/audio"
	"github.com/hammadsaedi/helikopter/internal/render"
	"github.com/hammadsaedi/helikopter/internal/term"
	"github.com/hammadsaedi/helikopter/internal/theme"
)

type state struct {
	opts  *options
	theme *theme.Theme
	tm    *term.Term

	lockNote  string
	soundNote string
	player    *audio.Player
	silence   bool

	themeNames []string
	total      time.Duration
	idleAfter  time.Duration

	screen *render.Screen
	scene  *render.Scene
	mode   theme.Mode
	style  render.Style

	paused bool
	idle   bool
	super  int
	size   float64

	started time.Time
	clock   float64 // animation time, frozen while paused
}

func (s *state) colorMode() theme.Mode {
	if m, explicit := theme.ParseMode(s.opts.colorArg); explicit {
		return m
	}
	return theme.DetectMode(s.tm.IsTTY())
}

// autoSuper turns supersampling off on very large terminals, where the extra
// samples cost real CPU and buy almost nothing at that pixel density.
func autoSuper(pixels int) int {
	if pixels > 90_000 {
		return 1
	}
	return 2
}

func (s *state) setup() {
	s.mode = s.colorMode()
	s.style = render.StyleHalfBlock
	if s.opts.ascii {
		s.style = render.StyleASCII
	}
	s.size = s.opts.size
	if s.size < 0.15 {
		s.size = 0.15
	}
	if s.size > 1.0 {
		s.size = 1.0
	}

	cols, rows := s.tm.Size()
	s.screen = render.NewScreen(cols, rows, s.style, s.mode)

	c := s.screen.Canvas()
	s.super = s.opts.quality
	if s.super < 1 || s.super > 2 {
		s.super = autoSuper(c.W * c.H)
	}
	s.scene = &render.Scene{
		Theme: s.theme,
		Seed:  uint32(s.opts.seed),
		Size:  s.size,
		Super: s.super,
	}
}

func (s *state) renderSnapshot() error {
	s.setup()
	s.scene.Render(s.screen.Canvas(), 3.7)
	s.screen.SetStatus(s.statusBar(0))
	os.Stdout.Write(s.screen.Flush())
	fmt.Println()
	return nil
}

func (s *state) runAnimated(sig <-chan os.Signal) error {
	if err := s.tm.MakeRaw(); err != nil {
		return err
	}
	s.tm.EnterAlt()
	defer s.tm.Cleanup()

	s.setup()
	keys := s.tm.Keys()

	fps := s.opts.fps
	if fps < 1 {
		fps = 1
	}
	if fps > 60 {
		fps = 60
	}
	budget := time.Second / time.Duration(fps)

	ticker := time.NewTicker(budget)
	defer ticker.Stop()
	tick := ticker.C

	s.started = time.Now()
	lastSize := time.Now()
	lastFrame := time.Now()
	var renderEMA time.Duration

	var deadline, idleDeadline <-chan time.Time
	if s.total > 0 {
		deadline = time.After(s.total)
	}
	if s.idleAfter > 0 {
		idleDeadline = time.After(s.idleAfter)
	}

	enterIdle := func() {
		s.idle = true
		ticker.Stop()
		tick = nil
		if s.player != nil {
			s.player.Pause()
		}
		s.drawIdleScreen()
	}
	leaveIdle := func() {
		s.idle = false
		ticker.Reset(budget)
		tick = ticker.C
		if s.player != nil && !s.silence {
			s.player.Resume()
		}
		s.screen.Invalidate()
		s.scene.Reset()
		lastFrame = time.Now()
	}

	for {
		select {
		case <-sig:
			return nil

		case <-deadline:
			return nil

		case <-idleDeadline:
			idleDeadline = nil
			if !s.idle {
				enterIdle()
			}

		case k, ok := <-keys:
			if !ok {
				keys = nil
				continue
			}
			switch k {
			case 'q', 'Q', 3, 4: // q, Ctrl-C, Ctrl-D
				return nil
			case ' ':
				s.paused = !s.paused
				lastFrame = time.Now()
			case 't', 'T':
				s.nextTheme(1)
			case 'm', 'M':
				if s.player != nil {
					if s.player.Toggle() {
						s.soundNote = s.player.Method()
					} else {
						s.soundNote = "muted"
					}
				}
			case 'i', 'I':
				if s.idle {
					leaveIdle()
				} else {
					enterIdle()
				}
			case '+', '=':
				s.resize(0.04)
			case '-', '_':
				s.resize(-0.04)
			}
			if s.idle {
				s.drawIdleScreen()
			}

		case <-tick:
			now := time.Now()

			if now.Sub(lastSize) > 250*time.Millisecond {
				lastSize = now
				if cols, rows := s.tm.Size(); cols != s.screen.Cols || rows != s.screen.Rows {
					s.screen.Resize(cols, rows)
					c := s.screen.Canvas()
					if s.opts.quality < 1 || s.opts.quality > 2 {
						s.super = autoSuper(c.W * c.H)
						s.scene.Super = s.super
					}
					s.scene.Reset()
				}
			}

			if !s.paused {
				s.clock += now.Sub(lastFrame).Seconds()
			}
			lastFrame = now

			t0 := time.Now()
			s.scene.Render(s.screen.Canvas(), s.clock)
			s.screen.SetStatus(s.statusBar(now.Sub(s.started)))
			out := s.screen.Flush()
			os.Stdout.Write(out)
			cost := time.Since(t0)

			// Keep the frame inside its budget by trading away supersampling
			// before we start dropping frames.
			if renderEMA == 0 {
				renderEMA = cost
			} else {
				renderEMA = (renderEMA*7 + cost) / 8
			}
			if s.opts.quality < 1 || s.opts.quality > 2 {
				if renderEMA > budget*3/5 && s.super > 1 {
					s.super--
					s.scene.Super = s.super
					renderEMA = 0
				} else if renderEMA < budget/5 && s.super < 2 {
					s.super++
					s.scene.Super = s.super
					renderEMA = 0
				}
			}
		}
	}
}

func (s *state) resize(delta float64) {
	s.size += delta
	if s.size < 0.15 {
		s.size = 0.15
	}
	if s.size > 1.0 {
		s.size = 1.0
	}
	s.scene.Size = s.size
}

func (s *state) nextTheme(step int) {
	idx := 0
	for i, n := range s.themeNames {
		if n == s.theme.Name {
			idx = i
			break
		}
	}
	idx = (idx + step + len(s.themeNames)) % len(s.themeNames)
	t, err := theme.Get(s.themeNames[idx])
	if err != nil {
		return
	}
	s.theme = t
	s.scene.Theme = t
	s.scene.Reset()
	s.screen.Invalidate()
}

// drawIdleScreen paints a still frame once. Nothing is redrawn until the user
// leaves idle, so the process sits entirely blocked.
func (s *state) drawIdleScreen() {
	c := s.screen.Canvas()
	for i := range c.Px {
		c.Px[i] = s.theme.SkyTop
	}
	s.scene.Render(c, s.clock)
	s.screen.SetStatus(s.statusBar(time.Since(s.started)))
	os.Stdout.Write(s.screen.Flush())
}

// runIdle is the no-animation path, used for --idle and whenever stdout is not
// a terminal.
//
// Wrapping an inhibitor in a supervising Go process would cost twice the memory
// of the inhibitor alone, so where the platform has a native one we exec into
// it and cease to exist. Where it does not — Windows, whose wake lock is a
// thread flag rather than a program — we hold the lock here and trim the
// runtime down instead.
func runIdle(o *options, total time.Duration, notTTY bool) error {
	reason := "idle mode"
	if notTTY {
		reason = "not a terminal"
	}
	window := "until interrupted"
	if total > 0 {
		window = "for " + total.Round(time.Second).String()
	}

	if o.noWakeLock {
		fmt.Printf("helikopter: idling %s (%s; no wake lock)\n", window, reason)
		return waitOut(total)
	}

	opts := awake.Options{Display: !o.noDisplay}

	if name, ok := awake.NativeInhibitor(opts); ok {
		fmt.Printf("helikopter: holding this machine awake %s (%s; becoming %s)\n",
			window, reason, name)
		// Only returns if the exec fails, in which case fall through.
		if err := awake.Become(opts, total); err != nil {
			fmt.Fprintf(os.Stderr, "helikopter: could not exec %s: %v\n", name, err)
		}
	}

	lock, err := awake.Acquire(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr,
			"helikopter: warning — no wake-lock mechanism available on this system")
	}
	defer lock.Release()

	fmt.Printf("helikopter: holding this machine awake %s (%s; wake lock: %s)\n",
		window, reason, lock.Method())

	// Nothing further will allocate, so hand the heap back and stop the
	// scheduler spreading across every core while we sit on a channel.
	runtime.GOMAXPROCS(1)
	debug.FreeOSMemory()

	return waitOut(total)
}

// waitOut blocks until interrupted, or until the duration elapses.
func waitOut(total time.Duration) error {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	var deadline <-chan time.Time
	if total > 0 {
		t := time.NewTimer(total)
		defer t.Stop()
		deadline = t.C
	}
	select {
	case <-sig:
	case <-deadline:
	}
	return nil
}

// ---------------------------------------------------------------------------
// status bar
// ---------------------------------------------------------------------------

type seg struct {
	text string
	col  theme.RGB
}

func (s *state) statusBar(elapsed time.Duration) []byte {
	th := s.theme

	stateWord := "flying"
	switch {
	case s.idle:
		stateWord = "idle"
	case s.paused:
		stateWord = "paused"
	}

	segs := []seg{
		{" helikopter ", th.UIKey},
		{"· ", th.UIDim},
		{th.Name + " ", th.UIText},
		{"· awake ", th.UIDim},
		{s.lockNote + " ", th.UIText},
		{"· sound ", th.UIDim},
		{s.soundNote + " ", th.UIText},
		{"· " + stateWord + " ", th.UIDim},
		{fmtDur(elapsed) + " ", th.UIText},
	}

	keys := []seg{
		{"q", th.UIKey}, {" quit  ", th.UIDim},
		{"t", th.UIKey}, {" theme  ", th.UIDim},
		{"m", th.UIKey}, {" mute  ", th.UIDim},
		{"i", th.UIKey}, {" idle ", th.UIDim},
	}

	left := width(segs)
	right := width(keys)
	pad := s.screen.Cols - left - right
	if pad < 1 {
		keys = nil
		pad = s.screen.Cols - left
	}
	if pad < 0 {
		pad = 0
	}

	var out []byte
	out = theme.AppendBg(out, dim(th.SkyTop), s.mode)
	for _, g := range segs {
		out = appendSeg(out, g, s.mode)
	}
	out = append(out, strings.Repeat(" ", pad)...)
	for _, g := range keys {
		out = appendSeg(out, g, s.mode)
	}
	return out
}

func appendSeg(dst []byte, g seg, m theme.Mode) []byte {
	dst = theme.AppendFg(dst, g.col, m)
	return append(dst, g.text...)
}

func width(segs []seg) int {
	n := 0
	for _, g := range segs {
		n += len([]rune(g.text))
	}
	return n
}

// dim darkens a colour for the status bar background.
func dim(c theme.RGB) theme.RGB {
	return theme.RGB{R: c.R / 2, G: c.G / 2, B: c.B / 2}
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	sec := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d", m, sec)
}
