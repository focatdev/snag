package voice

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestResampler(t *testing.T) {
	var b []byte
	for i := 0; i < 48000; i++ {
		v := uint16(int16(16000 * math.Sin(2*math.Pi*440*float64(i)/48000)))
		b = binary.LittleEndian.AppendUint16(b, v)
		b = binary.LittleEndian.AppendUint16(b, v)
	}
	r, err := newResampler("s16le", 48000, 2)
	if err != nil {
		t.Fatal(err)
	}

	var out []float32
	for off := 0; off < len(b); off += 1922 {
		out = append(out, r.Write(b[off:min(off+1922, len(b))])...)
	}

	if n := len(out); n < 15980 || n > 16000 {
		t.Fatalf("got %d samples from 1 s at 48 kHz, want about 16000", n)
	}
	var peak float32
	for _, v := range out {
		peak = max(peak, v)
	}
	if want := float32(16000) / 32768; math.Abs(float64(peak-want)) > 0.01 {
		t.Errorf("peak %v, want about %v", peak, want)
	}
}
