package art

import "math"

// The airframe lives in a normalised model space: the nose sits at x ≈ -1
// (the helicopter faces left, as it did in the original two-frame version),
// the tail rotor at x ≈ +1.45, and y is up with the cabin centreline at 0.
//
// Everything below is a side elevation of a light utility helicopter: chin
// bubble and wrap-around windscreen, cabin with a sliding door, engine cowl
// feeding an exhaust nozzle, tapered tail boom, swept fin carrying the tail
// rotor, and skid gear.

// Extents of the model, including rotor sweep. Used to fit it to the terminal.
const (
	modelMinX = -1.72
	modelMaxX = 1.68
	modelMinY = -0.54
	modelMaxY = 0.70
)

// Fuselage silhouette. topY/botY are sampled against noseX..tailX and
// smoothstep-interpolated; the pair defines both the outline and, via their
// midpoint and half-height, the generalised cylinder used for shading.
// The cabin runs to about x = 0.3 and then pinches into a slender boom, which
// is the proportion that reads as "helicopter" rather than "fish".
var (
	profX = []float64{-1.00, -0.94, -0.86, -0.72, -0.55, -0.32, -0.10, 0.08, 0.22, 0.36, 0.52, 0.75, 1.05, 1.42}
	profT = []float64{0.020, 0.110, 0.185, 0.245, 0.285, 0.302, 0.296, 0.262, 0.212, 0.160, 0.126, 0.112, 0.108, 0.106}
	profB = []float64{-0.020, -0.115, -0.195, -0.255, -0.295, -0.302, -0.284, -0.222, -0.130, -0.030, 0.028, 0.050, 0.052, 0.052}
)

// Windscreen lower edge: the glass wraps under the nose into a chin bubble.
var (
	glassX = []float64{-1.00, -0.90, -0.75, -0.60, -0.46}
	glassY = []float64{-0.010, -0.095, -0.130, -0.115, -0.040}
)

var finPoly = [][2]float64{
	{1.02, 0.09}, {1.20, 0.40}, {1.30, 0.50}, {1.54, 0.50}, {1.56, 0.36}, {1.44, 0.14}, {1.24, 0.06},
}

var finBox = bboxOf(finPoly)
var stabBox = bboxOf(stabPoly)

var stabPoly = [][2]float64{
	{0.92, 0.030}, {1.32, 0.046}, {1.32, 0.100}, {0.92, 0.088},
}

const (
	mainHubX, mainHubY = -0.04, 0.500
	mainRotorR         = 1.62
	mainRotorTilt      = 0.085 // vertical squash: we view the disc almost edge-on
	mainRotorConing    = 0.030
	mainBlades         = 4

	tailHubX, tailHubY = 1.44, 0.360
	tailRotorR         = 0.175
	tailBlades         = 4
)

// Options controls a single rasterised frame.
type Options struct {
	RotorPhase float64 // main rotor angle, radians
	TailPhase  float64 // tail rotor angle, radians
	Pitch      float64 // nose-down pitch, radians
	Shutter    float64 // rotor sweep captured per frame, radians (motion blur)
	BeaconOn   bool    // anti-collision beacon lit this frame
	StrobeOn   bool    // belly strobe lit this frame
	Super      int     // supersampling factor per axis (1..3)

	// blades holds the main rotor's blade tips for every shutter sub-step.
	// They depend only on the frame, not on the pixel, so RasterizeInto fills
	// this once instead of re-running 24 sin/cos calls per sample.
	blades []bladeTip
	steps  int
}

type bladeTip struct{ x, y float64 }

func (o *Options) precomputeBlades() {
	o.steps = 6
	if o.Shutter <= 0 {
		o.steps = 1
	}
	need := o.steps * mainBlades
	if cap(o.blades) < need {
		o.blades = make([]bladeTip, need)
	}
	o.blades = o.blades[:need]

	i := 0
	for s := 0; s < o.steps; s++ {
		ph := o.RotorPhase
		if o.steps > 1 {
			ph += o.Shutter * (float64(s)/float64(o.steps-1) - 0.5)
		}
		for b := 0; b < mainBlades; b++ {
			th := ph + float64(b)*2*math.Pi/mainBlades
			o.blades[i] = bladeTip{
				x: mainHubX + mainRotorR*math.Cos(th),
				y: mainHubY + mainRotorR*mainRotorTilt*math.Sin(th) + mainRotorConing,
			}
			i++
		}
	}
}

