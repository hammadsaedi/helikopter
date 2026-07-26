// Package audio synthesises the soundtrack as a WAV loop and plays it through
// whatever audio command the host already has. Nothing is embedded and nothing
// is downloaded: the samples are generated at startup.
package audio

import (
	"encoding/binary"
	"math"
)

const (
	sampleRate = 44100
	bpm        = 150.0
	beat       = 60.0 / bpm
	bars       = 8
	loopBeats  = bars * 4
)

// Config selects what ends up in the mix.
type Config struct {
	Music  bool
	Rotor  bool
	Volume float64 // 0..1

	// Synth generates the soundtrack instead of using the packaged recordings.
	Synth bool
	// File is a WAV supplied by the user, which replaces everything else.
	File string
}

// A note in the pattern: semitones relative to A3, position and length in beats.
type note struct {
	semi       int
	start, dur float64
}

// The hook: four-on-the-floor bounce under a two-syllable-per-beat lead, which
// is about as close to chanting "he-li-kop-ter" as square waves get.
var lead = buildLead()

func buildLead() []note {
	// Scale degrees over A minor, one bar of motif with a varied answer.
	motif := [][]int{
		{12, 12, 15, 12, 19, 17, 12, 0},
		{12, 12, 15, 12, 17, 15, 12, 0},
		{10, 10, 14, 10, 17, 15, 10, 0},
		{12, 12, 15, 19, 22, 19, 17, 15},
	}
	var out []note
	for b := 0; b < bars; b++ {
		row := motif[b%len(motif)]
		for i, s := range row {
			if s == 0 {
				continue
			}
			out = append(out, note{
				semi:  s,
				start: float64(b)*4 + float64(i)*0.5,
				dur:   0.44,
			})
		}
	}
	return out
}

var bass = buildBass()

func buildBass() []note {
	roots := []int{-12, -12, -17, -15}
	var out []note
	for b := 0; b < bars; b++ {
		r := roots[b%len(roots)]
		for i := 0; i < 8; i++ {
			out = append(out, note{semi: r, start: float64(b)*4 + float64(i)*0.5, dur: 0.30})
		}
	}
	return out
}

func freq(semi int) float64 {
	// A3 = 220 Hz is the reference.
	return 220 * math.Pow(2, float64(semi)/12)
}

// square with a soft edge, so it sits in a terminal without shrieking.
func square(phase, duty float64) float64 {
	p := phase - math.Floor(phase)
	if p < duty {
		return 1
	}
	return -1
}

func env(t, dur, attack, release float64) float64 {
	if t < 0 || t > dur {
		return 0
	}
	a := 1.0
	if t < attack {
		a = t / attack
	}
	if t > dur-release {
		a *= (dur - t) / release
	}
	if a < 0 {
		return 0
	}
	return a
}

// renderSynth produces a complete WAV file: one seamless bar-aligned loop.
func renderSynth(cfg Config) []byte {
	total := int(loopBeats * beat * sampleRate)
	buf := make([]float64, total)

	vol := cfg.Volume
	if vol <= 0 {
		vol = 0.7
	}

	if cfg.Music {
		renderVoice(buf, lead, 0.20, 0.5, func(ph float64) float64 {
			return square(ph, 0.32)*0.6 + math.Sin(2*math.Pi*ph)*0.4
		})
		renderVoice(buf, bass, 0.34, 0.5, func(ph float64) float64 {
			return math.Sin(2*math.Pi*ph)*0.8 + square(ph, 0.5)*0.2
		})
		renderDrums(buf)
	}
	if cfg.Rotor {
		renderRotor(buf)
	}

	// Soft-clip, then fade the loop seam so the restart isn't a click.
	out := make([]int16, total)
	fade := int(math.Round(0.012 * sampleRate))
	for i, v := range buf {
		v *= vol
		v = math.Tanh(v * 1.15)
		if i < fade {
			v *= float64(i) / float64(fade)
		}
		if i >= total-fade {
			// total-1-i, not total-i, so the very last sample lands on zero.
			v *= float64(total-1-i) / float64(fade)
		}
		out[i] = int16(v * 30000)
	}
	return wav(out, sampleRate)
}

