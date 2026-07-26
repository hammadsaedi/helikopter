package audio

import (
	"fmt"
	"math"
)

// Source names where the audio came from, for the status line and --help.
type Source string

const (
	SourceClips Source = "packaged"
	SourceSynth Source = "synth"
	SourceFile  Source = "file"
)

// Relative levels in the mix. The chant carries it; the rotor sits underneath.
const (
	voiceGain = 1.00
	rotorGain = 0.34
)

// Render builds the loop that gets written to disk and played.
//
// Three sources, in order of precedence: a WAV the user pointed at, the
// generated chiptune, or the recordings packaged with the binary.
func Render(cfg Config) ([]byte, Source, error) {
	switch {
	case cfg.File != "":
		c, err := loadFile(cfg.File)
		if err != nil {
			return nil, SourceFile, fmt.Errorf("--sound %s: %w", cfg.File, err)
		}
		return encode(c.samples, c.rate, cfg.Volume), SourceFile, nil

	case cfg.Synth:
		return renderSynth(cfg), SourceSynth, nil
	}

	b, err := renderClips(cfg)
	if err != nil {
		// Losing the packaged audio should not mean losing sound.
		return renderSynth(cfg), SourceSynth, nil
	}
	return b, SourceClips, nil
}

// renderClips mixes the packaged chant and rotor into a single loop. The loop
// is as long as whichever part is playing, with the other tiled to fill it.
func renderClips(cfg Config) ([]byte, error) {
	var voice, rotor *clip
	var err error

	if cfg.Music {
		if voice, err = loadAsset("voice.wav"); err != nil {
			return nil, err
		}
	}
	if cfg.Rotor {
		if rotor, err = loadAsset("rotor.wav"); err != nil {
			return nil, err
		}
	}
	if voice == nil && rotor == nil {
		return encode(nil, clipRate, cfg.Volume), nil
	}

	// The chant sets the loop length when it is present: cutting it to the
	// rotor's length would clip the phrase mid-word.
	lead := voice
	if lead == nil {
		lead = rotor
	}
	rate := lead.rate
	out := make([]float64, len(lead.samples))

	if voice != nil {
		mixInto(out, voice.resampleTo(rate), voiceGain)
	}
	if rotor != nil {
		mixInto(out, rotor.resampleTo(rate), rotorGain)
	}
	return encode(out, rate, cfg.Volume), nil
}

const clipRate = 22050

// encode applies volume, soft-clips, fades the loop seam and wraps the result
// in a WAV.
func encode(samples []float64, rate int, volume float64) []byte {
	if volume <= 0 {
		volume = 0.7
	}
	out := make([]int16, len(samples))

	// A few milliseconds at each end, so restarting the clip does not click.
	fade := rate / 200
	if fade*2 > len(samples) {
		fade = len(samples) / 2
	}

	n := len(samples)
	for i, v := range samples {
		v *= volume
		v = math.Tanh(v * 1.1)
		if fade > 0 {
			if i < fade {
				v *= float64(i) / float64(fade)
			}
			if i >= n-fade {
				v *= float64(n-1-i) / float64(fade)
			}
		}
		out[i] = int16(v * 30000)
	}
	return wav(out, rate)
}
