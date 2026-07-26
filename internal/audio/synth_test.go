package audio

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestSynthProducesAValidWav(t *testing.T) {
	b := renderSynth(Config{Music: true, Rotor: true, Volume: 0.7})

	if len(b) < 44 {
		t.Fatalf("output is shorter than a WAV header: %d bytes", len(b))
	}
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || string(b[36:40]) != "data" {
		t.Fatal("RIFF/WAVE/data chunk markers are wrong")
	}
	if got := binary.LittleEndian.Uint16(b[20:]); got != 1 {
		t.Errorf("format = %d, want 1 (PCM)", got)
	}
	if got := binary.LittleEndian.Uint16(b[22:]); got != 1 {
		t.Errorf("channels = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint32(b[24:]); got != sampleRate {
		t.Errorf("sample rate = %d, want %d", got, sampleRate)
	}
	if got := binary.LittleEndian.Uint16(b[34:]); got != 16 {
		t.Errorf("bit depth = %d, want 16", got)
	}

	dataLen := binary.LittleEndian.Uint32(b[40:])
	if int(dataLen) != len(b)-44 {
		t.Errorf("data chunk claims %d bytes, file carries %d", dataLen, len(b)-44)
	}
	if riff := binary.LittleEndian.Uint32(b[4:]); int(riff) != len(b)-8 {
		t.Errorf("RIFF size = %d, want %d", riff, len(b)-8)
	}

	wantSamples := int(SynthLoopSeconds() * sampleRate)
	if got := len(b[44:]) / 2; got != wantSamples {
		t.Errorf("got %d samples, want %d for a %.2fs loop", got, wantSamples, SynthLoopSeconds())
	}
}

func TestSynthIsAudible(t *testing.T) {
	b := renderSynth(Config{Music: true, Rotor: true, Volume: 0.7})
	var peak, energy float64
	n := (len(b) - 44) / 2
	for i := 0; i < n; i++ {
		v := float64(int16(binary.LittleEndian.Uint16(b[44+i*2:])))
		peak = math.Max(peak, math.Abs(v))
		energy += v * v
	}
	rms := math.Sqrt(energy / float64(n))

	if peak < 5000 {
		t.Errorf("peak amplitude %v is barely audible", peak)
	}
	if peak > 32767 {
		t.Errorf("peak amplitude %v clips", peak)
	}
	if rms < 500 {
		t.Errorf("RMS %v suggests near-silence", rms)
	}
}

func TestLoopSeamIsFaded(t *testing.T) {
	// The loop restarts by relaunching the player, so both ends must be near
	// zero or every repeat clicks.
	b := renderSynth(Config{Music: true, Rotor: true, Volume: 1})
	n := (len(b) - 44) / 2
	first := int16(binary.LittleEndian.Uint16(b[44:]))
	last := int16(binary.LittleEndian.Uint16(b[44+(n-1)*2:]))
	if first != 0 || last != 0 {
		t.Errorf("loop endpoints are not silent: first=%d last=%d", first, last)
	}
}

func TestSilenceComponentsAreSeparable(t *testing.T) {
	rotorOnly := renderSynth(Config{Music: false, Rotor: true, Volume: 0.7})
	both := renderSynth(Config{Music: true, Rotor: true, Volume: 0.7})
	nothing := renderSynth(Config{Music: false, Rotor: false, Volume: 0.7})

	if len(rotorOnly) != len(both) || len(both) != len(nothing) {
		t.Fatal("every configuration should produce the same loop length")
	}

	energy := func(b []byte) float64 {
		var e float64
		for i := 44; i+1 < len(b); i += 2 {
			v := float64(int16(binary.LittleEndian.Uint16(b[i:])))
			e += v * v
		}
		return e
	}
	if energy(nothing) != 0 {
		t.Error("music off and rotor off should render pure silence")
	}
	if energy(rotorOnly) >= energy(both) {
		t.Error("adding music should add energy")
	}
}

func TestVolumeScales(t *testing.T) {
	quiet := renderSynth(Config{Music: true, Rotor: true, Volume: 0.2})
	loud := renderSynth(Config{Music: true, Rotor: true, Volume: 1.0})

	peak := func(b []byte) float64 {
		var p float64
		for i := 44; i+1 < len(b); i += 2 {
			p = math.Max(p, math.Abs(float64(int16(binary.LittleEndian.Uint16(b[i:])))))
		}
		return p
	}
	if peak(quiet) >= peak(loud) {
		t.Errorf("volume 0.2 peaked at %v, volume 1.0 at %v", peak(quiet), peak(loud))
	}
}
