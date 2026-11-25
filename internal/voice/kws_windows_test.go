//go:build windows

package voice

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wavSamples(t *testing.T, file string) []float32 {
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(b, []byte("data"))
	if i < 0 {
		t.Fatal("no data chunk")
	}
	pcm := b[i+8:]
	out := make([]float32, len(pcm)/2)
	for j := range out {
		out[j] = float32(int16(binary.LittleEndian.Uint16(pcm[j*2:]))) / 32768
	}
	// trailing silence so the last word gets decoded
	return append(out, make([]float32, vrate)...)
}

// SNAG_KWS_TEST=dir with the two release archives and a wav folder of hit*.wav and miss*.wav, 16 kHz mono
func TestSpotterHears(t *testing.T) {
	src := os.Getenv("SNAG_KWS_TEST")
	if src == "" {
		t.Skip("SNAG_KWS_TEST not set")
	}
	dir := os.Getenv("SNAG_KWS_DIR")
	var err error
	if dir == "" {
		// not t.TempDir, the loaded dlls can't be deleted until the process exits
		dir, _ = os.MkdirTemp("", "snag-kws-*")
		err = installKWS(dir, filepath.Join(src, filepath.Base(kwsLibURL)), filepath.Join(src, filepath.Base(kwsModelURL)))
	}
	if err != nil {
		t.Fatal(err)
	}
	wavs, _ := filepath.Glob(filepath.Join(cmp.Or(os.Getenv("SNAG_KWS_WAVS"), filepath.Join(src, "wav")), "*.wav"))
	for _, w := range wavs {
		sp, err := NewSpotter(dir, cmp.Or(os.Getenv("SNAG_KWS_PHRASE"), "clip that"), 0.5)
		if err != nil {
			t.Fatal(err)
		}
		heard := ""
		s := wavSamples(t, w)
		for i := 0; i < len(s); i += 1600 {
			if k := sp.Feed(s[i:min(i+1600, len(s))]); k != "" {
				heard = k
			}
		}
		sp.Close()
		name := filepath.Base(w)
		want := name[:3] == "hit"
		t.Logf("%s: %q", name, heard)
		if (heard != "") != want {
			t.Errorf("%s: heard %q", name, heard)
		}
	}
}

func TestSplitMatchesModelKeywords(t *testing.T) {
	src := os.Getenv("SNAG_KWS_TEST")
	if src == "" {
		t.Skip("SNAG_KWS_TEST not set")
	}
	dir := t.TempDir()
	if err := installKWS(dir, filepath.Join(src, filepath.Base(kwsLibURL)), filepath.Join(src, filepath.Base(kwsModelURL))); err != nil {
		t.Fatal(err)
	}
	scores, err := loadPieces(modelOf(dir).Bpe)
	if err != nil {
		t.Fatal(err)
	}
	// from the model's own keywords.txt and test_keywords.txt
	for phrase, want := range map[string]string{
		"HELLO WORLD":     "▁HE LL O ▁WORLD",
		"HI GOOGLE":       "▁HI ▁GO O G LE",
		"HEY SIRI":        "▁HE Y ▁S I RI",
		"ALEXA":           "▁A LE X A",
		"LOVE AND PEACE":  "▁LOVE ▁AND ▁P E A CE",
		"PLAY MUSIC":      "▁PLAY ▁MU S IC",
		"GO HOME":         "▁GO ▁HOME",
		"HAPPY NEW YEAR":  "▁HA PP Y ▁NEW ▁YEAR",
		"MERRY CHRISTMAS": "▁ME R RY ▁ CH R IST MA S",
		"LIGHT UP":        "▁ L IGHT ▁UP",
		"LOVELY CHILD":    "▁LOVE LY ▁CHI L D",
		"FOREVER":         "▁FOR E VER",
	} {
		got, _, err := spmEncode(scores, phrase)
		if err != nil {
			t.Fatal(err)
		}
		if g := strings.Join(got, " "); g != want {
			t.Errorf("%s: got %q, want %q", phrase, g, want)
		}
	}
}
