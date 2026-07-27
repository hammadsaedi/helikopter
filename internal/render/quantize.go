package render

import (
	"image/color"
	"sort"

	"github.com/hammadsaedi/helikopter/internal/theme"
)

// Median-cut quantisation, for turning frames into the 256-colour palette a
// GIF needs.
//
// The obvious alternatives both fail on this scene: a fixed palette bands the
// sky gradient badly, and a popularity palette spends most of its entries on
// sky and leaves the helicopter with almost none, because the sky is the
// majority of every frame.

type colorBox struct {
	cols []theme.RGB
}

func (b *colorBox) bounds() (lo, hi [3]uint8) {
	lo = [3]uint8{255, 255, 255}
	for _, c := range b.cols {
		v := [3]uint8{c.R, c.G, c.B}
		for i := 0; i < 3; i++ {
			if v[i] < lo[i] {
				lo[i] = v[i]
			}
			if v[i] > hi[i] {
				hi[i] = v[i]
			}
		}
	}
	return lo, hi
}

// widest returns the channel with the greatest spread, and that spread.
func (b *colorBox) widest() (channel int, spread int) {
	lo, hi := b.bounds()
	for i := 0; i < 3; i++ {
		if s := int(hi[i]) - int(lo[i]); s > spread {
			channel, spread = i, s
		}
	}
	return channel, spread
}

func (b *colorBox) average() theme.RGB {
	if len(b.cols) == 0 {
		return theme.RGB{}
	}
	var r, g, bl int
	for _, c := range b.cols {
		r += int(c.R)
		g += int(c.G)
		bl += int(c.B)
	}
	n := len(b.cols)
	return theme.RGB{R: uint8(r / n), G: uint8(g / n), B: uint8(bl / n)}
}

// split divides the box at the median of its widest channel.
func (b *colorBox) split() (*colorBox, *colorBox) {
	ch, _ := b.widest()
	sort.Slice(b.cols, func(i, j int) bool {
		a, c := b.cols[i], b.cols[j]
		switch ch {
		case 0:
			return a.R < c.R
		case 1:
			return a.G < c.G
		default:
			return a.B < c.B
		}
	})
	mid := len(b.cols) / 2
	return &colorBox{cols: b.cols[:mid]}, &colorBox{cols: b.cols[mid:]}
}

// Quantize builds a palette of at most n colours covering the given pixels.
func Quantize(pixels []theme.RGB, n int) color.Palette {
	if n < 2 {
		n = 2
	}
	if n > 256 {
		n = 256
	}

	// Deduplicate first: a frame of 50,000 pixels holds only a few thousand
	// distinct colours, and splitting is cheaper on the smaller set.
	seen := make(map[theme.RGB]struct{}, 4096)
	uniq := make([]theme.RGB, 0, 4096)
	for _, p := range pixels {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		uniq = append(uniq, p)
	}

	boxes := []*colorBox{{cols: uniq}}
	for len(boxes) < n {
		// Split whichever box spans the most colour; splitting the largest by
		// count instead would keep subdividing the sky.
		best, bestSpread := -1, 0
		for i, b := range boxes {
			if len(b.cols) < 2 {
				continue
			}
			if _, s := b.widest(); s > bestSpread {
				best, bestSpread = i, s
			}
		}
		if best < 0 {
			break
		}
		lo, hi := boxes[best].split()
		boxes[best] = lo
		boxes = append(boxes, hi)
	}

	pal := make(color.Palette, 0, len(boxes))
	for _, b := range boxes {
		c := b.average()
		pal = append(pal, color.RGBA{R: c.R, G: c.G, B: c.B, A: 255})
	}
	return pal
}
