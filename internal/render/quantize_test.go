package render

import (
	"image/color"
	"math"
	"testing"

	"github.com/hammadsaedi/helikopter/internal/theme"
)

func dist(a theme.RGB, b color.Color) float64 {
	r, g, bl, _ := b.RGBA()
	dr := float64(a.R) - float64(r>>8)
	dg := float64(a.G) - float64(g>>8)
	db := float64(a.B) - float64(bl>>8)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

func TestQuantizeRespectsTheSizeLimit(t *testing.T) {
	var px []theme.RGB
	for i := 0; i < 5000; i++ {
		px = append(px, theme.RGB{R: uint8(i % 256), G: uint8(i / 7 % 256), B: uint8(i / 13 % 256)})
	}
	for _, n := range []int{2, 16, 64, 256} {
		if got := len(Quantize(px, n)); got > n {
			t.Errorf("Quantize(..., %d) returned %d colours", n, got)
		}
	}
	// Out-of-range requests are clamped rather than producing a broken palette.
	if got := len(Quantize(px, 0)); got < 2 || got > 256 {
		t.Errorf("Quantize(..., 0) returned %d colours", got)
	}
	if got := len(Quantize(px, 9999)); got > 256 {
		t.Errorf("Quantize(..., 9999) returned %d colours", got)
	}
}

func TestQuantizeKeepsSmallPalettesExactly(t *testing.T) {
	// Fewer distinct colours than slots means nothing should be approximated.
	in := []theme.RGB{{R: 198, G: 32, B: 38}, {R: 0, G: 0, B: 0}, {R: 255, G: 255, B: 255}, {R: 58, G: 78, B: 96}}
	var px []theme.RGB
	for i := 0; i < 500; i++ {
		px = append(px, in[i%len(in)])
	}
	pal := Quantize(px, 256)
	for _, c := range in {
		if d := dist(c, pal[pal.Index(color.RGBA{c.R, c.G, c.B, 255})]); d > 0.5 {
			t.Errorf("%v was not reproduced exactly, nearest is %.1f away", c, d)
		}
	}
}

// The point of median cut over a popularity palette: the sky is most of every
// frame, and a popularity palette would spend its entries there and leave the
// helicopter flat. A rare colour must still land close to something.
func TestQuantizeDoesNotAbandonRareColours(t *testing.T) {
	var px []theme.RGB
	// A large, near-uniform "sky".
	for i := 0; i < 20000; i++ {
		px = append(px, theme.RGB{R: uint8(40 + i%12), G: uint8(38 + i%10), B: uint8(70 + i%14)})
	}
	// A handful of saturated "airframe" pixels.
	rare := []theme.RGB{{R: 198, G: 32, B: 38}, {R: 255, G: 148, B: 62}, {R: 228, G: 226, B: 224}}
	for i := 0; i < 60; i++ {
		px = append(px, rare[i%len(rare)])
	}

	pal := Quantize(px, 64)
	for _, c := range rare {
		if d := dist(c, pal[pal.Index(color.RGBA{c.R, c.G, c.B, 255})]); d > 24 {
			t.Errorf("rare colour %v is %.1f from its nearest palette entry", c, d)
		}
	}
}

func TestQuantizeHandlesDegenerateInput(t *testing.T) {
	if pal := Quantize(nil, 16); len(pal) == 0 {
		t.Error("an empty input should still yield a usable palette")
	}
	one := []theme.RGB{{R: 10, G: 20, B: 30}}
	pal := Quantize(one, 16)
	if len(pal) == 0 {
		t.Fatal("a single colour should yield a palette")
	}
	if d := dist(one[0], pal[pal.Index(color.RGBA{10, 20, 30, 255})]); d > 0.5 {
		t.Errorf("single colour not reproduced, %.1f away", d)
	}
}
