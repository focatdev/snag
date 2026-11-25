package voice

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// sherpa-onnx wants keywords already split into the model's pieces, its own splitter is python only
func KeywordLine(spmPath, phrase string, sensitivity float64) (string, error) {
	scores, err := loadPieces(spmPath)
	if err != nil {
		return "", err
	}
	pieces, words, err := spmEncode(scores, phrase)
	if err != nil {
		return "", err
	}
	sensitivity = min(max(sensitivity, 0), 1)
	return fmt.Sprintf("%s #%.2f @%s", strings.Join(pieces, " "), 0.45-0.4*sensitivity, strings.Join(words, "_")), nil
}

// reads the normal pieces and their scores out of a sentencepiece model file, a protobuf
// ModelProto with repeated pieces = 1 { piece = 1, score = 2, type = 3 }
func loadPieces(path string) (map[string]float32, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	bad := errors.New("the keyword spotter's bpe.model is damaged, download it again")
	scores := map[string]float32{}
	for len(b) > 0 {
		num, typ, n := protowire(b)
		if n <= 0 {
			return nil, bad
		}
		b = b[n:]
		val, n := protoSkip(b, typ)
		if n < 0 {
			return nil, bad
		}
		b = b[n:]
		if num != 1 || typ != 2 {
			continue
		}
		var piece string
		var score float32
		kind := uint64(1)
		for len(val) > 0 {
			fnum, ftyp, n := protowire(val)
			if n <= 0 {
				return nil, bad
			}
			val = val[n:]
			fval, n := protoSkip(val, ftyp)
			if n < 0 {
				return nil, bad
			}
			switch {
			case fnum == 1 && ftyp == 2:
				piece = string(fval)
			case fnum == 2 && ftyp == 5:
				score = math.Float32frombits(binary.LittleEndian.Uint32(fval))
			case fnum == 3 && ftyp == 0:
				kind, _ = binary.Uvarint(fval)
			}
			val = val[n:]
		}
		if kind == 1 && piece != "" {
			scores[piece] = score
		}
	}
	if len(scores) == 0 {
		return nil, bad
	}
	return scores, nil
}

func protowire(b []byte) (num, typ uint64, n int) {
	k, n := binary.Uvarint(b)
	return k >> 3, k & 7, n
}

// returns the field's value bytes and how much of b the value took
func protoSkip(b []byte, typ uint64) ([]byte, int) {
	switch typ {
	case 0:
		_, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, -1
		}
		return b[:n], n
	case 1:
		if len(b) < 8 {
			return nil, -1
		}
		return b[:8], 8
	case 2:
		l, n := binary.Uvarint(b)
		if n <= 0 || uint64(len(b)-n) < l {
			return nil, -1
		}
		return b[n : n+int(l)], n + int(l)
	case 5:
		if len(b) < 4 {
			return nil, -1
		}
		return b[:4], 4
	}
	return nil, -1
}

const wordStart = "▁"

// sentencepiece unigram: the split of each word whose pieces have the best total score
func spmEncode(scores map[string]float32, phrase string) ([]string, []string, error) {
	var words []string
	for _, raw := range strings.Fields(strings.ToUpper(phrase)) {
		var b strings.Builder
		for _, r := range raw {
			if (r >= 'A' && r <= 'Z') || r == '\'' {
				b.WriteRune(r)
			} else if unicode.IsLetter(r) {
				return nil, nil, errors.New("the phrase needs English words")
			}
		}
		if b.Len() > 0 {
			words = append(words, b.String())
		}
	}
	if len(words) == 0 {
		return nil, nil, errors.New("the phrase needs English words")
	}
	maxLen := 0
	for p := range scores {
		maxLen = max(maxLen, len(p))
	}
	var out []string
	for _, w := range words {
		s := wordStart + w
		best := make([]float64, len(s)+1)
		from := make([]int, len(s)+1)
		for i := 1; i <= len(s); i++ {
			best[i] = math.Inf(-1)
		}
		for i := 0; i < len(s); i++ {
			if math.IsInf(best[i], -1) || !utf8.RuneStart(s[i]) {
				continue
			}
			for j := i + 1; j <= min(len(s), i+maxLen); j++ {
				if sc, ok := scores[s[i:j]]; ok && best[i]+float64(sc) > best[j] {
					best[j], from[j] = best[i]+float64(sc), i
				}
			}
		}
		if math.IsInf(best[len(s)], -1) {
			return nil, nil, fmt.Errorf("the model can't spell %q", w)
		}
		var parts []string
		for j := len(s); j > 0; j = from[j] {
			parts = append(parts, s[from[j]:j])
		}
		slices.Reverse(parts)
		out = append(out, parts...)
	}
	return out, words, nil
}
