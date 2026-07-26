package theme

import (
	"strings"
	"testing"

	"github.com/hammadsaedi/helikopter/internal/art"
)

func TestEveryThemeDefinesEveryMaterial(t *testing.T) {
	for _, th := range All() {
		for m := art.Material(1); m < art.MatCount; m++ {
			if th.Mat[m] == (RGB{}) {
				t.Errorf("theme %q leaves material %d black", th.Name, m)
			}
		}
		if th.Desc == "" {
			t.Errorf("theme %q has no description", th.Name)
		}
		if th.SkyTop == th.SkyHorizon {
			t.Errorf("theme %q has a flat sky", th.Name)
		}
	}
}

func TestDefaultThemeExists(t *testing.T) {
	if _, err := Get(DefaultName); err != nil {
		t.Fatalf("default theme %q is missing: %v", DefaultName, err)
	}
	if _, err := Get(""); err != nil {
		t.Fatalf("empty name should fall back to the default: %v", err)
	}
}

func TestUnknownThemeListsTheOptions(t *testing.T) {
	_, err := Get("helicoptre")
	if err == nil {
		t.Fatal("expected an error for an unknown theme")
	}
	for _, n := range Names() {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("error message should suggest %q: %s", n, err)
		}
	}
}

func TestParseMode(t *testing.T) {
	cases := map[string]struct {
		want     Mode
		explicit bool
	}{
		"never":     {ModeNone, true},
		"NO":        {ModeNone, true},
		"16":        {Mode16, true},
		"256":       {Mode256, true},
		"truecolor": {ModeTrue, true},
		"auto":      {ModeTrue, false},
		"":          {ModeTrue, false},
	}
	for in, want := range cases {
		got, explicit := ParseMode(in)
		if got != want.want || explicit != want.explicit {
			t.Errorf("ParseMode(%q) = (%v, %v), want (%v, %v)",
				in, got, explicit, want.want, want.explicit)
		}
	}
}

func TestNoColorSuppressesEscapes(t *testing.T) {
	if got := Fg(RGB{255, 0, 0}, ModeNone); got != "" {
		t.Errorf("ModeNone must emit nothing, got %q", got)
	}
	if got := Fg(RGB{255, 0, 0}, ModeTrue); got != "\x1b[38;2;255;0;0m" {
		t.Errorf("truecolor escape is wrong: %q", got)
	}
	if got := Bg(RGB{0, 0, 0}, Mode256); !strings.HasPrefix(got, "\x1b[48;5;") {
		t.Errorf("256-colour background escape is wrong: %q", got)
	}
}

func TestTo256StaysInRange(t *testing.T) {
	for r := 0; r < 256; r += 5 {
		for g := 0; g < 256; g += 5 {
			for b := 0; b < 256; b += 5 {
				if i := to256(RGB{uint8(r), uint8(g), uint8(b)}); i < 16 {
					t.Fatalf("to256(%d,%d,%d) = %d, below the colour cube", r, g, b, i)
				}
			}
		}
	}
}

func TestMixEndpoints(t *testing.T) {
	a, b := RGB{0, 0, 0}, RGB{255, 255, 255}
	if got := Mix(a, b, 0); got != a {
		t.Errorf("Mix at 0 = %v, want %v", got, a)
	}
	if got := Mix(a, b, 1); got != b {
		t.Errorf("Mix at 1 = %v, want %v", got, b)
	}
	// Out-of-range t must clamp rather than overflow the uint8s.
	if got := Mix(a, b, 4); got != b {
		t.Errorf("Mix past 1 = %v, want %v", got, b)
	}
}

func TestScaleClamps(t *testing.T) {
	if got := Scale(RGB{200, 200, 200}, 10, 1); got != (RGB{255, 255, 255}) {
		t.Errorf("Scale should clamp to white, got %v", got)
	}
	if got := Scale(RGB{200, 200, 200}, -5, 0); got != (RGB{}) {
		t.Errorf("Scale should clamp to black, got %v", got)
	}
}

// Line art picks glyphs by brightness, so AtLuminance has to actually reach
// the brightness it is asked for. Scaling the channels cannot: crimson clips
// at 255 and lands well short, which made a red aircraft render several ramp
// steps darker than a white one.
func TestAtLuminanceHitsItsTarget(t *testing.T) {
	colours := []RGB{
		{198, 32, 38},  // saturated red, the case that broke
		{72, 240, 236}, // cyan
		{236, 236, 236},
		{26, 26, 26},
		{0, 0, 0},
		{255, 255, 255},
	}
	for _, c := range colours {
		for _, target := range []float64{0.05, 0.17, 0.4, 0.75, 0.98, 1} {
			got := Luminance(AtLuminance(c, target))
			if diff := got - target; diff > 0.02 || diff < -0.02 {
				t.Errorf("AtLuminance(%v, %.2f) has luminance %.3f", c, target, got)
			}
		}
	}
}

func TestAtLuminanceKeepsHueDirection(t *testing.T) {
	// Brightening blends toward white, so red must stay the dominant channel.
	got := AtLuminance(RGB{198, 32, 38}, 0.9)
	if got.R <= got.G || got.R <= got.B {
		t.Errorf("brightened red lost its hue: %v", got)
	}
}
