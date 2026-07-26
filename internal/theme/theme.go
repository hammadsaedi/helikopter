// Package theme holds the colour palettes. A theme paints the whole scene, not
// just the helicopter: sky gradient, terrain, cloud, downwash and every
// material on the airframe. That's why "red" is a theme and not a flag — the
// hull being crimson only reads well against the right sky.
package theme

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hammadsaedi/helikopter/internal/art"
)

// Theme is a full palette for one look.
type Theme struct {
	Name string
	Desc string

	// Airframe, indexed by art.Material.
	Mat [art.MatCount]RGB

	// Sky, vertically interpolated from Top at the crown to Horizon at the
	// skyline.
	SkyTop     RGB
	SkyHorizon RGB

	// Sun or moon disc, drawn behind the clouds.
	Orb        RGB
	OrbGlow    RGB
	OrbVisible bool
	OrbX, OrbY float64 // fraction of the viewport

	Cloud      RGB
	CloudShade RGB

	// Terrain, back layer to front.
	Ridge  RGB
	Hills  RGB
	Ground RGB

	Star  RGB
	Stars bool
	Grid  RGB // synthwave horizon grid; zero value disables it

	Wash RGB // rotor downwash

	// Status bar.
	UIText RGB
	UIDim  RGB
	UIKey  RGB
}

// mats is a small helper so theme definitions read as a table.
type mats map[art.Material]RGB

func build(base mats) [art.MatCount]RGB {
	var out [art.MatCount]RGB
	for m, c := range base {
		out[m] = c
	}
	return out
}

var themes = map[string]*Theme{}

func register(t *Theme) { themes[t.Name] = t }

// DefaultName is the theme used when none is requested.
const DefaultName = "crimson"

