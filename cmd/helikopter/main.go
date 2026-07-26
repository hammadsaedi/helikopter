// Command helikopter flies a helicopter in your terminal and keeps the machine
// awake while it does.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hammadsaedi/helikopter/internal/audio"
	"github.com/hammadsaedi/helikopter/internal/awake"
	"github.com/hammadsaedi/helikopter/internal/term"
	"github.com/hammadsaedi/helikopter/internal/theme"
)

// Overwritten at release time via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type options struct {
	themeName  string
	listThemes bool

	silence bool
	noMusic bool
	volume  int

	fps      int
	size     float64
	quality  int
	ascii    bool
	colorArg string

	duration  string
	idle      bool
	idleAfter string

	noWakeLock bool
	noDisplay  bool

	seed     int64
	snapshot bool

	showVersion bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "helikopter:", err)
		os.Exit(1)
	}
}

func stringVar(p *string, val string, names []string, usage string) {
	for i, n := range names {
		u := usage
		if i > 0 {
			u = ""
		}
		flag.StringVar(p, n, val, u)
	}
}

func boolVar(p *bool, names []string, usage string) {
	for i, n := range names {
		u := usage
		if i > 0 {
			u = ""
		}
		flag.BoolVar(p, n, false, u)
	}
}

func parseFlags() *options {
	o := &options{}

	stringVar(&o.themeName, theme.DefaultName, []string{"theme", "t"},
		"colour theme, or \"random\"")
	flag.BoolVar(&o.listThemes, "list-themes", false, "list themes and exit")

	boolVar(&o.silence, []string{"silence", "s"}, "fly with no sound at all")
	flag.BoolVar(&o.noMusic, "no-music", false, "rotor noise only, no music")
	flag.IntVar(&o.volume, "volume", 70, "volume, 0-100")

	flag.IntVar(&o.fps, "fps", 20, "frames per second")
	flag.Float64Var(&o.size, "size", 0.78, "helicopter width as a fraction of the terminal")
	flag.IntVar(&o.quality, "quality", 0, "supersampling 1-2 (0 = auto)")
	flag.BoolVar(&o.ascii, "ascii", false, "ASCII shading instead of half-block pixels")
	flag.StringVar(&o.colorArg, "color", "auto", "auto|never|16|256|true")

	stringVar(&o.duration, "", []string{"duration", "d"},
		"fly for this long then exit (90m, 2h; bare numbers are minutes)")
	flag.BoolVar(&o.idle, "idle", false, "no animation: just hold the machine awake, at rest")
	flag.StringVar(&o.idleAfter, "idle-after", "",
		"animate for this long, then drop to --idle")

	flag.BoolVar(&o.noWakeLock, "no-awake", false, "do not hold a wake lock")
	flag.BoolVar(&o.noDisplay, "no-display", false,
		"let the display sleep; only block system idle sleep")

	flag.Int64Var(&o.seed, "seed", 0, "scenery seed (0 = random)")
	flag.BoolVar(&o.snapshot, "snapshot", false, "print one frame and exit")

	boolVar(&o.showVersion, []string{"version", "v"}, "print version and exit")

	flag.Usage = usage
	flag.Parse()
	return o
}

func usage() {
	w := flag.CommandLine.Output()
	fmt.Fprint(w, `helikopter — a helicopter for your terminal, and a wake lock for your machine

usage: helikopter [flags]

  Runs until you press q or Ctrl-C. While it runs, the display and the system
  are held awake by a wake lock scoped to this process. Use --idle for the same
  wake lock with no animation and effectively no CPU.

flags:
  -t, --theme NAME     colour theme, or "random" (default "crimson")
      --list-themes    list themes and exit

  -s, --silence        fly with no sound at all
      --no-music       rotor noise only, no music
      --volume N       volume, 0-100 (default 70)

      --fps N          frames per second (default 20)
      --size F         helicopter width as a fraction of the terminal (default 0.78)
      --quality N      supersampling 1-2 (default auto)
      --ascii          ASCII shading instead of half-block pixels
      --color MODE     auto|never|16|256|true (default "auto")

  -d, --duration DUR   fly for this long then exit (90m, 2h; bare numbers are minutes)
      --idle           no animation: hold the machine awake, at rest
      --idle-after DUR animate for this long, then drop to --idle

      --no-awake       do not hold a wake lock
      --no-display     let the display sleep; only block system idle sleep

      --seed N         scenery seed (0 = random)
      --snapshot       print one frame and exit
  -v, --version        print version and exit

keys:
  q, Ctrl-C  quit          t  next theme      m  mute / unmute
  space      pause         i  toggle idle     + / -  resize the helicopter

`)
}

func run() error {
	o := parseFlags()

	switch {
	case o.showVersion:
		fmt.Printf("helikopter %s (%s, built %s)\n", version, commit, date)
		return nil
	case o.listThemes:
		for _, t := range theme.All() {
			marker := "  "
			if t.Name == theme.DefaultName {
				marker = "* "
			}
			fmt.Printf("%s%-9s %s\n", marker, t.Name, t.Desc)
		}
		return nil
	}

	if o.seed == 0 {
		o.seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(o.seed))

	names := theme.Names()
	if strings.EqualFold(o.themeName, "random") {
		o.themeName = names[rng.Intn(len(names))]
	}
	th, err := theme.Get(o.themeName)
	if err != nil {
		return err
	}

	total, err := parseDur(o.duration)
	if err != nil {
		return fmt.Errorf("--duration: %w", err)
	}
	idleAfter, err := parseDur(o.idleAfter)
	if err != nil {
		return fmt.Errorf("--idle-after: %w", err)
	}

	tm := term.Open()
	defer tm.Cleanup()

	if o.snapshot {
		st := &state{opts: o, theme: th, tm: tm, themeNames: names}
		return st.renderSnapshot()
	}

	// Idle mode animates nothing and plays nothing, so it skips every bit of
	// that setup — no soundtrack is synthesised and no supervising lock is
	// taken — and hands off to the platform's own inhibitor instead.
	if o.idle || !tm.IsTTY() {
		return runIdle(o, total, !tm.IsTTY())
	}

	// A wake lock is the whole point, so take it before anything that can fail.
	lock := awake.Noop()
	awakeNote := "off"
	if !o.noWakeLock {
		l, err := awake.Acquire(awake.Options{Display: !o.noDisplay})
		if err != nil {
			awakeNote = "unavailable"
		} else {
			lock, awakeNote = l, "on"
		}
	}
	defer lock.Release()

	var player *audio.Player
	soundNote := "off"
	if !o.silence {
		p, err := audio.New(audio.Config{
			Music:  !o.noMusic,
			Rotor:  true,
			Volume: float64(o.volume) / 100,
		})
		if err != nil {
			soundNote = "unavailable"
		} else {
			player = p
			soundNote = "on"
			player.Start()
			defer player.Stop()
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	st := &state{
		opts: o, theme: th, tm: tm,
		awakeNote: awakeNote, soundNote: soundNote,
		player: player, silence: o.silence,
		themeNames: names,
		total:      total, idleAfter: idleAfter,
	}

	return st.runAnimated(sig)
}

// parseDur accepts Go duration strings; a bare number is read as minutes.
func parseDur(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(n * float64(time.Minute)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("cannot read %q as a duration", s)
	}
	return d, nil
}