func renderVoice(buf []float64, notes []note, gain, detune float64, osc func(float64) float64) {
	for _, n := range notes {
		f := freq(n.semi)
		start := int(n.start * beat * sampleRate)
		length := int(n.dur * beat * sampleRate)
		var ph, ph2 float64
		step := f / sampleRate
		step2 := f * math.Pow(2, detune/1200) / sampleRate
		for i := 0; i < length; i++ {
			j := start + i
			if j >= len(buf) {
				break
			}
			t := float64(i) / sampleRate
			e := env(t, n.dur*beat, 0.004, 0.06)
			buf[j] += (osc(ph) + osc(ph2)) * 0.5 * e * gain
			ph += step
			ph2 += step2
		}
	}
}

func renderDrums(buf []float64) {
	var seed uint32 = 0x5eed
	rnd := func() float64 {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		return float64(int32(seed))/float64(math.MaxInt32)*2 - 1
	}

	for b := 0.0; b < loopBeats; b++ {
		// Kick on every beat.
		start := int(b * beat * sampleRate)
		for i := 0; i < int(0.16*sampleRate); i++ {
			j := start + i
			if j >= len(buf) {
				break
			}
			t := float64(i) / sampleRate
			f := 130 * math.Exp(-t*28)
			buf[j] += math.Sin(2*math.Pi*f*t) * math.Exp(-t*14) * 0.55
		}
		// Hat on the off-beat.
		hs := int((b + 0.5) * beat * sampleRate)
		for i := 0; i < int(0.05*sampleRate); i++ {
			j := hs + i
			if j >= len(buf) {
				break
			}
			t := float64(i) / sampleRate
			buf[j] += rnd() * math.Exp(-t*90) * 0.10
		}
	}
}

// renderRotor is the airframe itself: turbine whine, plus broadband noise
// chopped at the blade-passing frequency.
func renderRotor(buf []float64) {
	var s uint32 = 0x1234abcd
	rnd := func() float64 {
		s ^= s << 13
		s ^= s >> 17
		s ^= s << 5
		return float64(int32(s))/float64(math.MaxInt32)*2 - 1
	}

	const bladePass = 8.6 // 2.15 rev/s × 4 blades, matching the animation
	var lp, lp2 float64
	for i := range buf {
		t := float64(i) / sampleRate

		n := rnd()
		lp += (n - lp) * 0.06   // low-pass for body
		lp2 += (lp - lp2) * 0.3 // second pole

		chop := 0.5 + 0.5*math.Sin(2*math.Pi*bladePass*t)
		chop = math.Pow(chop, 2.2)

		thump := math.Sin(2*math.Pi*bladePass*t) * 0.10
		turbine := math.Sin(2*math.Pi*720*t)*0.012 + math.Sin(2*math.Pi*1080*t)*0.008

		buf[i] += lp2*chop*1.7 + thump + turbine
	}
}

// wav wraps PCM samples in a 16-bit mono RIFF container.
func wav(samples []int16, rate int) []byte {
	const hdr = 44
	data := len(samples) * 2
	out := make([]byte, hdr+data)

	copy(out[0:], "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+data))
	copy(out[8:], "WAVE")
	copy(out[12:], "fmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1) // PCM
	binary.LittleEndian.PutUint16(out[22:], 1) // mono
	binary.LittleEndian.PutUint32(out[24:], uint32(rate))
	binary.LittleEndian.PutUint32(out[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(out[32:], 2)
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(data))

	for i, s := range samples {
		binary.LittleEndian.PutUint16(out[hdr+i*2:], uint16(s))
	}
	return out
}

// SynthLoopSeconds is the length of one generated loop.
func SynthLoopSeconds() float64 { return loopBeats * beat }
