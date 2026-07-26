package render

import (
	"math"

	"github.com/hammadsaedi/helikopter/internal/art"
	"github.com/hammadsaedi/helikopter/internal/theme"
)

const horizonFrac = 0.76

// Scene draws the animated frame. Everything that does not change from frame to
// frame is computed once: the sky gradient and sun are baked into a background
// buffer, and each cloud band is a ring of pre-shaded columns that scrolls by
// whole pixels, so a steady-state frame evaluates noise for at most one new
// column per band rather than for every pixel.
type Scene struct {
	Theme *theme.Theme
	Seed  uint32
	Size  float64
	Super int

	// PixelAspect is how tall a canvas pixel is relative to its width. Half
	// blocks give square pixels (1); the ASCII ramp is one whole cell per
	// pixel, and a cell is about twice as tall as it is wide (2).
	PixelAspect float64

	// Minimal strips the scene back for the ASCII ramp, where every pixel
	// becomes a glyph and a full sky turns into a field of punctuation. Cloud,
	// stars and the sun come off, and the sky is pushed down to the blank end
	// of the ramp, leaving an airframe against open space.
	Minimal bool

	w, h int
	hy   float64

	bg    []theme.RGB
	stars []star
	bands []*cloudBand

	terrTop []float64
	frame   *art.Frame
}

type star struct {
	x, y  int
	base  float64
	speed float64
	phase float64
}

