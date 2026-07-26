package render

import "math"

func hashU32(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return x
}

func hash2(x, y int, seed uint32) float64 {
	h := hashU32(uint32(x)*0x9e3779b1 ^ uint32(y)*0x85ebca6b ^ seed)
	return float64(h&0xffffff) / float64(0xffffff)
}

func fade(t float64) float64 { return t * t * (3 - 2*t) }

func noise1(x float64, seed uint32) float64 {
	i := math.Floor(x)
	f := fade(x - i)
	a := hash2(int(i), 0, seed)
	b := hash2(int(i)+1, 0, seed)
	return a + (b-a)*f
}

func noise2(x, y float64, seed uint32) float64 {
	xi, yi := math.Floor(x), math.Floor(y)
	fx, fy := fade(x-xi), fade(y-yi)
	ix, iy := int(xi), int(yi)
	a := hash2(ix, iy, seed)
	b := hash2(ix+1, iy, seed)
	c := hash2(ix, iy+1, seed)
	d := hash2(ix+1, iy+1, seed)
	return (a+(b-a)*fx)*(1-fy) + (c+(d-c)*fx)*fy
}

func fbm1(x float64, seed uint32, oct int) float64 {
	var sum, amp, norm float64 = 0, 1, 0
	for i := 0; i < oct; i++ {
		sum += amp * noise1(x, seed+uint32(i)*7919)
		norm += amp
		x *= 2
		amp *= 0.5
	}
	return sum / norm
}

func fbm2(x, y float64, seed uint32, oct int) float64 {
	var sum, amp, norm float64 = 0, 1, 0
	for i := 0; i < oct; i++ {
		sum += amp * noise2(x, y, seed+uint32(i)*7919)
		norm += amp
		x, y = x*2, y*2
		amp *= 0.5
	}
	return sum / norm
}
