package render

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/hammadsaedi/helikopter/internal/theme"
)

func newScene(t *testing.T) *Scene {
	t.Helper()
	th, err := theme.Get("crimson")
	if err != nil {
		t.Fatal(err)
	}
	return &Scene{Theme: th, Seed: 3, Size: 0.78, Super: 1}
}

func TestSceneFillsEveryPixel(t *testing.T) {
	s := newScene(t)
	c := NewCanvas(160, 80)
	s.Render(c, 2.5)

	// A gap would show as the zero value, which is pure black; no theme uses it.
	for i, p := range c.Px {
		if p == (theme.RGB{}) {
			t.Fatalf("pixel %d (%d,%d) was never painted", i, i%c.W, i/c.W)
		}
	}
}

func TestSceneIsDeterministic(t *testing.T) {
	a, b := newScene(t), newScene(t)
	ca, cb := NewCanvas(120, 60), NewCanvas(120, 60)
	a.Render(ca, 4.25)
	b.Render(cb, 4.25)
	for i := range ca.Px {
		if ca.Px[i] != cb.Px[i] {
			t.Fatalf("same seed and time produced different pixels at %d", i)
		}
	}
}

func TestSceneSurvivesTinyAndOddSizes(t *testing.T) {
	for _, dim := range [][2]int{{1, 1}, {8, 4}, {13, 7}, {200, 3}, {3, 200}} {
		s := newScene(t)
		c := NewCanvas(dim[0], dim[1])
		s.Render(c, 1.0) // must not panic
	}
}

func TestCloudRingScrollMatchesAFullRebuild(t *testing.T) {
	// The incremental column scroll is the main efficiency trick; if it ever
	// diverges from recomputing the band outright, clouds would drift wrong.
	inc := newScene(t)
	c := NewCanvas(96, 48)
	for _, tm := range []float64{0, 1, 2, 3, 4, 5, 6, 7, 8} {
		inc.Render(c, tm)
	}

	fresh := newScene(t)
	c2 := NewCanvas(96, 48)
	fresh.Render(c2, 8)

	for i := range c.Px {
		if c.Px[i] != c2.Px[i] {
			t.Fatalf("scrolled clouds diverged from a fresh render at pixel %d", i)
		}
	}
}

func TestResizeRebuildsCaches(t *testing.T) {
	s := newScene(t)
	small := NewCanvas(60, 30)
	s.Render(small, 1)
	big := NewCanvas(140, 70)
	s.Render(big, 1) // must not panic or read the old cache
	if s.w != 140 || s.h != 70 {
		t.Fatalf("scene did not adopt the new size: %dx%d", s.w, s.h)
	}
}

func TestScreenDiffShrinksTheSecondFrame(t *testing.T) {
	sc := NewScreen(100, 30, StyleHalfBlock, theme.ModeTrue)
	s := newScene(t)

	s.Render(sc.Canvas(), 1.0)
	sc.SetStatus([]byte("status"))
	first := len(sc.Flush())

	// Re-flushing an unchanged canvas should cost almost nothing beyond the
	// status line, because every cell matches the previous frame.
	second := len(sc.Flush())

	if second >= first/4 {
		t.Errorf("diffing is not working: first=%d bytes, unchanged reflush=%d bytes",
			first, second)
	}
}

func TestScreenInvalidateForcesFullRepaint(t *testing.T) {
	sc := NewScreen(80, 24, StyleHalfBlock, theme.ModeTrue)
	s := newScene(t)
	s.Render(sc.Canvas(), 1.0)
	first := len(sc.Flush())
	sc.Invalidate()
	again := len(sc.Flush())
	if again < first/2 {
		t.Errorf("Invalidate should repaint everything: %d vs %d bytes", again, first)
	}
}

func TestNoColorModeEmitsNoEscapes(t *testing.T) {
	sc := NewScreen(60, 20, StyleASCII, theme.ModeNone)
	s := newScene(t)
	s.Render(sc.Canvas(), 1.0)
	out := sc.Flush()

	// Cursor positioning (ESC [ row ; col H) and the line clear are fine; SGR
	// colour selection is not.
	for _, bad := range []string{"\x1b[38;", "\x1b[48;", "\x1b[38m", "\x1b[48m"} {
		if bytes.Contains(out, []byte(bad)) {
			t.Errorf("colour escape %q leaked into ModeNone output", bad)
		}
	}
	if colourSGR.Match(out) {
		t.Errorf("ModeNone emitted an SGR colour code: %q", firstMatch(out))
	}
}

// Matches SGR sequences that set a colour: 30-37, 40-47, 90-97, 100-107, 38, 48.
var colourSGR = regexp.MustCompile(`\x1b\[(3[0-8]|4[0-8]|9[0-7]|10[0-7])[;m]`)

func firstMatch(b []byte) string {
	if m := colourSGR.Find(b); m != nil {
		return string(m)
	}
	return ""
}

func TestHalfBlockCollapsesIdenticalHalves(t *testing.T) {
	sc := NewScreen(10, 3, StyleHalfBlock, theme.ModeTrue)
	sc.Canvas().Fill(theme.RGB{R: 10, G: 20, B: 30})
	out := sc.Flush()
	if bytes.ContainsRune(out, '▀') {
		t.Error("a uniform canvas should emit spaces, not half-blocks")
	}
}

func TestScreenResizeKeepsGeometryConsistent(t *testing.T) {
	sc := NewScreen(80, 24, StyleHalfBlock, theme.ModeTrue)
	if got := sc.Canvas().H; got != (24-1)*2 {
		t.Fatalf("half-block canvas height = %d, want %d", got, (24-1)*2)
	}
	sc.Resize(50, 10)
	if sc.Canvas().W != 50 || sc.Canvas().H != (10-1)*2 {
		t.Fatalf("after resize canvas is %dx%d", sc.Canvas().W, sc.Canvas().H)
	}
	if sc.ArtRows() != 9 {
		t.Fatalf("ArtRows() = %d, want 9", sc.ArtRows())
	}
}

func TestASCIIStyleUsesOnePixelPerRow(t *testing.T) {
	sc := NewScreen(40, 12, StyleASCII, theme.ModeTrue)
	if sc.Canvas().H != 11 {
		t.Fatalf("ASCII canvas height = %d, want 11", sc.Canvas().H)
	}
}

func BenchmarkSceneRender(b *testing.B) {
	th, _ := theme.Get("crimson")
	s := &Scene{Theme: th, Seed: 1, Size: 0.78, Super: 2}
	c := NewCanvas(200, 98)
	s.Render(c, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Render(c, float64(i)*0.04)
	}
}

func BenchmarkScreenFlush(b *testing.B) {
	th, _ := theme.Get("crimson")
	sc := NewScreen(200, 50, StyleHalfBlock, theme.ModeTrue)
	s := &Scene{Theme: th, Seed: 1, Size: 0.78, Super: 2}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Render(sc.Canvas(), float64(i)*0.04)
		sc.SetStatus([]byte("bench"))
		sc.Flush()
	}
}
