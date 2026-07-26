package render

import (
	"bytes"
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

	canvas *Canvas
	style  Style
	mode   theme.Mode

	prev []cell
	cur  []cell

	status []byte
	buf    bytes.Buffer
	esc    []byte

	full bool
}

var ramp = []rune(" .`',:;\"~-+=<>ilv1cxjtfLCJUYXZO0Qoahkbdpqwm*WMB8&%$#@")

// NewScreen reserves the last terminal row for the status line.
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
	s.artRows = rows - 1

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

func (s *Screen) Canvas() *Canvas    { return s.canvas }
func (s *Screen) ArtRows() int       { return s.artRows }
func (s *Screen) SetStatus(b []byte) { s.status = b }
func (s *Screen) Invalidate()        { s.full = true }

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
		for r := 0; r < s.artRows; r++ {
			for x := 0; x < s.Cols; x++ {
				col := s.canvas.Get(x, r)
				g := ramp[int(lum(col)*float64(len(ramp)-1)+0.5)]
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

	// The status line is one row and changes constantly, so always repaint it.
	s.buf.WriteString("\x1b[0m\x1b[")
	s.buf.WriteString(strconv.Itoa(s.Rows))
	s.buf.WriteString(";1H\x1b[2K")
	s.buf.Write(s.status)
	s.buf.WriteString("\x1b[0m")

	s.prev, s.cur = s.cur, s.prev
	s.full = false
	return s.buf.Bytes()
}