// The fuselage profile is sampled several times per pixel, so it is baked
// into a table once and read with a linear interpolation instead of walking
// the control points on every call.
const lutN = 2048

var (
	lutTop, lutBot, lutDHalf [lutN + 1]float64
	lutX0, lutX1, lutInvStep float64
)

func init() {
	lutX0, lutX1 = profX[0], profX[len(profX)-1]
	step := (lutX1 - lutX0) / lutN
	lutInvStep = 1 / step
	for i := 0; i <= lutN; i++ {
		x := lutX0 + float64(i)*step
		lutTop[i] = lerpProfile(profX, profT, x)
		lutBot[i] = lerpProfile(profX, profB, x)
	}
	const d = 0.02
	for i := 0; i <= lutN; i++ {
		x := lutX0 + float64(i)*step
		hi := lerpProfile(profX, profT, x+d) - lerpProfile(profX, profB, x+d)
		lo := lerpProfile(profX, profT, x-d) - lerpProfile(profX, profB, x-d)
		lutDHalf[i] = (hi - lo) / 2 / (2 * d)
	}
}

func lutAt(tab *[lutN + 1]float64, x float64) float64 {
	if x <= lutX0 {
		return tab[0]
	}
	if x >= lutX1 {
		return tab[lutN]
	}
	f := (x - lutX0) * lutInvStep
	i := int(f)
	return tab[i] + (tab[i+1]-tab[i])*(f-float64(i))
}

func bodyTop(x float64) float64 { return lutAt(&lutTop, x) }
func bodyBot(x float64) float64 { return lutAt(&lutBot, x) }

func inBody(x, y float64) bool {
	if x < profX[0] || x > profX[len(profX)-1] {
		return false
	}
	return y >= bodyBot(x) && y <= bodyTop(x)
}

// cowlTop is the engine deck: a bump grafted onto the fuselage top behind the
// rotor mast, tapering into the exhaust.
func cowlTop(x float64) float64 {
	bump := smoothstep(-0.24, -0.08, x) * (1 - smoothstep(0.18, 0.36, x))
	return bodyTop(x) + 0.125*bump
}

func inCowl(x, y float64) bool {
	if x < -0.24 || x > 0.36 {
		return false
	}
	return y >= bodyTop(x)-0.03 && y <= cowlTop(x)
}

// ---------------------------------------------------------------------------
// lighting
// ---------------------------------------------------------------------------

// Key light: above, ahead of, and slightly toward the viewer.
var lx, ly, lz = normalize3(-0.50, 0.62, 0.60)

// Half-vector for the specular term, with the eye at +z.
var hx, hy, hz = normalize3(lx, ly, lz+1)

func normalize3(x, y, z float64) (float64, float64, float64) {
	n := math.Sqrt(x*x + y*y + z*z)
	if n == 0 {
		return 0, 0, 1
	}
	return x / n, y / n, z / n
}

// shadeNormal turns a surface normal into a lit intensity plus a specular term.
func shadeNormal(nx, ny, nz, ambient, diffuse, rim, specK float64, specP int) (float64, float64) {
	d := nx*lx + ny*ly + nz*lz
	if d < 0 {
		d = 0
	}
	e := 1 - clamp(nz, 0, 1)
	sh := ambient + diffuse*d + rim*e*e*e

	var sp float64
	if specK > 0 {
		if s := nx*hx + ny*hy + nz*hz; s > 0 {
			sp = specK * ipow(s, specP)
		}
	}
	return sh, sp
}

// ipow is exponentiation by squaring; the specular exponents are all small
// integers, and this is several times cheaper than math.Pow.
func ipow(base float64, n int) float64 {
	result := 1.0
	for n > 0 {
		if n&1 == 1 {
			result *= base
		}
		base *= base
		n >>= 1
	}
	return result
}

