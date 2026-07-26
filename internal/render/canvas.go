package render

import (
	"bytes"
	"math"
	"strconv"

	"github.com/hammadsaedi/helikopter/internal/theme"
)

// Style selects how pixels become characters. Half-block packs two vertical
// pixels into one cell with U+2580; ASCII falls back to a luminance ramp.
type Style int

const (
	StyleHalfBlock Style = iota
	StyleASCII
)

// Canvas is an RGB pixel buffer, twice as tall as its terminal region in
// half-block style.
type Canvas struct {
	W, H int
	Px   []theme.RGB
}

// NewCanvas allocates a canvas of w×h pixels.
func NewCanvas(w, h int) *Canvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Canvas{W: w, H: h, Px: make([]theme.RGB, w*h)}
}

func (c *Canvas) Set(x, y int, col theme.RGB) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	c.Px[y*c.W+x] = col
}

func (c *Canvas) Get(x, y int) theme.RGB {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return theme.RGB{}
	}
	return c.Px[y*c.W+x]
}

// Blend composites col over the existing pixel with coverage a.
func (c *Canvas) Blend(x, y int, col theme.RGB, a float64) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H || a <= 0 {
		return
	}
	if a >= 1 {
		c.Px[y*c.W+x] = col
		return
	}
	i := y*c.W + x
	c.Px[i] = theme.Mix(c.Px[i], col, a)
}

func (c *Canvas) Fill(col theme.RGB) {
	for i := range c.Px {
		c.Px[i] = col
	}
}

type cell struct {
	glyph  rune
	fg, bg theme.RGB
	blank  bool
}

// Screen owns the pixel canvas plus the status line and emits the smallest
// byte stream that will bring the terminal up to date.
type Screen struct {
	Cols, Rows int // full terminal size
	artRows    int // rows given over to the animation
	statusRows int // rows reserved for the status area

	canvas *Canvas
	style  Style
	mode   theme.Mode

	prev []cell
	cur  []cell

	status [][]byte
	buf    bytes.Buffer
	esc    []byte

	full bool
}

// A short ramp whose glyphs rise monotonically in *visual density*. Long
// ramps look like texture noise at terminal resolution: neighbouring levels
// map to glyphs of similar weight but wildly different shape, and the eye
// reads shape first.
var ramp = []rune(" .:-=+*#%@")

// NewScreen reserves the last row or two for the status area.
func NewScreen(cols, rows int, style Style, mode theme.Mode) *Screen {
	s := &Screen{style: style, mode: mode}
	s.Resize(cols, rows)
	return s
}

func (s *Screen) Resize(cols, rows int) {
	if cols < 8 {
		cols = 8
	}
	if rows < 4 {
		rows = 4
	}
	s.Cols, s.Rows = cols, rows

	// Two rows when there is height to spare, so the key hints get a line of
	// their own instead of competing with the flight information.
	s.statusRows = 2
	if rows < 10 {
		s.statusRows = 1
	}
	s.artRows = rows - s.statusRows

	ph := s.artRows
	if s.style == StyleHalfBlock {
		ph *= 2
	}
	s.canvas = NewCanvas(cols, ph)

	n := cols * s.artRows
	s.prev = make([]cell, n)
	s.cur = make([]cell, n)
	for i := range s.prev {
		s.prev[i].blank = true
	}
	s.full = true
}

func (s *Screen) Canvas() *Canvas      { return s.canvas }
func (s *Screen) ArtRows() int         { return s.artRows }
func (s *Screen) StatusRows() int      { return s.statusRows }
func (s *Screen) SetStatus(l [][]byte) { s.status = l }
func (s *Screen) Invalidate()          { s.full = true }

func lum(c theme.RGB) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

func (s *Screen) composeCells() {
	switch s.style {
	case StyleHalfBlock:
		for r := 0; r < s.artRows; r++ {
			for x := 0; x < s.Cols; x++ {
				top := s.canvas.Get(x, r*2)
				bot := s.canvas.Get(x, r*2+1)
				c := cell{glyph: '▀', fg: top, bg: bot}
				if top == bot {
					// Identical halves render the same as a space on that
					// background, for fewer bytes.
					c.glyph, c.fg = ' ', bot
				}
				s.cur[r*s.Cols+x] = c
			}
		}
	default:
		// Stretch the frame's luminance across the whole ramp, otherwise a
		// scene that never reaches black or white only ever uses the middle
		// glyphs and the picture turns to mush.
		lo, hi := 1.0, 0.0
		for _, c := range s.canvas.Px {
			l := lum(c)
			if l < lo {
				lo = l
			}
			if l > hi {
				hi = l
			}
		}
		span := hi - lo
		if span < 1e-3 {
			span = 1
		}

		for r := 0; r < s.artRows; r++ {
			for x := 0; x < s.Cols; x++ {
				col := s.canvas.Get(x, r)
				t := (lum(col) - lo) / span
				// Lift the midtones so the ramp is used across its length
				// instead of piling everything at the dark end.
				t = math.Pow(t, 0.72)
				g := ramp[int(t*float64(len(ramp)-1)+0.5)]
				if s.mode == theme.ModeNone {
					s.cur[r*s.Cols+x] = cell{glyph: g, fg: theme.RGB{R: 255, G: 255, B: 255}}
				} else {
					s.cur[r*s.Cols+x] = cell{glyph: g, fg: col}
				}
			}
		}
	}
}

// Flush returns the byte delta since the previous frame.
func (s *Screen) Flush() []byte {
	s.composeCells()
	s.buf.Reset()

	noColor := s.mode == theme.ModeNone
	var curFg, curBg theme.RGB
	haveFg, haveBg := false, false

	for r := 0; r < s.artRows; r++ {
		moved := false
		for x := 0; x < s.Cols; x++ {
			i := r*s.Cols + x
			c := s.cur[i]
			if !s.full && !s.prev[i].blank && s.prev[i] == c {
				moved = true
				continue
			}
			if moved || x == 0 {
				s.buf.WriteString("\x1b[")
				s.buf.WriteString(strconv.Itoa(r + 1))
				s.buf.WriteByte(';')
				s.buf.WriteString(strconv.Itoa(x + 1))
				s.buf.WriteByte('H')
				moved = false
			}
			if !noColor {
				if !haveFg || c.fg != curFg {
					s.esc = theme.AppendFg(s.esc[:0], c.fg, s.mode)
					s.buf.Write(s.esc)
					curFg, haveFg = c.fg, true
				}
				if s.style == StyleHalfBlock || c.glyph == ' ' {
					if !haveBg || c.bg != curBg {
						s.esc = theme.AppendBg(s.esc[:0], c.bg, s.mode)
						s.buf.Write(s.esc)
						curBg, haveBg = c.bg, true
					}
				}
			}
			s.buf.WriteRune(c.glyph)
		}
	}

	// The status area is a row or two and changes constantly, so it is always
	// repainted rather than diffed.
	for i := 0; i < s.statusRows; i++ {
		s.buf.WriteString("\x1b[0m\x1b[")
		s.buf.WriteString(strconv.Itoa(s.artRows + 1 + i))
		s.buf.WriteString(";1H\x1b[2K")
		if i < len(s.status) {
			s.buf.Write(s.status[i])
		}
		s.buf.WriteString("\x1b[0m")
	}

	s.prev, s.cur = s.cur, s.prev
	s.full = false
	return s.buf.Bytes()
}
