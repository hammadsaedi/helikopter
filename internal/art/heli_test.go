package art

import (
	"math"
	"testing"
)

func TestProfileTablesAlign(t *testing.T) {
	if len(profX) != len(profT) || len(profX) != len(profB) {
		t.Fatalf("profile tables differ in length: x=%d top=%d bot=%d",
			len(profX), len(profT), len(profB))
	}
	for i := 1; i < len(profX); i++ {
		if profX[i] <= profX[i-1] {
			t.Fatalf("profX must ascend: index %d is %v after %v", i, profX[i], profX[i-1])
		}
	}
	for i := range profX {
		if profT[i] <= profB[i] {
			t.Fatalf("top must exceed bottom at x=%v: %v <= %v", profX[i], profT[i], profB[i])
		}
	}
}

func TestModelFitsItsDeclaredBounds(t *testing.T) {
	// Nothing may be drawn outside the box the rasteriser fits to, or it gets
	// silently clipped at some terminal sizes and not others.
	o := Options{RotorPhase: 0.7, TailPhase: 1.9, Shutter: 0.5, Super: 1}
	o.precomputeBlades()
	const step = 0.004
	for x := modelMinX - 0.12; x <= modelMaxX+0.12; x += step {
		for y := modelMinY - 0.12; y <= modelMaxY+0.12; y += step {
			inBounds := x >= modelMinX && x <= modelMaxX && y >= modelMinY && y <= modelMaxY
			if inBounds {
				continue
			}
			if m, _, _, a := sample(x, y, &o); m != MatNone && a > 0 {
				t.Fatalf("material %d drawn outside the model box at (%.3f, %.3f)", m, x, y)
			}
		}
	}
}

func TestRasterizeProducesAnAirframe(t *testing.T) {
	f := Rasterize(240, 90, Options{RotorPhase: 1.1, TailPhase: 0.3, Shutter: 0.5, Super: 2})

	seen := map[Material]int{}
	for _, p := range f.Px {
		if p.Alpha > 0 {
			seen[p.Mat]++
		}
	}
	// The parts that make it recognisable must all survive rasterisation at a
	// typical terminal size.
	for _, m := range []Material{MatHull, MatCanopy, MatSkid, MatRotor, MatFin, MatHub} {
		if seen[m] == 0 {
			t.Errorf("material %d never rendered", m)
		}
	}
	if seen[MatHull] < 200 {
		t.Errorf("hull is suspiciously small: %d pixels", seen[MatHull])
	}
}

func TestRasterizeIntoReusesTheBuffer(t *testing.T) {
	o := Options{RotorPhase: 0.4, Shutter: 0.5, Super: 1}
	f1 := RasterizeInto(nil, 120, 44, o)
	f2 := RasterizeInto(f1, 120, 44, o)
	if f1 != f2 {
		t.Fatal("a same-size frame should be reused, not reallocated")
	}

	// A reused buffer must be cleared: rotate the rotor and no stale blade
	// pixels may survive where the new frame has none.
	blank := RasterizeInto(nil, 120, 44, Options{Super: 1})
	reused := RasterizeInto(f2, 120, 44, Options{Super: 1})
	for i := range blank.Px {
		if blank.Px[i] != reused.Px[i] {
			t.Fatalf("reused frame differs from a fresh one at pixel %d", i)
		}
	}
}

func TestRotorSweepsFullCircle(t *testing.T) {
	// Sampled over a whole revolution the blades must reach both extremes of
	// the disc, otherwise the rotor is not actually turning.
	var minX, maxX = math.MaxFloat64, -math.MaxFloat64
	for ph := 0.0; ph < 2*math.Pi; ph += 0.05 {
		o := Options{RotorPhase: ph, Shutter: 0}
		o.precomputeBlades()
		for x := mainHubX - mainRotorR; x <= mainHubX+mainRotorR; x += 0.02 {
			if rotorCoverage(x, mainHubY+mainRotorConing, &o) > 0 {
				minX = math.Min(minX, x)
				maxX = math.Max(maxX, x)
			}
		}
	}
	if minX > mainHubX-mainRotorR*0.9 || maxX < mainHubX+mainRotorR*0.9 {
		t.Fatalf("rotor sweep is truncated: reached %.2f..%.2f", minX, maxX)
	}
}

func TestAspectRatioMatchesBounds(t *testing.T) {
	want := (modelMaxX - modelMinX) / (modelMaxY - modelMinY)
	if got := AspectRatio(); math.Abs(got-want) > 1e-9 {
		t.Fatalf("AspectRatio() = %v, want %v", got, want)
	}
}
