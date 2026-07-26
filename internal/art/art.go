// Package art rasterizes a parametric helicopter into a material buffer.
//
// The helicopter is not stored as ASCII frames. It is defined in a normalised
// model space by a set of implicit shapes, then sampled at whatever resolution
// the terminal happens to be. That buys three things a hand-drawn sprite can't:
// smooth surface shading from a real light direction, a rotor that genuinely
// rotates instead of cycling through poses, and art that stays crisp from an
// 80x24 window up to a full-screen terminal.
package art

import "math"

// Material identifies which part of the airframe a pixel belongs to. Themes map
// materials to colours, so the same geometry renders as a gunship or a news
// chopper without touching this file.
type Material uint8

const (
	MatNone Material = iota
	MatHull
	MatBelly
	MatCanopy
	MatFrame
	MatEngine
	MatExhaust
	MatFin
	MatSkid
	MatRotor
	MatHub
	MatNavRed
	MatNavGreen
	MatBeacon
	MatCount
)

// Group buckets materials into the parts a viewer would name. Line-art
// rendering outlines boundaries between groups, so panel lines, window frames
// and louvres do not each get an outline of their own and fill the aircraft in.
func Group(m Material) uint8 {
	switch m {
	case MatNone:
		return 0
	case MatHull, MatBelly, MatEngine, MatFin, MatFrame:
		return 1
	case MatCanopy:
		return 2
	case MatSkid:
		return 3
	case MatRotor:
		return 4
	case MatHub:
		return 5
	case MatExhaust:
		return 6
	default:
		return 7 // navigation lights
	}
}

// Pixel is one sample of the airframe. Alpha is coverage, which is fractional
// at silhouette edges (from supersampling) and along the rotor disc (from
// motion blur), so the compositor can blend it against the sky.
type Pixel struct {
	Mat   Material
	Shade float64 // diffuse + ambient + rim, roughly 0..1.4
	Spec  float64 // specular highlight, added on top of the material colour
	Alpha float64
}

// Frame is a rasterised airframe, row-major, origin top-left.
type Frame struct {
	W, H int
	Px   []Pixel

	// blades is scratch space for the rotor tips, kept here so a reused frame
	// reuses it too and a steady-state render allocates nothing.
	blades []bladeTip
}

func newFrame(w, h int) *Frame {
	return &Frame{W: w, H: h, Px: make([]Pixel, w*h)}
}

// At returns the pixel at (x, y), or a transparent pixel when out of bounds.
func (f *Frame) At(x, y int) Pixel {
	if x < 0 || y < 0 || x >= f.W || y >= f.H {
		return Pixel{}
	}
	return f.Px[y*f.W+x]
}

// ---------------------------------------------------------------------------
// small maths helpers
// ---------------------------------------------------------------------------

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

// lerpProfile samples a piecewise curve defined by ascending control points,
// smoothstep-interpolated between them. Used for the fuselage silhouette.
func lerpProfile(xs, ys []float64, x float64) float64 {
	n := len(xs)
	if x <= xs[0] {
		return ys[0]
	}
	if x >= xs[n-1] {
		return ys[n-1]
	}
	for i := 0; i < n-1; i++ {
		if x <= xs[i+1] {
			t := smoothstep(xs[i], xs[i+1], x)
			return ys[i] + (ys[i+1]-ys[i])*t
		}
	}
	return ys[n-1]
}

// Distances are compared against thresholds far more often than they are
// needed as values, so the helpers below return squared distances. That keeps
// square roots out of the per-pixel path entirely.

// distToSegment2 is the squared distance from p to the segment ab.
func distToSegment2(px, py, ax, ay, bx, by float64) float64 {
	_, d2 := segmentT2(px, py, ax, ay, bx, by)
	return d2
}

// segmentT2 returns the normalised position (0..1) of the projection of p onto
// ab, alongside the squared distance to it.
func segmentT2(px, py, ax, ay, bx, by float64) (t, dist2 float64) {
	vx, vy := bx-ax, by-ay
	wx, wy := px-ax, py-ay
	den := vx*vx + vy*vy
	if den == 0 {
		return 0, wx*wx + wy*wy
	}
	t = clamp((wx*vx+wy*vy)/den, 0, 1)
	dx, dy := px-(ax+t*vx), py-(ay+t*vy)
	return t, dx*dx + dy*dy
}

func hypot2(dx, dy float64) float64 { return dx*dx + dy*dy }

// inPolygon does a standard even-odd crossing test.
func inPolygon(px, py float64, poly [][2]float64) bool {
	in := false
	n := len(poly)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := poly[i][0], poly[i][1]
		xj, yj := poly[j][0], poly[j][1]
		if (yi > py) != (yj > py) && px < (xj-xi)*(py-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

// polyEdgeDist is the distance from p to the nearest edge of poly. Used to
// bevel flat plates so they read as surfaces rather than cut-outs.
func polyEdgeDist(px, py float64, poly [][2]float64) float64 {
	best := math.MaxFloat64
	n := len(poly)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		if d := distToSegment2(px, py, poly[i][0], poly[i][1], poly[j][0], poly[j][1]); d < best {
			best = d
		}
	}
	return math.Sqrt(best)
}

// box is an axis-aligned bounding box used to reject polygon tests cheaply.
type box struct{ x0, y0, x1, y1 float64 }

func (b box) contains(x, y float64) bool {
	return x >= b.x0 && x <= b.x1 && y >= b.y0 && y <= b.y1
}

func bboxOf(poly [][2]float64) box {
	b := box{math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64, -math.MaxFloat64}
	for _, p := range poly {
		b.x0 = math.Min(b.x0, p[0])
		b.y0 = math.Min(b.y0, p[1])
		b.x1 = math.Max(b.x1, p[0])
		b.y1 = math.Max(b.y1, p[1])
	}
	return b
}

func roundedRect(px, py, x0, y0, x1, y1, r float64) bool {
	if px < x0 || px > x1 || py < y0 || py > y1 {
		return false
	}
	cx := clamp(px, x0+r, x1-r)
	cy := clamp(py, y0+r, y1-r)
	return hypot2(px-cx, py-cy) <= r*r
}

// onRoundedRectEdge reports whether the point sits within w of the outline.
func onRoundedRectEdge(px, py, x0, y0, x1, y1, r, w float64) bool {
	return roundedRect(px, py, x0-w, y0-w, x1+w, y1+w, r+w) &&
		!roundedRect(px, py, x0+w, y0+w, x1-w, y1-w, math.Max(0, r-w))
}
