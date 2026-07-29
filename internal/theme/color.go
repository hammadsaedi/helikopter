package theme

import (
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// RGB is a 24-bit colour. Themes are authored in truecolor and degraded down to
// whatever the terminal can actually show.
type RGB struct{ R, G, B uint8 }

// Mode is the colour depth the terminal supports.
type Mode int

const (
	ModeNone Mode = iota // no colour at all
	Mode16               // the original eight, plus bright
	Mode256              // xterm indexed
	ModeTrue             // 24-bit
)

func (m Mode) String() string {
	switch m {
	case ModeTrue:
		return "truecolor"
	case Mode256:
		return "256"
	case Mode16:
		return "16"
	default:
		return "none"
	}
}

// ParseMode maps a --color flag value onto a Mode. "auto" returns false so the
// caller knows to fall back to detection.
func ParseMode(s string) (Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "never", "none", "off", "no", "0":
		return ModeNone, true
	case "16", "ansi":
		return Mode16, true
	case "256":
		return Mode256, true
	case "true", "truecolor", "24bit", "rgb", "always":
		return ModeTrue, true
	default:
		return ModeTrue, false
	}
}

// DetectMode works out the colour depth from the environment. It deliberately
// honours NO_COLOR (https://no-color.org) before anything else.
func DetectMode(isTTY bool) Mode {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return ModeNone
	}
	term := os.Getenv("TERM")
	if term == "dumb" || (term == "" && !isTTY) {
		return ModeNone
	}
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	if strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit") {
		return ModeTrue
	}
	// Terminals that are known-good but don't always advertise COLORTERM.
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "vscode", "Hyper", "ghostty", "Apple_Terminal":
		if os.Getenv("TERM_PROGRAM") == "Apple_Terminal" {
			return Mode256 // Terminal.app still tops out at 256
		}
		return ModeTrue
	}
	if os.Getenv("WT_SESSION") != "" { // Windows Terminal
		return ModeTrue
	}
	// Modern Windows consoles (PowerShell, cmd on Windows 10 1607+) support
	// 24-bit colour via Virtual Terminal Sequences, but do not advertise it
	// through COLORTERM, TERM, or WT_SESSION the way Unix terminals do.
	if runtime.GOOS == "windows" && isTTY {
		return ModeTrue
	}
	if strings.Contains(term, "256") {
		return Mode256
	}
	if term == "" {
		return Mode16
	}
	return Mode16
}

// ---------------------------------------------------------------------------
// escape sequences
// ---------------------------------------------------------------------------

// Reset clears all attributes.
const Reset = "\x1b[0m"

// AppendFg writes a foreground colour escape for c into dst.
func AppendFg(dst []byte, c RGB, m Mode) []byte { return appendColor(dst, c, m, true) }

// AppendBg writes a background colour escape for c into dst.
func AppendBg(dst []byte, c RGB, m Mode) []byte { return appendColor(dst, c, m, false) }

func appendColor(dst []byte, c RGB, m Mode, fg bool) []byte {
	switch m {
	case ModeNone:
		return dst
	case ModeTrue:
		if fg {
			dst = append(dst, "\x1b[38;2;"...)
		} else {
			dst = append(dst, "\x1b[48;2;"...)
		}
		dst = strconv.AppendUint(dst, uint64(c.R), 10)
		dst = append(dst, ';')
		dst = strconv.AppendUint(dst, uint64(c.G), 10)
		dst = append(dst, ';')
		dst = strconv.AppendUint(dst, uint64(c.B), 10)
		return append(dst, 'm')
	case Mode256:
		if fg {
			dst = append(dst, "\x1b[38;5;"...)
		} else {
			dst = append(dst, "\x1b[48;5;"...)
		}
		dst = strconv.AppendUint(dst, uint64(to256(c)), 10)
		return append(dst, 'm')
	default:
		idx := to16(c)
		code := 30 + idx%8
		if !fg {
			code = 40 + idx%8
		}
		if idx >= 8 {
			code += 60
		}
		dst = append(dst, "\x1b["...)
		dst = strconv.AppendInt(dst, int64(code), 10)
		return append(dst, 'm')
	}
}

