package render

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/hammadsaedi/helikopter/internal/theme"
)

// TestPreview writes the scene to PNG files so the art can be inspected as
// pixels rather than as escape codes. Off by default.
//
//	HELIKOPTER_PREVIEW=/tmp/out go test ./internal/render -run Preview
func TestPreview(t *testing.T) {
	dir := os.Getenv("HELIKOPTER_PREVIEW")
	if dir == "" {
		t.Skip("set HELIKOPTER_PREVIEW=<dir> to write preview PNGs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	shots := []struct {
		name    string
		w, h    int
		size    float64
		themeFn string
	}{}
	for _, n := range theme.Names() {
		shots = append(shots, struct {
			name    string
			w, h    int
			size    float64
			themeFn string
		}{n, 320, 180, 0.78, n})
	}
	// Actual terminal geometries: canvas width = columns, height = (rows-1)*2.
	for _, extra := range []struct {
		name    string
		w, h    int
		size    float64
		themeFn string
	}{
		{"closeup", 900, 380, 0.98, "crimson"},
		{"term-80x24", 80, 46, 0.78, "crimson"},
		{"term-120x30", 120, 58, 0.78, "crimson"},
		{"term-200x50", 200, 98, 0.78, "night"},
	} {
		shots = append(shots, extra)
	}

	for _, sh := range shots {
		name := sh.name
		th, err := theme.Get(sh.themeFn)
		if err != nil {
			t.Fatal(err)
		}
		c := NewCanvas(sh.w, sh.h)
		s := &Scene{Theme: th, Seed: 7, Size: sh.size, Super: 3}
		s.Render(c, 3.7)

		img := image.NewRGBA(image.Rect(0, 0, c.W, c.H))
		for y := 0; y < c.H; y++ {
			for x := 0; x < c.W; x++ {
				p := c.Get(x, y)
				img.Set(x, y, color.RGBA{p.R, p.G, p.B, 255})
			}
		}
		f, err := os.Create(dir + "/" + name + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}