func init() {
	register(&Theme{
		Name: "crimson",
		Desc: "red gunship against a dust-orange dusk",
		Mat: build(mats{
			art.MatHull:     {198, 32, 38},
			art.MatBelly:    {96, 18, 26},
			art.MatCanopy:   {58, 78, 96},
			art.MatFrame:    {42, 44, 50},
			art.MatEngine:   {150, 26, 32},
			art.MatExhaust:  {255, 148, 62},
			art.MatFin:      {170, 28, 34},
			art.MatSkid:     {74, 76, 84},
			art.MatRotor:    {228, 226, 224},
			art.MatHub:      {120, 122, 130},
			art.MatNavRed:   {255, 66, 66},
			art.MatNavGreen: {70, 255, 120},
			art.MatBeacon:   {255, 246, 214},
		}),
		SkyTop: RGB{46, 40, 74}, SkyHorizon: RGB{238, 142, 78},
		Orb: RGB{255, 226, 168}, OrbGlow: RGB{250, 168, 92}, OrbVisible: true, OrbX: 0.78, OrbY: 0.62,
		Cloud: RGB{252, 196, 158}, CloudShade: RGB{150, 96, 104},
		Ridge: RGB{88, 60, 82}, Hills: RGB{60, 40, 58}, Ground: RGB{36, 24, 34},
		Star: RGB{255, 240, 220}, Stars: false,
		Wash:   RGB{242, 178, 132},
		UIText: RGB{240, 226, 216}, UIDim: RGB{150, 116, 112}, UIKey: RGB{255, 110, 90},
	})

	register(&Theme{
		Name: "night",
		Desc: "blacked-out airframe, moonlight and strobes",
		Mat: build(mats{
			art.MatHull:     {58, 64, 76},
			art.MatBelly:    {26, 30, 38},
			art.MatCanopy:   {30, 44, 60},
			art.MatFrame:    {20, 22, 28},
			art.MatEngine:   {48, 54, 64},
			art.MatExhaust:  {255, 122, 48},
			art.MatFin:      {52, 58, 70},
			art.MatSkid:     {44, 48, 56},
			art.MatRotor:    {150, 164, 186},
			art.MatHub:      {70, 76, 88},
			art.MatNavRed:   {255, 48, 48},
			art.MatNavGreen: {48, 255, 108},
			art.MatBeacon:   {255, 255, 236},
		}),
		SkyTop: RGB{6, 8, 22}, SkyHorizon: RGB{24, 32, 62},
		Orb: RGB{236, 240, 255}, OrbGlow: RGB{96, 116, 168}, OrbVisible: true, OrbX: 0.80, OrbY: 0.22,
		Cloud: RGB{40, 48, 74}, CloudShade: RGB{20, 26, 44},
		Ridge: RGB{16, 20, 38}, Hills: RGB{10, 14, 28}, Ground: RGB{5, 7, 16},
		Star: RGB{226, 234, 255}, Stars: true,
		Wash:   RGB{70, 84, 120},
		UIText: RGB{198, 212, 240}, UIDim: RGB{92, 104, 134}, UIKey: RGB{120, 190, 255},
	})

	register(&Theme{
		Name: "sunset",
		Desc: "warm chrome on a violet-to-amber sky",
		Mat: build(mats{
			art.MatHull:     {242, 168, 92},
			art.MatBelly:    {126, 70, 76},
			art.MatCanopy:   {86, 62, 96},
			art.MatFrame:    {70, 46, 60},
			art.MatEngine:   {214, 132, 84},
			art.MatExhaust:  {255, 236, 170},
			art.MatFin:      {236, 150, 96},
			art.MatSkid:     {96, 68, 78},
			art.MatRotor:    {255, 226, 196},
			art.MatHub:      {150, 104, 100},
			art.MatNavRed:   {255, 80, 96},
			art.MatNavGreen: {110, 255, 150},
			art.MatBeacon:   {255, 252, 232},
		}),
		SkyTop: RGB{62, 34, 96}, SkyHorizon: RGB{252, 176, 96},
		Orb: RGB{255, 244, 206}, OrbGlow: RGB{255, 132, 96}, OrbVisible: true, OrbX: 0.72, OrbY: 0.70,
		Cloud: RGB{255, 190, 172}, CloudShade: RGB{158, 92, 122},
		Ridge: RGB{96, 54, 96}, Hills: RGB{64, 34, 68}, Ground: RGB{38, 20, 42},
		Star: RGB{255, 240, 220}, Stars: false,
		Wash:   RGB{255, 198, 156},
		UIText: RGB{255, 234, 214}, UIDim: RGB{166, 122, 138}, UIKey: RGB{255, 160, 120},
	})

	register(&Theme{
		Name: "arctic",
		Desc: "search-and-rescue white over a cold blue day",
		Mat: build(mats{
			art.MatHull:     {236, 240, 246},
			art.MatBelly:    {150, 164, 182},
			art.MatCanopy:   {88, 132, 172},
			art.MatFrame:    {70, 84, 100},
			art.MatEngine:   {214, 222, 232},
			art.MatExhaust:  {255, 170, 96},
			art.MatFin:      {228, 92, 62},
			art.MatSkid:     {96, 108, 124},
			art.MatRotor:    {200, 214, 232},
			art.MatHub:      {126, 140, 158},
			art.MatNavRed:   {255, 70, 70},
			art.MatNavGreen: {60, 240, 120},
			art.MatBeacon:   {255, 255, 255},
		}),
		SkyTop: RGB{58, 122, 190}, SkyHorizon: RGB{186, 220, 244},
		Orb: RGB{255, 255, 246}, OrbGlow: RGB{206, 232, 255}, OrbVisible: true, OrbX: 0.20, OrbY: 0.18,
		Cloud: RGB{255, 255, 255}, CloudShade: RGB{186, 202, 222},
		Ridge: RGB{176, 196, 216}, Hills: RGB{150, 174, 198}, Ground: RGB{224, 234, 244},
		Star: RGB{255, 255, 255}, Stars: false,
		Wash:   RGB{214, 232, 248},
		UIText: RGB{28, 42, 60}, UIDim: RGB{104, 126, 148}, UIKey: RGB{14, 96, 176},
	})

	register(&Theme{
		Name: "jungle",
		Desc: "olive drab, low over the treeline",
		Mat: build(mats{
			art.MatHull:     {96, 108, 66},
			art.MatBelly:    {48, 56, 34},
			art.MatCanopy:   {58, 76, 70},
			art.MatFrame:    {38, 42, 30},
			art.MatEngine:   {84, 94, 58},
			art.MatExhaust:  {255, 156, 70},
			art.MatFin:      {88, 100, 60},
			art.MatSkid:     {58, 62, 48},
			art.MatRotor:    {200, 206, 186},
			art.MatHub:      {92, 96, 78},
			art.MatNavRed:   {255, 64, 56},
			art.MatNavGreen: {80, 255, 120},
			art.MatBeacon:   {255, 250, 224},
		}),
		SkyTop: RGB{126, 158, 168}, SkyHorizon: RGB{212, 220, 196},
		Orb: RGB{255, 250, 220}, OrbGlow: RGB{226, 226, 178}, OrbVisible: true, OrbX: 0.28, OrbY: 0.24,
		Cloud: RGB{236, 240, 226}, CloudShade: RGB{168, 180, 168},
		Ridge: RGB{74, 96, 66}, Hills: RGB{52, 72, 46}, Ground: RGB{32, 46, 28},
		Star: RGB{255, 255, 240}, Stars: false,
		Wash:   RGB{206, 214, 190},
		UIText: RGB{228, 234, 212}, UIDim: RGB{132, 146, 118}, UIKey: RGB{170, 220, 110},
	})

	register(&Theme{
		Name: "vapor",
		Desc: "synthwave: neon grid, pink hull, cyan glass",
		Mat: build(mats{
			art.MatHull:     {244, 76, 158},
			art.MatBelly:    {108, 34, 110},
			art.MatCanopy:   {72, 240, 236},
			art.MatFrame:    {58, 30, 88},
			art.MatEngine:   {186, 60, 190},
			art.MatExhaust:  {255, 240, 120},
			art.MatFin:      {126, 74, 236},
			art.MatSkid:     {96, 60, 150},
			art.MatRotor:    {150, 246, 255},
			art.MatHub:      {132, 92, 190},
			art.MatNavRed:   {255, 60, 120},
			art.MatNavGreen: {90, 255, 200},
			art.MatBeacon:   {255, 255, 255},
		}),
		SkyTop: RGB{22, 8, 48}, SkyHorizon: RGB{148, 34, 132},
		Orb: RGB{255, 214, 96}, OrbGlow: RGB{255, 82, 140}, OrbVisible: true, OrbX: 0.62, OrbY: 0.66,
		Cloud: RGB{120, 44, 130}, CloudShade: RGB{62, 20, 82},
		Ridge: RGB{58, 18, 82}, Hills: RGB{36, 10, 58}, Ground: RGB{14, 4, 30},
		Star: RGB{200, 220, 255}, Stars: true,
		Grid:   RGB{236, 64, 196},
		Wash:   RGB{200, 96, 220},
		UIText: RGB{248, 220, 255}, UIDim: RGB{140, 96, 176}, UIKey: RGB{88, 244, 236},
	})

	register(&Theme{
		Name: "matrix",
		Desc: "phosphor green, everything else black",
		Mat: build(mats{
			art.MatHull:     {58, 226, 96},
			art.MatBelly:    {18, 92, 40},
			art.MatCanopy:   {120, 255, 160},
			art.MatFrame:    {14, 64, 30},
			art.MatEngine:   {44, 178, 76},
			art.MatExhaust:  {190, 255, 200},
			art.MatFin:      {50, 200, 86},
			art.MatSkid:     {22, 110, 46},
			art.MatRotor:    {160, 255, 186},
			art.MatHub:      {30, 140, 58},
			art.MatNavRed:   {160, 255, 190},
			art.MatNavGreen: {220, 255, 230},
			art.MatBeacon:   {236, 255, 240},
		}),
		SkyTop: RGB{0, 6, 2}, SkyHorizon: RGB{0, 22, 8},
		OrbVisible: false,
		Cloud:      RGB{0, 34, 14}, CloudShade: RGB{0, 18, 8},
		Ridge: RGB{0, 44, 18}, Hills: RGB{0, 30, 12}, Ground: RGB{0, 16, 6},
		Star: RGB{80, 255, 130}, Stars: true,
		Wash:   RGB{30, 150, 66},
		UIText: RGB{120, 255, 150}, UIDim: RGB{30, 120, 54}, UIKey: RGB{200, 255, 210},
	})

	register(&Theme{
		Name: "mono",
		Desc: "greyscale, and what you get when colour is off",
		// Spaced out by luminance rather than by hue, because this is the
		// palette the ASCII ramp falls back to: the airframe has to sit above
		// the sky, and the sky above the ground, on brightness alone.
		Mat: build(mats{
			art.MatHull:     {236, 236, 236},
			art.MatBelly:    {132, 132, 132},
			art.MatCanopy:   {170, 170, 170},
			art.MatFrame:    {70, 70, 70},
			art.MatEngine:   {206, 206, 206},
			art.MatExhaust:  {255, 255, 255},
			art.MatFin:      {220, 220, 220},
			art.MatSkid:     {112, 112, 112},
			art.MatRotor:    {248, 248, 248},
			art.MatHub:      {150, 150, 150},
			art.MatNavRed:   {255, 255, 255},
			art.MatNavGreen: {255, 255, 255},
			art.MatBeacon:   {255, 255, 255},
		}),
		SkyTop: RGB{26, 26, 26}, SkyHorizon: RGB{104, 104, 104},
		OrbVisible: true, Orb: RGB{190, 190, 190}, OrbGlow: RGB{120, 120, 120},
		OrbX: 0.78, OrbY: 0.60,
		Cloud: RGB{158, 158, 158}, CloudShade: RGB{96, 96, 96},
		Ridge: RGB{74, 74, 74}, Hills: RGB{54, 54, 54}, Ground: RGB{36, 36, 36},
		Star: RGB{210, 210, 210}, Stars: false,
		Wash:   RGB{130, 130, 130},
		UIText: RGB{224, 224, 224}, UIDim: RGB{120, 120, 120}, UIKey: RGB{255, 255, 255},
	})
}

// Get looks up a theme by name. "random" picks one; the pick is left to the
// caller's seed, so Get returns the sorted list position instead of choosing.
func Get(name string) (*Theme, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		n = DefaultName
	}
	if t, ok := themes[n]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("unknown theme %q (try: %s)", name, strings.Join(Names(), ", "))
}

// Names returns every theme name, sorted.
func Names() []string {
	out := make([]string, 0, len(themes))
	for n := range themes {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// All returns every theme, sorted by name.
func All() []*Theme {
	out := make([]*Theme, 0, len(themes))
	for _, n := range Names() {
		out = append(out, themes[n])
	}
	return out
}

// Sky returns the sky colour at vertical fraction t (0 at the top of the
// viewport, 1 at the horizon).
func (t *Theme) Sky(f float64) RGB {
	return Mix(t.SkyTop, t.SkyHorizon, clamp01(f))
}
