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

	awakeNote string
	soundNote string

	// The wake lock is held by the state rather than by run(), so it can be
	// dropped and retaken while flying instead of only at startup.
	lock      awake.Lock
	awakeOpts awake.Options
	wantAwake bool
	// acquire is a seam for tests; production always uses awake.Acquire.
	acquire func(awake.Options) (awake.Lock, error)
	player  *audio.Player
	silence bool

	themeNames []string
	total      time.Duration
	idleAfter  time.Duration

	screen *render.Screen
	scene  *render.Scene
	mode   theme.Mode
	style  render.Style

	paused bool
	idle   bool
	muted  bool
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

	// Half blocks carry the whole picture in colour: with colour off they
	// degrade to a meaningless wall of ▀. Fall back to the ramp instead.
	s.style = render.StyleHalfBlock
	if s.opts.ascii || s.mode == theme.ModeNone {
		s.style = render.StyleASCII
	}

	// Without colour, hue is gone and only luminance separates the helicopter
	// from the sky. The greyscale palette is built for that, so prefer it
	// unless a theme was asked for by name.
	if s.mode == theme.ModeNone && !s.opts.themeExplicit {
		if t, err := theme.Get("mono"); err == nil {
			s.theme = t
		}
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
	pixelAspect, minimal := 1.0, false
	if s.style == render.StyleASCII {
		pixelAspect, minimal = 2.0, true
	}
	s.scene = &render.Scene{
		Theme:       s.theme,
		Seed:        uint32(s.opts.seed),
		Size:        s.size,
		Super:       s.super,
		PixelAspect: pixelAspect,
		Minimal:     minimal,
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

	// While suspended the animation ticker is stopped entirely. This slow one
	// takes its place purely so a resize is still noticed; it costs one ioctl
	// a second and nothing else.
	housekeep := time.NewTicker(time.Second)
	defer housekeep.Stop()
	housekeep.Stop()
	var slow <-chan time.Time

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

	// Pausing and idling are the same thing to the renderer: nothing moves, so
	// there is no reason to redraw an identical frame twenty times a second,
	// and no reason to keep the sound running either.
	sync := func() {
		if s.suspended() {
			ticker.Stop()
			tick = nil
			housekeep.Reset(time.Second)
			slow = housekeep.C
		} else {
			housekeep.Stop()
			slow = nil
			ticker.Reset(budget)
			tick = ticker.C
			lastFrame = time.Now()
			s.screen.Invalidate()
			s.scene.Reset()
		}
		s.syncAudio()
		s.drawFrame(time.Since(s.started))
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
				s.idle = true
				sync()
			}

		case <-slow:
			// Suspended: the only thing worth watching for is a resize.
			if cols, rows := s.tm.Size(); cols != s.screen.Cols || rows != s.screen.Rows {
				s.applyResize(cols, rows)
				s.drawFrame(time.Since(s.started))
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
				sync()
			case 't', 'T':
				s.nextTheme(1)
			case 'm', 'M':
				s.muted = !s.muted
				s.syncAudio()
			case 'i', 'I':
				s.idle = !s.idle
				sync()
			case 'w', 'W':
				s.toggleAwake()
			case '+', '=':
				s.resize(0.04)
			case '-', '_':
				s.resize(-0.04)
			}
			if s.suspended() {
				// Redraw once so the change is visible without resuming.
				s.drawFrame(time.Since(s.started))
			}

		case <-tick:
			now := time.Now()

			if now.Sub(lastSize) > 250*time.Millisecond {
				lastSize = now
				if cols, rows := s.tm.Size(); cols != s.screen.Cols || rows != s.screen.Rows {
					s.applyResize(cols, rows)
				}
			}

			s.clock += now.Sub(lastFrame).Seconds()
			lastFrame = now

			t0 := time.Now()
			s.drawFrame(now.Sub(s.started))
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

// setAwake takes or drops the wake lock. Releasing is what actually lets the
// machine sleep again: holding it is the entire point of the program, so
// leaving it held after the user has asked for sleep would be a lie.
func (s *state) setAwake(on bool) {
	if s.lock != nil {
		s.lock.Release()
		s.lock = nil
	}
	s.wantAwake = on
	if !on {
		s.awakeNote = "off"
		return
	}

	acq := s.acquire
	if acq == nil {
		acq = awake.Acquire
	}
	l, err := acq(s.awakeOpts)
	if err != nil {
		// Say so rather than claiming a lock we do not hold.
		s.wantAwake = false
		s.awakeNote = "unavailable"
		return
	}
	s.lock = l
	s.awakeNote = "on"
}

func (s *state) toggleAwake() { s.setAwake(!s.wantAwake) }

// releaseAwake drops the lock for good, on the way out.
func (s *state) releaseAwake() {
	if s.lock != nil {
		s.lock.Release()
		s.lock = nil
	}
	s.wantAwake = false
}

// suspended reports whether the animation is frozen, whether by pause or by
// idle. Both stop the clock, the redraw and the sound.
func (s *state) suspended() bool { return s.paused || s.idle }

// audioState decides whether sound should be running, and what the status
// line should call it. Kept separate from the player so the rules can be
// tested on their own.
func audioState(hasPlayer, silence, muted, suspended bool) (play bool, note string) {
	switch {
	case silence:
		return false, "off"
	case !hasPlayer:
		return false, "unavailable"
	case muted:
		return false, "muted"
	case suspended:
		// Nothing is moving, so nothing should be playing either.
		return false, "paused"
	default:
		return true, "on"
	}
}

// syncAudio brings playback in line with the current state.
func (s *state) syncAudio() {
	play, note := audioState(s.player != nil, s.silence, s.muted, s.suspended())
	s.soundNote = note
	if s.player == nil {
		return
	}
	if play {
		s.player.Resume()
	} else {
		s.player.Pause()
	}
}

func (s *state) drawFrame(elapsed time.Duration) {
	s.scene.Render(s.screen.Canvas(), s.clock)
	s.screen.SetStatus(s.statusBar(elapsed))
	os.Stdout.Write(s.screen.Flush())
}

func (s *state) applyResize(cols, rows int) {
	s.screen.Resize(cols, rows)
	c := s.screen.Canvas()
	if s.opts.quality < 1 || s.opts.quality > 2 {
		s.super = autoSuper(c.W * c.H)
		s.scene.Super = s.super
	}
	s.scene.Reset()
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

// statusBar renders the status area: flight information on the first row and
// the key hints on the second, where there is room for two.
//
// Each row is kept strictly inside the terminal width. One character too many
// wraps, and a wrap in the alternate screen scrolls everything and corrupts
// the next differential update — which is exactly what used to happen at 78
// columns. Detail is dropped by priority as the terminal narrows, and whatever
// survives is still truncated to fit.
func (s *state) statusBar(elapsed time.Duration) [][]byte {
	// Leave the last cell of each row untouched: writing it makes some
	// terminals scroll even with auto-wrap off.
	max := s.screen.Cols - 1
	if max < 1 {
		return nil
	}

	info := s.infoSegs(elapsed)
	keys := s.keySegs()

	if s.screen.StatusRows() >= 2 {
		// Pick the fullest key hint that fits on its own row.
		best := keys[len(keys)-1]
		for _, variant := range keys {
			var flat []seg
			for _, g := range variant {
				flat = append(flat, g...)
			}
			if width(flat) <= max {
				best = variant
				break
			}
		}
		return [][]byte{s.line(info, nil, max), s.line(best, nil, max)}
	}
	// Only one row to work with: fit what we can of both.
	return [][]byte{s.line(info, keys, max)}
}

func (s *state) infoSegs(elapsed time.Duration) [][]seg {
	th := s.theme
	stateWord := "flying"
	switch {
	case s.idle:
		stateWord = "idle"
	case s.paused:
		stateWord = "paused"
	}
	name := th.UIKey
	if s.style == render.StyleASCII {
		name = th.UIText
	}
	return [][]seg{
		{{" helikopter ", name}},
		{{"· ", th.UIDim}, {th.Name + " ", th.UIText}},
		{{"· " + stateWord + " ", th.UIDim}, {fmtDur(elapsed) + " ", th.UIText}},
		{{"· awake ", th.UIDim}, {s.awakeNote + " ", th.UIText}},
		{{"· sound ", th.UIDim}, {s.soundNote + " ", th.UIText}},
	}
}

// keySegs returns the key hints from fullest to tersest, so a narrow terminal
// keeps as many as will fit.
func (s *state) keySegs() [][][]seg {
	th := s.theme
	key := th.UIKey
	if s.style == render.StyleASCII {
		// Match the drawing: two tones from the palette, no accent hue.
		key = th.UIText
	}
	k := func(k2, label string) []seg {
		return []seg{{k2, key}, {" " + label, th.UIDim}}
	}
	sep := seg{" · ", th.UIDim}

	join := func(groups ...[]seg) [][]seg {
		var out [][]seg
		for i, g := range groups {
			if i > 0 {
				out = append(out, []seg{sep})
			}
			out = append(out, g)
		}
		return out
	}

	return [][][]seg{
		join(k("q", "quit"), k("t", "next theme"), k("m", "mute"), k("w", "awake"),
			k("space", "pause"), k("i", "idle"), k("+ / -", "resize")),
		join(k("q", "quit"), k("t", "theme"), k("m", "mute"), k("w", "awake"),
			k("space", "pause"), k("i", "idle")),
		join(k("q", "quit"), k("t", "theme"), k("m", "mute"), k("w", "awake"),
			k("i", "idle")),
		join(k("q", "quit"), k("t", "theme"), k("w", "awake")),
		join(k("q", "quit")),
	}
}

// line lays out chunks on the left and the best-fitting variant of right on the
// far side, padding between and truncating so the result is exactly max wide.
func (s *state) line(left [][]seg, right [][][]seg, max int) []byte {
	var chosen []seg
	used := 0
	for _, c := range left {
		w := width(c)
		if used+w > max {
			break
		}
		chosen = append(chosen, c...)
		used += w
	}

	var tail []seg
	for _, variant := range right {
		var flat []seg
		for _, g := range variant {
			flat = append(flat, g...)
		}
		if w := width(flat); used+w <= max {
			tail = flat
			used += w
			break
		}
	}

	pad := max - used
	if pad < 0 {
		pad = 0
	}

	// In ramp mode the picture has no background of its own, so a coloured
	// band under the status line reads as a foreign strip pasted over it.
	var out []byte
	if s.style == render.StyleHalfBlock {
		out = theme.AppendBg(nil, dim(s.theme.SkyTop), s.mode)
	}
	n := 0
	emit := func(g seg) {
		if n >= max {
			return
		}
		r := []rune(g.text)
		if n+len(r) > max {
			r = r[:max-n]
		}
		out = theme.AppendFg(out, g.col, s.mode)
		out = append(out, string(r)...)
		n += len(r)
	}

	for _, g := range chosen {
		emit(g)
	}
	emit(seg{strings.Repeat(" ", pad), s.theme.UIDim})
	for _, g := range tail {
		emit(g)
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
