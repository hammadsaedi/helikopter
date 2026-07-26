package audio

import (
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
)

// The chant and the rotor bed ship with the binary as mono 22.05 kHz WAV.
//
// They are WAV rather than the MP3s they came from on purpose: half the
// playback commands we rely on — aplay, paplay, PowerShell's SoundPlayer —
// only understand WAV, so shipping MP3 would have meant sound on macOS and
// silence nearly everywhere else.
//
//go:embed assets/voice.wav assets/rotor.wav
var assets embed.FS

// clip is decoded PCM: mono float samples in -1..1, plus its rate.
type clip struct {
	rate    int
	samples []float64
}

func (c *clip) seconds() float64 {
	if c.rate == 0 {
		return 0
	}
	return float64(len(c.samples)) / float64(c.rate)
}

// decodeWAV reads a PCM WAV. Only what we ship and what a user is likely to
// hand us is supported: 8/16/24/32-bit integer and 32-bit float, any channel
// count, which is downmixed to mono.
func decodeWAV(b []byte) (*clip, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, errors.New("not a RIFF/WAVE file")
	}

	var (
		format        uint16
		channels      uint16
		rate          uint32
		bits          uint16
		data          []byte
		haveFmt       bool
		haveDataChunk bool
	)

	for pos := 12; pos+8 <= len(b); {
		id := string(b[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		body := pos + 8
		if size < 0 || body+size > len(b) {
			size = len(b) - body // tolerate a truncated final chunk
		}

		switch id {
		case "fmt ":
			if size < 16 {
				return nil, errors.New("short fmt chunk")
			}
			format = binary.LittleEndian.Uint16(b[body:])
			channels = binary.LittleEndian.Uint16(b[body+2:])
			rate = binary.LittleEndian.Uint32(b[body+4:])
			bits = binary.LittleEndian.Uint16(b[body+14:])
			haveFmt = true
		case "data":
			data = b[body : body+size]
			haveDataChunk = true
		}

		pos = body + size
		if size%2 == 1 {
			pos++ // chunks are word aligned
		}
	}

	if !haveFmt || !haveDataChunk {
		return nil, errors.New("missing fmt or data chunk")
	}
	if channels == 0 || rate == 0 {
		return nil, errors.New("nonsense channel count or sample rate")
	}
	// 0xFFFE is WAVE_FORMAT_EXTENSIBLE, whose payload is still plain PCM here.
	if format != 1 && format != 3 && format != 0xFFFE {
		return nil, fmt.Errorf("unsupported WAV format %d (only PCM)", format)
	}

	bytesPer := int(bits) / 8
	if bytesPer == 0 {
		return nil, errors.New("zero bit depth")
	}
	frame := bytesPer * int(channels)
	if frame == 0 {
		return nil, errors.New("zero frame size")
	}

	n := len(data) / frame
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		var sum float64
		for ch := 0; ch < int(channels); ch++ {
			off := i*frame + ch*bytesPer
			sum += sampleAt(data, off, bits, format)
		}
		out[i] = sum / float64(channels)
	}
	return &clip{rate: int(rate), samples: out}, nil
}

func sampleAt(data []byte, off int, bits, format uint16) float64 {
	switch {
	case format == 3 && bits == 32:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off:])))
	case bits == 8:
		return (float64(data[off]) - 128) / 128 // 8-bit WAV is unsigned
	case bits == 16:
		return float64(int16(binary.LittleEndian.Uint16(data[off:]))) / 32768
	case bits == 24:
		v := int32(data[off]) | int32(data[off+1])<<8 | int32(data[off+2])<<16
		if v&0x800000 != 0 {
			v |= ^0xffffff
		}
		return float64(v) / 8388608
	case bits == 32:
		return float64(int32(binary.LittleEndian.Uint32(data[off:]))) / 2147483648
	}
	return 0
}

func loadAsset(name string) (*clip, error) {
	b, err := assets.ReadFile("assets/" + name)
	if err != nil {
		return nil, err
	}
	return decodeWAV(b)
}

// loadFile decodes a WAV from disk, for --sound.
func loadFile(path string) (*clip, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := decodeWAV(b)
	if err != nil {
		if strings.EqualFold(pathExt(path), ".wav") {
			return nil, err
		}
		return nil, fmt.Errorf("%w — only WAV is supported, because the audio "+
			"commands available on Linux and Windows cannot play anything else", err)
	}
	return c, nil
}

func pathExt(p string) string {
	if i := strings.LastIndex(p, "."); i >= 0 {
		return p[i:]
	}
	return ""
}

// resampleTo returns the clip's samples at the target rate, using linear
// interpolation. Good enough for a chant and a rotor.
func (c *clip) resampleTo(rate int) []float64 {
	if c.rate == rate || c.rate == 0 {
		return c.samples
	}
	ratio := float64(c.rate) / float64(rate)
	n := int(float64(len(c.samples)) / ratio)
	out := make([]float64, n)
	for i := range out {
		src := float64(i) * ratio
		j := int(src)
		f := src - float64(j)
		a := c.samples[j]
		b := a
		if j+1 < len(c.samples) {
			b = c.samples[j+1]
		}
		out[i] = a + (b-a)*f
	}
	return out
}

// mixInto tiles src across dst at the given gain, so a short bed fills a longer
// loop without a gap.
func mixInto(dst, src []float64, gain float64) {
	if len(src) == 0 || gain == 0 {
		return
	}
	for i := range dst {
		dst[i] += src[i%len(src)] * gain
	}
}
