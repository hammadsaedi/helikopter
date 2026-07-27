package render

import (
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hammadsaedi/helikopter/internal/theme"
)

// Frames are rendered at true terminal resolution and then scaled by whole
// pixels, so the result is exactly what the terminal shows rather than a
// smooth approximation of it.
const (
	gifCols   = 200 // terminal columns
	gifRows   = 46  // animation rows
	gifScale  = 3
	gifFrames = 40
	gifFPS    = 20
)

// TestWriteGIF renders the demo animation for the README. Off by default.
//
//	HELIKOPTER_GIF=./docs go test ./internal/render -run WriteGIF -count=1
func TestWriteGIF(t *testing.T) {
	dir := os.Getenv("HELIKOPTER_GIF")
	if dir == "" {
		t.Skip("set HELIKOPTER_GIF=<dir> to write the demo GIF")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	th, err := theme.Get(themeOr("crimson"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Scene{Theme: th, Seed: 7, Size: 0.78, Super: 2}

	w, h := gifCols, gifRows*2
	canvas := NewCanvas(w, h)

	// One palette for the whole animation: a per-frame palette makes the sky
	// shimmer as its entries are reassigned between frames.
	var sample []theme.RGB
	frames := make([][]theme.RGB, 0, gifFrames)
	for i := 0; i < gifFrames; i++ {
		s.Render(canvas, float64(i)/gifFPS)
		buf := make([]theme.RGB, len(canvas.Px))
		copy(buf, canvas.Px)
		frames = append(frames, buf)
		if i%4 == 0 {
			sample = append(sample, buf...)
		}
	}
	pal := Quantize(sample, 256)

	out := &gif.GIF{LoopCount: 0}
	for _, buf := range frames {
		img := image.NewPaletted(image.Rect(0, 0, w*gifScale, h*gifScale), pal)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				p := buf[y*w+x]
				idx := uint8(pal.Index(color.RGBA{p.R, p.G, p.B, 255}))
				for dy := 0; dy < gifScale; dy++ {
					row := (y*gifScale + dy) * img.Stride
					for dx := 0; dx < gifScale; dx++ {
						img.Pix[row+x*gifScale+dx] = idx
					}
				}
			}
		}
		out.Image = append(out.Image, img)
		out.Delay = append(out.Delay, 100/gifFPS) // hundredths of a second
	}

	f, err := os.Create(filepath.Join(dir, "demo.gif"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, out); err != nil {
		t.Fatal(err)
	}

	fi, _ := f.Stat()
	t.Logf("wrote demo.gif: %dx%d, %d frames, %.1f MB",
		w*gifScale, h*gifScale, len(out.Image), float64(fi.Size())/(1<<20))
}

// TestWriteSocialPreview renders the card GitHub shows when a link is shared.
func TestWriteSocialPreview(t *testing.T) {
	dir := os.Getenv("HELIKOPTER_GIF")
	if dir == "" {
		t.Skip("set HELIKOPTER_GIF=<dir> to write the social preview")
	}

	th, err := theme.Get(themeOr("crimson"))
	if err != nil {
		t.Fatal(err)
	}
	// GitHub renders social previews at 1280x640.
	const sw, sh, scale = 320, 160, 4
	c := NewCanvas(sw, sh)
	// 3.82s puts the rotor mid-sweep. At some instants the blades are almost
	// edge-on and the rotor nearly vanishes, which is a poor look for a still.
	(&Scene{Theme: th, Seed: 7, Size: 0.82, Super: 3}).Render(c, 3.82)

	img := image.NewRGBA(image.Rect(0, 0, sw*scale, sh*scale))
	for y := 0; y < sh*scale; y++ {
		for x := 0; x < sw*scale; x++ {
			p := c.Get(x/scale, y/scale)
			img.Set(x, y, color.RGBA{p.R, p.G, p.B, 255})
		}
	}

	f, err := os.Create(filepath.Join(dir, "social.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	fi, _ := f.Stat()
	t.Logf("wrote social.png: %dx%d, %.0f KB", sw*scale, sh*scale, float64(fi.Size())/1024)
}

func themeOr(def string) string {
	if n := os.Getenv("HELIKOPTER_THEME"); n != "" {
		return n
	}
	return def
}