// plateShade lights a flat surface: normal-on to the viewer in the middle,
// rolling off over `bevel` at the edges. edgeDist is the distance to the
// outline, tilt biases the surface upward, lift raises the ambient level.
func plateShade(edgeDist, bevel, tilt, lift float64) (float64, float64) {
	t := clamp(edgeDist/bevel, 0, 1)
	nz := math.Sqrt(t)
	ny := tilt * (1 - t)
	nx := -0.25 * (1 - t)
	nx, ny, nz = normalize3(nx, ny, nz)
	return shadeNormal(nx, ny, nz, 0.18+lift, 0.72, 0.32, 0.30, 18)
}

// shadeAt is the common case: shade a point using the fuselage's normal.
func shadeAt(x, y, ambient, diffuse, rim, specK float64, specP int) (float64, float64) {
	nx, ny, nz := bodyNormal(x, y)
	return shadeNormal(nx, ny, nz, ambient, diffuse, rim, specK, specP)
}

// bodyNormal treats the fuselage as a generalised cylinder along x: the radial
// direction comes from where y sits between the top and bottom profiles, and
// the axial component from how fast the body is tapering.
func bodyNormal(x, y float64) (nx, ny, nz float64) {
	top, bot := bodyTop(x), bodyBot(x)
	yc := (top + bot) / 2
	half := (top - bot) / 2
	if half <= 1e-6 {
		return 0, 0, 1
	}
	ry := clamp((y-yc)/half, -1, 1)
	rz := math.Sqrt(math.Max(0, 1-ry*ry))
	return normalize3(-lutAt(&lutDHalf, x), ry, rz)
}

// ---------------------------------------------------------------------------
// rotors
// ---------------------------------------------------------------------------

// rotorCoverage integrates blade coverage across the shutter interval, which is
// what turns a set of thin lines into a convincing motion-blurred disc.
func rotorCoverage(x, y float64, o *Options) float64 {
	// Cheap reject: everything outside the disc's bounding band.
	const band = mainRotorR*mainRotorTilt + mainRotorConing + 0.05
	if y < mainHubY-band || y > mainHubY+band+0.04 {
		return 0
	}
	if x < mainHubX-mainRotorR-0.04 || x > mainHubX+mainRotorR+0.04 {
		return 0
	}

	var acc float64
	for s := 0; s < o.steps; s++ {
		for b := 0; b < mainBlades; b++ {
			tip := o.blades[s*mainBlades+b]
			t, dist2 := segmentT2(x, y, mainHubX, mainHubY, tip.x, tip.y)
			// Blades are wide at the root and taper to the tip.
			if w := 0.042 - 0.026*t; dist2 <= w*w {
				acc++
				break // one blade is enough for this sub-step
			}
		}
	}
	return clamp(acc/float64(o.steps), 0, 1)
}

// tailCoverage models the tail rotor as what it actually looks like at speed:
// a translucent disc, denser at the rim, with a few blade shadows sweeping
// round it. Cheaper and far more convincing than stacking blade lines.
func tailCoverage(x, y float64, o *Options) float64 {
	dx, dy := x-tailHubX, y-tailHubY
	d2 := dx*dx + dy*dy
	if d2 > tailRotorR*tailRotorR {
		return 0
	}
	d := math.Sqrt(d2)
	edge := 1 - smoothstep(tailRotorR*0.88, tailRotorR, d)

	a := (0.14 + 0.20*smoothstep(tailRotorR*0.2, tailRotorR*0.9, d)) * edge

	spoke := math.Cos(float64(tailBlades) * (math.Atan2(dy, dx) - o.TailPhase))
	a += 0.42 * ipow(clamp(spoke, 0, 1), 5) *
		smoothstep(tailRotorR*0.12, tailRotorR*0.55, d) * edge

	return clamp(a, 0, 1)
}

// ---------------------------------------------------------------------------
// sampling
// ---------------------------------------------------------------------------