// Fg returns a foreground escape as a string, for low-frequency use such as the
// status bar and help text.
func Fg(c RGB, m Mode) string { return string(AppendFg(nil, c, m)) }

// Bg returns a background escape as a string.
func Bg(c RGB, m Mode) string { return string(AppendBg(nil, c, m)) }

// to256 maps a colour onto the xterm palette, choosing between the 6×6×6 cube
// and the 24-step greyscale ramp by whichever lands closer.
func to256(c RGB) uint8 {
	q := func(v uint8) int {
		if v < 48 {
			return 0
		}
		if v < 115 {
			return 1
		}
		return int((float64(v) - 35) / 40)
	}
	levels := []int{0, 95, 135, 175, 215, 255}
	ri, gi, bi := q(c.R), q(c.G), q(c.B)
	if ri > 5 {
		ri = 5
	}
	if gi > 5 {
		gi = 5
	}
	if bi > 5 {
		bi = 5
	}
	cubeErr := sq(levels[ri]-int(c.R)) + sq(levels[gi]-int(c.G)) + sq(levels[bi]-int(c.B))

	grey := (int(c.R)*299 + int(c.G)*587 + int(c.B)*114) / 1000
	gi2 := (grey - 8) / 10
	if gi2 < 0 {
		gi2 = 0
	}
	if gi2 > 23 {
		gi2 = 23
	}
	gv := 8 + gi2*10
	greyErr := sq(gv-int(c.R)) + sq(gv-int(c.G)) + sq(gv-int(c.B))

	if greyErr < cubeErr {
		return uint8(232 + gi2)
	}
	return uint8(16 + 36*ri + 6*gi + bi)
}

var basic16 = [16]RGB{
	{0, 0, 0}, {170, 0, 0}, {0, 170, 0}, {170, 85, 0},
	{0, 0, 170}, {170, 0, 170}, {0, 170, 170}, {170, 170, 170},
	{85, 85, 85}, {255, 85, 85}, {85, 255, 85}, {255, 255, 85},
	{85, 85, 255}, {255, 85, 255}, {85, 255, 255}, {255, 255, 255},
}

func to16(c RGB) int {
	best, bestD := 0, math.MaxInt
	for i, p := range basic16 {
		d := sq(int(p.R)-int(c.R)) + sq(int(p.G)-int(c.G)) + sq(int(p.B)-int(c.B))
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

func sq(v int) int { return v * v }

// ---------------------------------------------------------------------------
// colour maths
// ---------------------------------------------------------------------------

// Mix blends a into b by t (0 → a, 1 → b).
func Mix(a, b RGB, t float64) RGB {
	t = clamp01(t)
	return RGB{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
	}
}

// Scale multiplies a colour by an intensity, with an additive term for
// specular highlights. Both are clamped into range.
func Scale(c RGB, shade, add float64) RGB {
	f := func(v uint8) uint8 {
		x := float64(v)*shade + add*255
		if x < 0 {
			x = 0
		}
		if x > 255 {
			x = 255
		}
		return uint8(x)
	}
	return RGB{f(c.R), f(c.G), f(c.B)}
}

// Luminance is the perceived brightness of c, 0..1.
func Luminance(c RGB) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// AtLuminance returns c at a target brightness.
//
// It blends toward white or black rather than scaling the channels, because
// scaling cannot reach a high target from a saturated colour: multiplying
// crimson (luminance 0.27) by 3.7 just clips the channels at 255 and lands at
// 0.58, not the 0.98 asked for. Line art picks glyphs by brightness, so a red
// aircraft came out several ramp steps darker than a white one and the drawing
// fell apart. Blending hits the target exactly, at the cost of some saturation.
func AtLuminance(c RGB, target float64) RGB {
	target = clamp01(target)
	l := Luminance(c)
	switch {
	case l < 1e-4:
		v := uint8(target*255 + 0.5)
		return RGB{v, v, v}
	case target > l:
		return Mix(c, RGB{255, 255, 255}, (target-l)/(1-l))
	default:
		return Mix(RGB{}, c, target/l)
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