type cloudBand struct {
	y0, y1        int
	speed, sx, sy float64
	thresh, alpha float64
	seed          uint32
	col           []theme.RGB
	a             []float32
	rows, cols    int
	head          int
	world         int
	ready         bool
	envAt         []float64
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func smoothstep(a, b, x float64) float64 {
	if a == b {
		return 0
	}
	t := clamp((x-a)/(b-a), 0, 1)
	return t * t * (3 - 2*t)
}

// prepare rebuilds every cached layer. Called on the first frame and after a
// resize or theme change.
func (s *Scene) prepare(c *Canvas) {
	s.w, s.h = c.W, c.H
	s.hy = float64(c.H) * horizonFrac
	s.terrTop = make([]float64, c.W)

	s.bakeBackground()
	s.bakeStars()
	s.bakeBands()
}

// Reset drops all caches, forcing a rebuild on the next frame.
func (s *Scene) Reset() { s.w, s.h = 0, 0 }

func (s *Scene) bakeBackground() {
	th := s.Theme
	s.bg = make([]theme.RGB, s.w*s.h)
	for y := 0; y < s.h; y++ {
		col := th.Sky(clamp(float64(y)/math.Max(s.hy, 1), 0, 1))
		if s.Minimal {
			// Black, so the ramp leaves it blank and the airframe has open
			// space around it.
			col = theme.RGB{}
		}
		row := s.bg[y*s.w : (y+1)*s.w]
		for x := range row {
			row[x] = col
		}
	}
	if !th.OrbVisible || s.Minimal {
		return
	}

	ox, oy := th.OrbX*float64(s.w), th.OrbY*s.hy
	r := math.Max(float64(s.w)*0.035, 4)
	reach := r * 7
	for y := int(oy - reach); y <= int(oy+reach); y++ {
		if y < 0 || y >= s.h {
			continue
		}
		for x := int(ox - reach); x <= int(ox+reach); x++ {
			if x < 0 || x >= s.w {
				continue
			}
			d := math.Hypot(float64(x)-ox, float64(y)-oy)
			if d > reach {
				continue
			}
			i := y*s.w + x
			if g := math.Pow(1-d/reach, 2.6); g > 0.004 {
				s.bg[i] = theme.Mix(s.bg[i], th.OrbGlow, g*0.55)
			}
			if d <= r {
				s.bg[i] = theme.Mix(s.bg[i], th.Orb, smoothstep(r, r-1.4, d))
			}
		}
	}
}

func (s *Scene) bakeStars() {
	s.stars = nil
	if !s.Theme.Stars || s.Minimal {
		return
	}
	n := s.w * int(s.hy) / 190
	s.stars = make([]star, 0, n)
	for i := 0; i < n; i++ {
		h := hashU32(uint32(i)*2654435761 + s.Seed)
		x := int(h % uint32(s.w))
		y := int((h / 7919) % uint32(math.Max(s.hy-2, 1)))
		depth := 1 - float64(y)/math.Max(s.hy, 1)
		s.stars = append(s.stars, star{
			x: x, y: y,
			base:  clamp(0.30+0.70*depth, 0, 1),
			speed: 1.1 + float64(i%7)*0.31,
			phase: float64(i),
		})
	}
}

func (s *Scene) bakeBands() {
	s.bands = nil
	if s.Minimal {
		return
	}
	specs := []cloudBand{
		{y0: int(0.04 * s.hy), y1: int(0.34 * s.hy), speed: 1.6, sx: 0.055, sy: 0.16, thresh: 0.545, alpha: 0.85, seed: s.Seed + 11},
		{y0: int(0.28 * s.hy), y1: int(0.62 * s.hy), speed: 4.2, sx: 0.038, sy: 0.20, thresh: 0.575, alpha: 0.95, seed: s.Seed + 29},
	}
	s.bands = nil
	for i := range specs {
		b := specs[i]
		if b.y1 >= s.h {
			b.y1 = s.h - 1
		}
		if b.y1 <= b.y0 {
			continue
		}
		b.rows = b.y1 - b.y0 + 1
		b.cols = s.w
		b.col = make([]theme.RGB, b.rows*b.cols)
		b.a = make([]float32, b.rows*b.cols)

		// Vertical envelope, identical for every column.
		b.envAt = make([]float64, b.rows)
		y0f, y1f := float64(b.y0), float64(b.y1)
		for r := 0; r < b.rows; r++ {
			y := y0f + float64(r)
			b.envAt[r] = smoothstep(y0f, y0f+(y1f-y0f)*0.35, y) *
				(1 - smoothstep(y0f+(y1f-y0f)*0.6, y1f, y))
		}
		s.bands = append(s.bands, &b)
	}
}

// fillColumn computes one world column of cloud into ring slot idx.
func (b *cloudBand) fillColumn(th *theme.Theme, world, idx int) {
	wx := float64(world) * b.sx
	for r := 0; r < b.rows; r++ {
		i := r*b.cols + idx
		env := b.envAt[r]
		if env <= 0.01 {
			b.a[i] = 0
			continue
		}
		y := float64(b.y0 + r)
		d := fbm2(wx, y*b.sy, b.seed, 4)
		if d <= b.thresh {
			b.a[i] = 0
			continue
		}
		a := smoothstep(b.thresh, b.thresh+0.10, d) * env * b.alpha
		if a <= 0.01 {
			b.a[i] = 0
			continue
		}
		up := fbm2(wx, (y-3)*b.sy, b.seed, 4)
		b.col[i] = theme.Mix(th.CloudShade, th.Cloud, clamp(0.5+(d-up)*3.2, 0, 1))
		b.a[i] = float32(a)
	}
}

// advance scrolls the band to time t, computing only newly exposed columns.
func (b *cloudBand) advance(th *theme.Theme, t float64) {
	target := int(t * b.speed)
	if !b.ready {
		for i := 0; i < b.cols; i++ {
			b.fillColumn(th, target+i, i)
		}
		b.head, b.world, b.ready = 0, target, true
		return
	}
	steps := target - b.world
	if steps <= 0 {
		return
	}
	if steps >= b.cols {
		for i := 0; i < b.cols; i++ {
			b.fillColumn(th, target+i, i)
		}
		b.head, b.world = 0, target
		return
	}
	for n := 0; n < steps; n++ {
		b.fillColumn(th, b.world+b.cols, b.head)
		b.head = (b.head + 1) % b.cols
		b.world++
	}
}

func (b *cloudBand) blit(c *Canvas) {
	for r := 0; r < b.rows; r++ {
		y := b.y0 + r
		base := r * b.cols
		for x := 0; x < b.cols; x++ {
			i := base + (b.head+x)%b.cols
			if a := b.a[i]; a > 0 {
				c.Blend(x, y, b.col[i], float64(a))
			}
		}
	}
}

// Render composites one frame. t is elapsed seconds.
func (s *Scene) Render(c *Canvas, t float64) {
	if s.w != c.W || s.h != c.H || s.bg == nil {
		s.prepare(c)
	}
	copy(c.Px, s.bg)

	for _, st := range s.stars {
		tw := 0.55 + 0.45*math.Sin(t*st.speed+st.phase)
		c.Blend(st.x, st.y, s.Theme.Star, st.base*tw)
	}
	for _, b := range s.bands {
		b.advance(s.Theme, t)
		b.blit(c)
	}
	s.terrain(c, t)

	W, H := float64(c.W), float64(c.H)
	bob := math.Sin(t*1.15)*0.022 + math.Sin(t*2.31+1.7)*0.009
	sway := math.Sin(t*0.61+0.4) * 0.012
	pitch := math.Sin(t*1.15+math.Pi/2) * 0.030

	fw := int(W * s.Size)
	if fw < 24 {
		fw = 24
	}
	pa := s.PixelAspect
	if pa <= 0 {
		pa = 1
	}
	fh := int(float64(fw) / art.AspectRatio() / pa)
	if fh < 6 {
		fh = 6
	}
	if float64(fh) > H*0.92 {
		fh = int(H * 0.92)
		fw = int(float64(fh) * art.AspectRatio() * pa)
	}

	s.frame = art.RasterizeInto(s.frame, fw, fh, art.Options{
		RotorPhase: t * 2.15 * 2 * math.Pi,
		TailPhase:  t * 5.30 * 2 * math.Pi,
		Pitch:      pitch,
		Shutter:    0.50,
		BeaconOn:   math.Mod(t, 1.30) < 0.13,
		StrobeOn:   math.Mod(t+0.65, 1.30) < 0.07,
		Super:      s.Super,
	})

	ox := int(W*(0.47+sway)) - fw/2
	oy := int(H*(0.40+bob)) - fh/2

	if !s.Minimal {
		// In ramp mode the wash lands as scattered speckle around the
		// aircraft rather than as moving air.
		s.downwash(c, ox, oy, fw, fh, t)
	}
	s.blitHeli(c, ox, oy)
}

func (s *Scene) terrain(c *Canvas, t float64) {
	th := s.Theme
	H := float64(c.H)
	hy := s.hy

	type layer struct {
		col         theme.RGB
		amp, wl     float64
		speed, lift float64
		haze        float64
		seed        uint32
		oct         int
		// crest is the brightness this layer's horizon line takes in Minimal
		// mode, chosen so the three ridges land on distinct ramp steps and
		// read as depth rather than as three bright bars.
		crest float64
	}
	layers := [...]layer{
		{th.Ridge, H * 0.130, 26, 2.5, 0.005, 0.55, s.Seed + 101, 4, 0.17},
		{th.Hills, H * 0.070, 15, 8.0, 0.030, 0.28, s.Seed + 211, 3, 0.27},
		{th.Ground, H * 0.045, 11, 26.0, 0.070, 0.00, s.Seed + 307, 2, 0.40},
	}

	for li, l := range layers {
		// One horizon in ramp mode. Three crest lines break into dashes and
		// read as scattered debris rather than as distance.
		if s.Minimal && li != len(layers)-1 {
			continue
		}
		prevY := -1
		for x := 0; x < c.W; x++ {
			n := fbm1((float64(x)+t*l.speed)/l.wl, l.seed, l.oct)
			top := hy + l.lift*H - l.amp*n
			y0 := int(top)
			if y0 < 0 {
				y0 = 0
			}

			if s.Minimal {
				// Just the crest: filling underneath turned the bottom third
				// of the screen into a field of dots. Each column is bridged
				// to the previous one so the horizon reads as a continuous
				// line rather than a row of dashes.
				crest := theme.AtLuminance(l.col, l.crest)
				lo, hi := y0, y0
				if prevY >= 0 {
					if prevY < lo {
						lo = prevY
					}
					if prevY > hi {
						hi = prevY
					}
				}
				for y := lo; y <= hi; y++ {
					if y >= 0 && y < c.H {
						c.Set(x, y, crest)
					}
				}
				prevY = y0
				continue
			}

			for y := y0; y < c.H; y++ {
				col := l.col
				if l.haze > 0 {
					f := 1 - clamp((float64(y)-top)/(H*0.18), 0, 1)
					col = theme.Mix(col, th.SkyHorizon, l.haze*f)
				}
				col = theme.Mix(col, theme.RGB{}, clamp((float64(y)-top)/(H*0.30), 0, 1)*0.22)
				c.Set(x, y, col)
			}
		}
	}

	if th.Grid != (theme.RGB{}) && !s.Minimal {
		s.grid(c, t)
	}
}

func (s *Scene) grid(c *Canvas, t float64) {
	th := s.Theme
	H, W := float64(c.H), float64(c.W)
	base := s.hy + 0.07*H
	if base >= H {
		return
	}
	for i := 1; i < 40; i++ {
		z := float64(i) - math.Mod(t*0.55, 1)
		y := base + (H-base)*(1-1/(1+z*0.42))
		if y < base || y >= H {
			continue
		}
		a := clamp((y-base)/(H-base)*1.5, 0.08, 0.75)
		for x := 0; x < c.W; x++ {
			c.Blend(x, int(y), th.Grid, a)
		}
	}
	for i := -14; i <= 14; i++ {
		for y := int(base); y < c.H; y++ {
			p := (float64(y) - base) / math.Max(H-base, 1)
			x := W/2 + float64(i)*p*W*0.085
			if x >= 0 && x < W {
				c.Blend(int(x), y, th.Grid, clamp(p*0.6, 0.05, 0.55))
			}
		}
	}
}

func (s *Scene) downwash(c *Canvas, ox, oy, fw, fh int, t float64) {
	th := s.Theme
	hubX, hubY := art.RotorHub(fw, fh)
	cx := float64(ox) + hubX
	top := float64(oy) + hubY + float64(fh)*0.42
	span := float64(fw) * 0.34

	for y := int(top); y < c.H; y++ {
		if y < 0 {
			continue
		}
		fall := (float64(y) - top) / math.Max(s.hy-top, 1)
		if fall < 0 {
			continue
		}
		fade := (1 - clamp(fall, 0, 1)) * 0.30
		if fade <= 0.01 {
			continue
		}
		spread := span * (1 + fall*0.85)
		for x := int(cx - spread); x <= int(cx+spread); x++ {
			off := math.Abs(float64(x)-cx) / spread
			if off > 1 {
				continue
			}
			n := fbm2(float64(x)*0.30, (float64(y)-t*46)*0.11, s.Seed+77, 2)
			a := (n - 0.52) * 5 * fade * (1 - off*off)
			if s.Minimal {
				a *= 0.4 // the ramp turns faint wash into distracting speckle
			}
			if a > 0.01 {
				c.Blend(x, y, th.Wash, clamp(a, 0, 0.5))
			}
		}
	}
}

func (s *Scene) blitHeli(c *Canvas, ox, oy int) {
	th := s.Theme
	f := s.frame
	for y := 0; y < f.H; y++ {
		for x := 0; x < f.W; x++ {
			p := f.At(x, y)
			if p.Mat == art.MatNone || p.Alpha <= 0 {
				continue
			}
			a := p.Alpha
			if p.Mat == art.MatRotor {
				a *= 0.88
			}
			col := theme.Scale(th.Mat[p.Mat], p.Shade, p.Spec)
			if s.Minimal {
				col = rampShade(f, x, y, p, col)
			}
			c.Blend(ox+x, oy+y, col, a)
		}
	}
}

// rampShade draws the airframe as line art rather than as tone.
//
// Smooth shading is what makes the colour render look solid, and it is exactly
// what a character ramp cannot carry: every lit surface saturates to the same
// two or three dense glyphs and the aircraft reads as a blob. So outlines —
// the silhouette and the boundaries between parts — go bright, and interiors
// stay dim with just enough variation to suggest the form underneath.
func rampShade(f *art.Frame, x, y int, p art.Pixel, col theme.RGB) theme.RGB {
	if p.Mat == art.MatRotor {
		return theme.AtLuminance(col, 0.18+0.34*p.Alpha)
	}
	if isOutline(f, x, y, p.Mat) {
		return theme.AtLuminance(col, 0.98)
	}
	return theme.AtLuminance(col, 0.05+0.08*clamp(p.Shade/1.2, 0, 1))
}

// isOutline reports whether the pixel sits on the silhouette or on a boundary
// between two parts of the airframe.
func isOutline(f *art.Frame, x, y int, mat art.Material) bool {
	// Window frames, pillars and panel lines are one or two pixels wide at
	// terminal resolution. Outlining them puts a bright mark on both sides of
	// every one and the aircraft fills in solid, so they are not boundaries.
	if mat == art.MatFrame {
		return false
	}
	g := art.Group(mat)
	for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		n := f.At(x+d[0], y+d[1])
		if n.Mat == art.MatFrame {
			continue
		}
		ng := art.Group(n.Mat)
		if ng == g {
			continue
		}
		// The rotor sweeps across the airframe; treating it as a boundary
		// would outline the whole disc, differently, every frame.
		if ng == art.Group(art.MatRotor) && n.Alpha < 0.9 {
			continue
		}
		if n.Mat == art.MatNone && n.Alpha < 0.5 {
			return true // silhouette
		}
		return true // a different part of the aircraft
	}
	return false
}
