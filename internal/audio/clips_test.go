package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func energyOf(b []byte) float64 {
	var e float64
	for i := 44; i+1 < len(b); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(b[i:])))
		e += v * v
	}
	return e
}

func peakOf(b []byte) float64 {
	var p float64
	for i := 44; i+1 < len(b); i += 2 {
		p = math.Max(p, math.Abs(float64(int16(binary.LittleEndian.Uint16(b[i:])))))
	}
	return p
}

// The packaged clips have to decode, or every install falls back to the synth
// without anyone noticing.
func TestPackagedClipsDecode(t *testing.T) {
	for _, name := range []string{"voice.wav", "rotor.wav"} {
		c, err := loadAsset(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c.rate != clipRate {
			t.Errorf("%s: sample rate %d, want %d", name, c.rate, clipRate)
		}
		if d := c.seconds(); d < 1 || d > 60 {
			t.Errorf("%s: implausible duration %.2fs", name, d)
		}
		var peak float64
		for _, v := range c.samples {
			peak = math.Max(peak, math.Abs(v))
		}
		if peak < 0.05 {
			t.Errorf("%s: decoded to near-silence, peak %.3f", name, peak)
		}
		if peak > 1.01 {
			t.Errorf("%s: samples out of range, peak %.3f", name, peak)
		}
	}
}

func TestDefaultSourceIsThePackagedAudio(t *testing.T) {
	b, src, err := Render(Config{Music: true, Rotor: true, Volume: 0.7})
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceClips {
		t.Fatalf("default source is %q, want %q", src, SourceClips)
	}
	if peakOf(b) < 3000 {
		t.Errorf("packaged mix is too quiet, peak %.0f", peakOf(b))
	}
}

func TestChantSetsTheLoopLength(t *testing.T) {
	// Trimming the loop to the rotor bed would cut the chant mid-phrase.
	voice, err := loadAsset("voice.wav")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := Render(Config{Music: true, Rotor: true, Volume: 0.7})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := (len(b)-44)/2, len(voice.samples); got != want {
		t.Errorf("loop is %d samples, want the chant's %d", got, want)
	}
}

func TestRotorOnlyDropsTheChant(t *testing.T) {
	both, _, _ := Render(Config{Music: true, Rotor: true, Volume: 0.7})
	rotor, _, _ := Render(Config{Music: false, Rotor: true, Volume: 0.7})
	if energyOf(rotor) >= energyOf(both) {
		t.Error("dropping the chant should lower the energy")
	}
	if peakOf(rotor) < 500 {
		t.Error("the rotor bed alone is inaudible")
	}
}

func TestSynthFlagBypassesTheClips(t *testing.T) {
	_, src, err := Render(Config{Music: true, Rotor: true, Volume: 0.7, Synth: true})
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceSynth {
		t.Errorf("source is %q, want %q", src, SourceSynth)
	}
}

func TestSoundFileOverridesEverything(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.wav")

	// A one-second tone, 16-bit mono.
	n := 8000
	pcm := make([]int16, n)
	for i := range pcm {
		pcm[i] = int16(12000 * math.Sin(2*math.Pi*440*float64(i)/8000))
	}
	if err := os.WriteFile(path, wav(pcm, 8000), 0o600); err != nil {
		t.Fatal(err)
	}

	b, src, err := Render(Config{Music: true, Rotor: true, Volume: 0.9, File: path})
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceFile {
		t.Errorf("source is %q, want %q", src, SourceFile)
	}
	if got := (len(b) - 44) / 2; got != n {
		t.Errorf("got %d samples, want the file's %d", got, n)
	}
	if rate := binary.LittleEndian.Uint32(b[24:]); rate != 8000 {
		t.Errorf("sample rate %d, want the file's 8000", rate)
	}
}

func TestSoundFileErrorsAreReported(t *testing.T) {
	if _, _, err := Render(Config{File: filepath.Join(t.TempDir(), "nope.wav")}); err == nil {
		t.Error("a missing --sound file should be an error")
	}

	bad := filepath.Join(t.TempDir(), "notaudio.mp3")
	if err := os.WriteFile(bad, []byte("ID3\x04\x00\x00\x00not a wav"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := Render(Config{File: bad})
	if err == nil {
		t.Fatal("a non-WAV --sound file should be an error")
	}
	// The message has to explain why WAV, or the user just sees "invalid file".
	if !contains(err.Error(), "WAV") {
		t.Errorf("error should mention the WAV requirement: %v", err)
	}
}

func TestClipLoopSeamIsSilent(t *testing.T) {
	b, _, err := Render(Config{Music: true, Rotor: true, Volume: 1})
	if err != nil {
		t.Fatal(err)
	}
	n := (len(b) - 44) / 2
	first := int16(binary.LittleEndian.Uint16(b[44:]))
	last := int16(binary.LittleEndian.Uint16(b[44+(n-1)*2:]))
	if first != 0 || last != 0 {
		t.Errorf("loop endpoints are not silent: first=%d last=%d", first, last)
	}
}

func TestDecodeWAVRejectsRubbish(t *testing.T) {
	for name, in := range map[string][]byte{
		"empty":     {},
		"short":     []byte("RIFF"),
		"not riff":  []byte("XXXXsomethingWAVEfmt "),
		"no chunks": append([]byte("RIFF"), append(make([]byte, 4), []byte("WAVE")...)...),
	} {
		if _, err := decodeWAV(in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDecodeWAVHandlesStereoAndBitDepths(t *testing.T) {
	// Stereo 16-bit: the two channels must be averaged to mono.
	const frames = 100
	data := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		binary.LittleEndian.PutUint16(data[i*4:], uint16(int16(1000)))
		binary.LittleEndian.PutUint16(data[i*4+2:], uint16(int16(3000)))
	}
	b := buildWAV(1, 2, 44100, 16, data)

	c, err := decodeWAV(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.samples) != frames {
		t.Fatalf("got %d mono samples from %d stereo frames", len(c.samples), frames)
	}
	want := (1000.0 + 3000.0) / 2 / 32768
	if math.Abs(c.samples[0]-want) > 1e-6 {
		t.Errorf("stereo downmix = %.6f, want %.6f", c.samples[0], want)
	}
}

// buildWAV assembles a WAV header around raw PCM, for the decoder tests.
func buildWAV(format, channels uint16, rate uint32, bits uint16, data []byte) []byte {
	out := make([]byte, 44+len(data))
	copy(out[0:], "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+len(data)))
	copy(out[8:], "WAVE")
	copy(out[12:], "fmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], format)
	binary.LittleEndian.PutUint16(out[22:], channels)
	binary.LittleEndian.PutUint32(out[24:], rate)
	binary.LittleEndian.PutUint32(out[28:], rate*uint32(channels)*uint32(bits)/8)
	binary.LittleEndian.PutUint16(out[32:], channels*bits/8)
	binary.LittleEndian.PutUint16(out[34:], bits)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(data)))
	copy(out[44:], data)
	return out
}

func contains(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