// sample evaluates the whole model at one point in model space, front-most
// part first. It returns the material, its lit shade, a specular term and
// coverage (fractional only where a rotor blade is blurring).
func sample(x, y float64, o *Options) (Material, float64, float64, float64) {
	// --- navigation lights ------------------------------------------------
	if hypot2(x+0.985, y-0.005) <= 0.026*0.026 {
		return MatNavRed, 1.35, 0, 1
	}
	if hypot2(x-1.545, y-0.305) <= 0.022*0.022 {
		return MatNavGreen, 1.35, 0, 1
	}
	if o.BeaconOn && hypot2(x-1.500, y-0.470) <= 0.024*0.024 {
		return MatBeacon, 1.5, 0, 1
	}
	if o.StrobeOn && hypot2(x+0.255, y+0.300) <= 0.022*0.022 {
		return MatBeacon, 1.5, 0, 1
	}

	// --- rotor head -------------------------------------------------------
	// Swashplate, then mast, then the hub cap on top.
	if math.Abs(x-mainHubX) <= 0.135 && y >= 0.430 && y <= 0.462 {
		sh, sp := shadeNormal(0, 0.2, 0.98, 0.26, 0.8, 0.3, 0.5, 24)
		return MatHub, sh, sp, 1
	}
	if x >= -0.095 && x <= 0.015 && y >= cowlTop(-0.04)-0.02 && y <= 0.500 {
		sh, sp := shadeNormal(-0.35, 0, 0.94, 0.26, 0.85, 0.35, 0.6, 30)
		return MatHub, sh, sp, 1
	}
	if ex, ey := (x-mainHubX)/0.115, (y-mainHubY)/0.052; ex*ex+ey*ey <= 1 {
		ny := clamp((y-mainHubY)/0.052, -1, 1)
		nz := math.Sqrt(math.Max(0, 1-ny*ny))
		sh, sp := shadeNormal(0, ny, nz, 0.26, 0.85, 0.35, 0.7, 28)
		return MatHub, sh, sp, 1
	}

	// --- exhaust ----------------------------------------------------------
	if roundedRect(x, y, 0.245, 0.132, 0.368, 0.208, 0.033) {
		// Hot in the throat, cooler at the lip.
		d := math.Hypot((x-0.310)/0.058, (y-0.170)/0.036)
		return MatExhaust, 0.55 + 0.95*clamp(1-d, 0, 1), 0, 1
	}

	// --- engine cowl ------------------------------------------------------
	if inCowl(x, y) {
		top := cowlTop(x)
		base := bodyTop(x) - 0.03
		half := (top - base) / 2
		ny := clamp((y-(top+base)/2)/math.Max(half, 1e-6), -1, 1)
		nz := math.Sqrt(math.Max(0, 1-ny*ny))
		sh, sp := shadeNormal(-0.15, ny, nz, 0.24, 0.85, 0.30, 0.35, 18)
		// Intake louvres.
		if x > -0.10 && x < 0.16 && math.Mod(math.Abs(x*46), 2) < 0.85 && y > bodyTop(x)+0.01 {
			sh *= 0.72
		}
		return MatEngine, sh, sp, 1
	}

	// --- skids ------------------------------------------------------------
	const strutR2, skidR2 = 0.021 * 0.021, 0.026 * 0.026
	if distToSegment2(x, y, -0.580, -0.292, -0.664, -0.458) <= strutR2 ||
		distToSegment2(x, y, -0.100, -0.292, -0.016, -0.458) <= strutR2 ||
		distToSegment2(x, y, -0.812, -0.392, -0.720, -0.458) <= skidR2 ||
		distToSegment2(x, y, -0.720, -0.458, 0.240, -0.458) <= skidR2 {
		sh, sp := shadeNormal(0, 0.55, 0.83, 0.22, 0.8, 0.45, 0.6, 26)
		return MatSkid, sh, sp, 1
	}

	// --- empennage --------------------------------------------------------
	// The tail rotor is mounted outboard of the fin, so it occludes it.
	if a := tailCoverage(x, y, o); a > 0 {
		return MatRotor, 1.0, 0, a
	}
	// Tail gearbox fairing and rotor hub.
	if ex, ey := (x-tailHubX)/0.075, (y-tailHubY)/0.055; ex*ex+ey*ey <= 1 {
		ny := clamp((y-tailHubY)/0.055, -1, 1)
		nz := math.Sqrt(math.Max(0, 1-ny*ny))
		sh, sp := shadeNormal(0, ny, nz, 0.26, 0.85, 0.35, 0.7, 26)
		return MatHub, sh, sp, 1
	}

	if stabBox.contains(x, y) && inPolygon(x, y, stabPoly) {
		sh, sp := plateShade(polyEdgeDist(x, y, stabPoly), 0.030, 0.35, 0.30)
		return MatFin, sh, sp, 1
	}
	if finBox.contains(x, y) && inPolygon(x, y, finPoly) {
		// Bevel from the edge inward so the fin reads as a plate with
		// thickness, and brighten toward the tip where the light falls.
		sh, sp := plateShade(polyEdgeDist(x, y, finPoly), 0.055, 0.16,
			0.20+0.30*smoothstep(0.05, 0.50, y))
		return MatFin, sh, sp, 1
	}

	// --- glazing and panel lines -----------------------------------------
	if inBody(x, y) {
		gb := lerpProfile(glassX, glassY, x)

		// Windscreen frame: the lower glass edge, the rear post, and two
		// intermediate pillars.
		inGlass := x <= -0.455 && y >= gb
		if inGlass {
			nearEdge := math.Abs(y-gb) <= 0.022
			pillar := math.Abs(x+0.855) <= 0.014 || math.Abs(x+0.660) <= 0.014
			if nearEdge || pillar {
				sh, sp := shadeAt(x, y, 0.24, 0.85, 0.40, 0.30, 20)
				return MatFrame, sh * 0.85, sp * 0.4, 1
			}
			nx, ny, nz := bodyNormal(x, y)
			sh, sp := shadeNormal(nx, ny, nz, 0.30, 0.55, 0.55, 1.5, 42)
			return MatCanopy, sh, sp, 1
		}
		if x > -0.470 && x <= -0.435 && y >= lerpProfile(glassX, glassY, -0.46) {
			sh, sp := shadeAt(x, y, 0.24, 0.85, 0.40, 0.30, 20)
			return MatFrame, sh * 0.85, sp * 0.4, 1
		}

		// Cabin window.
		if roundedRect(x, y, -0.360, -0.020, -0.120, 0.200, 0.055) {
			if onRoundedRectEdge(x, y, -0.360, -0.020, -0.120, 0.200, 0.055, 0.016) {
				sh, sp := shadeAt(x, y, 0.24, 0.85, 0.40, 0.30, 20)
				return MatFrame, sh * 0.85, sp * 0.4, 1
			}
			nx, ny, nz := bodyNormal(x, y)
			sh, sp := shadeNormal(nx, ny, nz, 0.30, 0.55, 0.55, 1.5, 42)
			return MatCanopy, sh, sp, 1
		}

		// Sliding door outline and the boom join seam.
		if onRoundedRectEdge(x, y, -0.400, -0.280, -0.020, 0.240, 0.060, 0.009) ||
			(math.Abs(x-0.560) <= 0.008 && y > bodyBot(x)+0.012 && y < bodyTop(x)-0.012) {
			sh, sp := shadeAt(x, y, 0.24, 0.85, 0.40, 0.30, 20)
			return MatFrame, sh * 0.8, sp * 0.3, 1
		}

		// Bare airframe. The belly gets its own material so themes can darken
		// the underside independently of the flanks.
		nx, ny, nz := bodyNormal(x, y)
		sh, sp := shadeNormal(nx, ny, nz, 0.24, 0.88, 0.40, 0.30, 20)
		mat := MatHull
		if ny < -0.35 {
			mat = MatBelly
		}
		return mat, sh, sp, 1
	}

	// --- main rotor (behind the airframe) ---------------------------------
	if a := rotorCoverage(x, y, o); a > 0 {
		return MatRotor, 1.0, 0, a
	}

	return MatNone, 0, 0, 0
}

