package render

import (
	"testing"

	"github.com/hammadsaedi/helikopter/internal/theme"
)

func rampScene(t *testing.T, name string) *Scene {
	t.Helper()
	th, err := theme.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	return &Scene{Theme: th, Seed: 3, Size: 0.78, Super: 1, PixelAspect: 2, Minimal: true}
}

// composed renders one frame and returns the glyph grid.
func composed(t *testing.T, sc *Screen, s *Scene) [][]rune {
	t.Helper()
	s.Render(sc.Canvas(), 2.5)
	sc.composeCells()

	out := make([][]rune, sc.ArtRows())
	for r := range out {
		out[r] = make([]rune, sc.Cols)
		for x := 0; x < sc.Cols; x++ {
			out[r][x] = sc.cur[r*sc.Cols+x].glyph
		}
	}
	return out
}

// In ramp mode the sky must come out blank. Rendering a full sky gradient
// through a character ramp turns it into a field of punctuation, which is what
// made --ascii and --color never unreadable.
func TestRampModeLeavesTheSkyBlank(t *testing.T) {
	for _, name := range []string{"mono", "crimson", "night"} {
		sc := NewScreen(100, 26, StyleASCII, theme.ModeNone)
		grid := composed(t, sc, rampScene(t, name))

		// Sample the upper corners, well away from the helicopter.
		blank, total := 0, 0
		for r := 0; r < 6; r++ {
			for _, x := range []int{0, 1, 2, 3, 4, 95, 96, 97, 98, 99} {
				total++
				if grid[r][x] == ' ' {
					blank++
				}
			}
		}
		if float64(blank)/float64(total) < 0.9 {
			t.Errorf("theme %q: only %d/%d sky cells are blank", name, blank, total)
		}
	}
}

// The airframe has to reach the dense end of the ramp whatever the theme's
// paint happens to be: a red hull is a third as luminous as a white one, and
// crimson used to render as faint punctuation.
func TestRampModeAirframeReachesTheTopOfTheRamp(t *testing.T) {
	dense := ramp[len(ramp)-3:] // the three densest glyphs

	for _, name := range theme.Names() {
		sc := NewScreen(100, 26, StyleASCII, theme.ModeNone)
		grid := composed(t, sc, rampScene(t, name))

		found := 0
		for _, row := range grid {
			for _, g := range row {
				for _, d := range dense {
					if g == d {
						found++
					}
				}
			}
		}
		if found < 8 {
			t.Errorf("theme %q: only %d cells reach the dense end of the ramp", name, found)
		}
	}
}

// One cell is about twice as tall as it is wide, so ramp mode must squash the
// model vertically or the helicopter comes out stretched to twice its height.
func TestPixelAspectKeepsTheAirframeInProportion(t *testing.T) {
	th, _ := theme.Get("mono")

	measure := func(aspect float64) (w, h int) {
		s := &Scene{Theme: th, Seed: 3, Size: 0.78, Super: 1, PixelAspect: aspect, Minimal: true}
		c := NewCanvas(160, 120)
		s.Render(c, 2.5)
		return s.frame.W, s.frame.H
	}

	w1, h1 := measure(1)
	w2, h2 := measure(2)

	if w1 != w2 {
		t.Fatalf("width should not depend on pixel aspect: %d vs %d", w1, w2)
	}
	// Twice as tall a pixel means half as many pixel rows for the same shape.
	if got, want := float64(h1)/float64(h2), 2.0; got < want*0.9 || got > want*1.1 {
		t.Errorf("height ratio between aspect 1 and 2 is %.2f, want about %.1f", got, want)
	}
}

// Colour off must not leave the picture as an undifferentiated wall of glyphs,
// which is what half blocks degrade to when nothing can set a colour.
func TestNoColourStillProducesStructure(t *testing.T) {
	sc := NewScreen(100, 26, StyleASCII, theme.ModeNone)
	grid := composed(t, sc, rampScene(t, "mono"))

	seen := map[rune]int{}
	for _, row := range grid {
		for _, g := range row {
			seen[g]++
		}
	}
	if len(seen) < 5 {
		t.Errorf("expected a range of ramp glyphs, got %d distinct: %v", len(seen), seen)
	}
	if seen[' '] == 0 {
		t.Error("nothing is blank; the picture has no negative space")
	}
}
