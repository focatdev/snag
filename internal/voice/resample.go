package voice

import (
	"encoding/binary"
	"fmt"
	"math"
)

const vrate = 16000

type sampleFormat int

const (
	fmtS16LE sampleFormat = iota + 1
	fmtS24LE
	fmtS32LE
	fmtF32LE
)

func parseSampleFormat(s string) (sampleFormat, int, error) {
	switch s {
	case "s16le":
		return fmtS16LE, 2, nil
	case "s24le":
		return fmtS24LE, 3, nil
	case "s32le":
		return fmtS32LE, 4, nil
	case "f32le":
		return fmtF32LE, 4, nil
	default:
		return 0, 0, fmt.Errorf("unsupported audio format %q", s)
	}
}

// turns wasapi frames into mono 16 kHz float samples for the spotter
type resampler struct {
	fmt  sampleFormat
	bps  int
	ch   int
	step float64
	fir  []float64
	raw  []byte
	in   []float64
	filt []float64
	out  []float32
	pos  float64
}

func newResampler(sampleFmt string, rate, channels int) (*resampler, error) {
	sf, bps, err := parseSampleFormat(sampleFmt)
	if err != nil {
		return nil, err
	}
	if rate <= 0 {
		rate = vrate
	}
	if channels <= 0 {
		channels = 1
	}
	step := float64(rate) / vrate
	r := &resampler{fmt: sf, bps: bps, ch: channels, step: step}
	if step > 1 {
		// low-pass before decimating so energy above 8 kHz doesn't fold into speech
		r.fir = lowpassFIR(31, 0.45/step)
	}
	return r, nil
}

func lowpassFIR(taps int, fc float64) []float64 {
	h := make([]float64, taps)
	mid := taps / 2
	var sum float64
	for i := range h {
		n := float64(i - mid)
		var v float64
		if i == mid {
			v = 2 * fc
		} else {
			v = math.Sin(2*math.Pi*fc*n) / (math.Pi * n)
		}
		// hamming window
		w := 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(taps-1))
		h[i] = v * w
		sum += h[i]
	}
	for i := range h {
		h[i] /= sum
	}
	return h
}

// Write returns resampled 16 kHz mono samples; the returned slice is valid until the next Write.
func (r *resampler) Write(b []byte) []float32 {
	r.raw = append(r.raw, b...)
	fs := r.bps * r.ch
	n := len(r.raw) / fs * fs
	for i := 0; i < n; i += fs {
		var sum float64
		for c := 0; c < r.ch; c++ {
			sum += r.sample(r.raw[i+c*r.bps:])
		}
		s := sum / float64(r.ch)
		if len(r.fir) > 0 {
			r.in = append(r.in, s)
		} else {
			r.filt = append(r.filt, s)
		}
	}
	r.raw = append(r.raw[:0], r.raw[n:]...)

	if taps := len(r.fir); taps > 0 && len(r.in) >= taps {
		avail := len(r.in) - taps + 1
		for i := 0; i < avail; i++ {
			var acc float64
			for j, w := range r.fir {
				acc += r.in[i+j] * w
			}
			r.filt = append(r.filt, acc)
		}
		r.in = append(r.in[:0], r.in[avail:]...)
	}

	r.out = r.out[:0]
	for r.pos+1 < float64(len(r.filt)) {
		i := int(r.pos)
		f := r.pos - float64(i)
		r.out = append(r.out, float32(max(-1, min(1, r.filt[i]*(1-f)+r.filt[i+1]*f))))
		r.pos += r.step
	}
	k := min(int(r.pos), len(r.filt))
	r.filt = append(r.filt[:0], r.filt[k:]...)
	r.pos -= float64(k)
	return r.out
}

func (r *resampler) sample(b []byte) float64 {
	switch r.fmt {
	case fmtF32LE:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
	case fmtS24LE:
		u := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
		return float64(int32(u<<8)>>8) / 8388608
	case fmtS32LE:
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
	default:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
	}
}