// ---------------------------------------------------------------------------
// rasterisation
// ---------------------------------------------------------------------------

// Rasterize renders the helicopter into a fresh w×h frame.
func Rasterize(w, h int, o Options) *Frame {
	return RasterizeInto(nil, w, h, o)
}

// RasterizeInto renders into dst, reallocating only if its size differs. The
// render loop reuses one frame for the life of the process.
func RasterizeInto(dst *Frame, w, h int, o Options) *Frame {
	f := dst
	if f == nil || f.W != w || f.H != h {
		f = newFrame(w, h)
	} else {
		for i := range f.Px {
			f.Px[i] = Pixel{}
		}
	}
	if w <= 0 || h <= 0 {
		return f
	}
	o.blades = f.blades
	o.precomputeBlades()
	f.blades = o.blades

	ss := o.Super
	if ss < 1 {
		ss = 1
	}
	if ss > 2 {
		ss = 2
	}

	const mw = modelMaxX - modelMinX
	const mh = modelMaxY - modelMinY
	scale := math.Min(float64(w)/mw, float64(h)/mh)
	offX := (float64(w) - mw*scale) / 2
	offY := (float64(h) - mh*scale) / 2

	sinP, cosP := math.Sin(o.Pitch), math.Cos(o.Pitch)

	var accShade, accSpec, accAlpha [MatCount]float64

	for py := 0; py < h; py++ {
		for px := 0; px < w; px++ {
			for i := range accShade {
				accShade[i], accSpec[i], accAlpha[i] = 0, 0, 0
			}
			var any bool

			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					fx := float64(px) + (float64(sx)+0.5)/float64(ss)
					fy := float64(py) + (float64(sy)+0.5)/float64(ss)

					mx := (fx-offX)/scale + modelMinX
					// Frame y grows downward; model y grows up.
					my := modelMaxY - (fy-offY)/scale

					// Pitch about the rotor mast, which is roughly the CG.
					rx, ry := mx-mainHubX, my
					mx = mainHubX + rx*cosP + ry*sinP
					my = -rx*sinP + ry*cosP

					mat, sh, sp, a := sample(mx, my, &o)
					if mat == MatNone || a <= 0 {
						continue
					}
					any = true
					accShade[mat] += sh * a
					accSpec[mat] += sp * a
					accAlpha[mat] += a
				}
			}
			if !any {
				continue
			}

			// The material with the most coverage wins the pixel; total
			// coverage becomes its alpha so silhouette edges blend.
			best := MatNone
			var bestA, total float64
			for m := Material(1); m < MatCount; m++ {
				total += accAlpha[m]
				if accAlpha[m] > bestA {
					bestA, best = accAlpha[m], m
				}
			}
			if best == MatNone {
				continue
			}
			n := float64(ss * ss)
			f.Px[py*w+px] = Pixel{
				Mat:   best,
				Shade: accShade[best] / bestA,
				Spec:  accSpec[best] / bestA,
				Alpha: clamp(total/n, 0, 1),
			}
		}
	}
	return f
}

// AspectRatio is the model's width:height, so callers can size a frame that
// fits the helicopter without letterboxing.
func AspectRatio() float64 {
	return (modelMaxX - modelMinX) / (modelMaxY - modelMinY)
}

// RotorHub returns the main rotor hub position in normalised frame coordinates
// (0..1 across the fitted frame). The renderer uses it to anchor the downwash.
func RotorHub(w, h int) (float64, float64) {
	const mw = modelMaxX - modelMinX
	const mh = modelMaxY - modelMinY
	scale := math.Min(float64(w)/mw, float64(h)/mh)
	offX := (float64(w) - mw*scale) / 2
	offY := (float64(h) - mh*scale) / 2
	x := offX + (mainHubX-modelMinX)*scale
	y := offY + (modelMaxY-mainHubY)*scale
	return x, y
}
