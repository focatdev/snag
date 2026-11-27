//go:build windows

package voice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"snag/internal/wasapi"
)

// mirrors c-api.h of the pinned sherpa-onnx release, the layout changes between versions
type kwsFeatureConfig struct {
	SampleRate int32
	FeatureDim int32
}

type kwsModelConfig struct {
	Encoder, Decoder, Joiner *byte
	ParaEncoder, ParaDecoder *byte
	Zipformer2Ctc            *byte
	Tokens                   *byte
	NumThreads               int32
	Provider                 *byte
	Debug                    int32
	ModelType                *byte
	ModelingUnit             *byte
	BpeVocab                 *byte
	TokensBuf                *byte
	TokensBufSize            int32
	NemoCtc                  *byte
	TOneCtc                  *byte
}

type kwsConfig struct {
	Feat              kwsFeatureConfig
	Model             kwsModelConfig
	MaxActivePaths    int32
	NumTrailingBlanks int32
	KeywordsScore     float32
	KeywordsThreshold float32
	KeywordsFile      *byte
	KeywordsBuf       *byte
	KeywordsBufSize   int32
}

type kwsResult struct {
	Keyword *byte
}

type kwsLib struct {
	create, destroy, stream, destroyStream, accept, ready, decode, reset, result, freeResult *syscall.Proc
}

var (
	kwsMu      sync.Mutex
	kwsLoaded  *kwsLib
	kwsLoadDir string
)

const loadWithAlteredSearchPath = 0x8

var procLoadLibraryEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LoadLibraryExW")

func loadLibraryEx(path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, _, err := procLoadLibraryEx.Call(uintptr(unsafe.Pointer(p)), 0, loadWithAlteredSearchPath)
	if h == 0 {
		return 0, err
	}
	return syscall.Handle(h), nil
}

// onnxruntime.dll also ships with windows, load ours from the install folder before anything else does
func loadKWS(dir string) (*kwsLib, error) {
	dir = filepath.Clean(dir)
	kwsMu.Lock()
	defer kwsMu.Unlock()
	if kwsLoaded != nil {
		if strings.EqualFold(kwsLoadDir, dir) {
			return kwsLoaded, nil
		}
		return nil, errors.New("restart snag to use a different voice folder")
	}
	if _, err := loadLibraryEx(filepath.Join(dir, "onnxruntime.dll")); err != nil {
		return nil, fmt.Errorf("can't load onnxruntime.dll: %w", err)
	}
	h, err := loadLibraryEx(filepath.Join(dir, kwsLibDLL))
	if err != nil {
		return nil, fmt.Errorf("can't load %s: %w", kwsLibDLL, err)
	}
	dll := &syscall.DLL{Name: kwsLibDLL, Handle: h}
	l := &kwsLib{}
	for _, p := range []struct {
		dst  **syscall.Proc
		name string
	}{
		{&l.create, "SherpaOnnxCreateKeywordSpotter"},
		{&l.destroy, "SherpaOnnxDestroyKeywordSpotter"},
		{&l.stream, "SherpaOnnxCreateKeywordStream"},
		{&l.destroyStream, "SherpaOnnxDestroyOnlineStream"},
		{&l.accept, "SherpaOnnxOnlineStreamAcceptWaveform"},
		{&l.ready, "SherpaOnnxIsKeywordStreamReady"},
		{&l.decode, "SherpaOnnxDecodeKeywordStream"},
		{&l.reset, "SherpaOnnxResetKeywordStream"},
		{&l.result, "SherpaOnnxGetKeywordResult"},
		{&l.freeResult, "SherpaOnnxDestroyKeywordResult"},
	} {
		proc, err := dll.FindProc(p.name)
		if err != nil {
			return nil, fmt.Errorf("%s is missing %s (download it again): %w", kwsLibDLL, p.name, err)
		}
		*p.dst = proc
	}
	kwsLoaded, kwsLoadDir = l, dir
	return l, nil
}

func cstr(s string) *byte {
	b, _ := syscall.BytePtrFromString(s)
	return b
}

func gostr(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

type Spotter struct {
	lib    *kwsLib
	h, st  uintptr
	closed bool
}

// sensitivity 0..1, higher fires on less certain matches
func NewSpotter(dir, phrase string, sensitivity float64) (*Spotter, error) {
	m, err := Find(dir)
	if err != nil {
		return nil, err
	}
	keywords, err := KeywordLine(m.Bpe, phrase, sensitivity)
	if err != nil {
		return nil, err
	}
	lib, err := loadKWS(dir)
	if err != nil {
		return nil, err
	}
	kw := cstr(keywords)
	cfg := &kwsConfig{
		Feat: kwsFeatureConfig{SampleRate: vrate, FeatureDim: 80},
		Model: kwsModelConfig{
			Encoder: cstr(m.Encoder), Decoder: cstr(m.Decoder), Joiner: cstr(m.Joiner),
			Tokens: cstr(m.Tokens), NumThreads: 1, Provider: cstr("cpu"),
		},
		MaxActivePaths:    4,
		NumTrailingBlanks: 1,
		KeywordsScore:     1.0,
		KeywordsThreshold: 0.25,
		KeywordsBuf:       kw,
		KeywordsBufSize:   int32(len(keywords)),
	}
	h, _, _ := lib.create.Call(uintptr(unsafe.Pointer(cfg)))
	runtime.KeepAlive(cfg)
	if h == 0 {
		return nil, errors.New("keyword spotter failed to start, download it again")
	}
	st, _, _ := lib.stream.Call(h)
	if st == 0 {
		lib.destroy.Call(h)
		return nil, errors.New("keyword spotter failed to start")
	}
	return &Spotter{lib: lib, h: h, st: st}, nil
}

// 16 kHz mono samples in, the keyword out when it was just heard
func (s *Spotter) Feed(samples []float32) string {
	if len(samples) == 0 || s.closed {
		return ""
	}
	s.lib.accept.Call(s.st, vrate, uintptr(unsafe.Pointer(&samples[0])), uintptr(len(samples)))
	runtime.KeepAlive(samples)
	heard := ""
	for {
		r, _, _ := s.lib.ready.Call(s.h, s.st)
		if int32(r) == 0 {
			break
		}
		s.lib.decode.Call(s.h, s.st)
		res, _, _ := s.lib.result.Call(s.h, s.st)
		if res == 0 {
			continue
		}
		if k := gostr((*kwsResult)(unsafe.Add(nil, res)).Keyword); k != "" {
			heard = k
			// a match has to be reset or it fires again
			s.Reset()
		}
		s.lib.freeResult.Call(res)
	}
	return heard
}

func (s *Spotter) Reset() {
	if !s.closed {
		s.lib.reset.Call(s.h, s.st)
	}
}

func (s *Spotter) Close() {
	if s.closed {
		return
	}
	s.closed = true
	s.lib.destroyStream.Call(s.st)
	s.lib.destroy.Call(s.h)
}

// streams the mic into the spotter, no audio leaves the pc and there's no per-utterance decode
func Listen(ctx context.Context, o Options, onHeard func(phrase string)) error {
	sp, err := NewSpotter(o.Dir, o.Phrase, o.Sensitivity)
	if err != nil {
		return err
	}
	defer sp.Close()
	chunks := make(chan []byte, 400)
	var dropped atomic.Int64
	c, err := wasapi.Open(ctx, o.Mic, false, func(b []byte) {
		select {
		case chunks <- b:
		default:
			dropped.Add(1)
		}
	})
	if err != nil {
		return err
	}
	conv, err := newResampler(c.Format.SampleFmt, c.Format.SampleRate, c.Format.Channels)
	if err != nil {
		return err
	}
	c.Start()
	var lastHeard, lastDropLog time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case b := <-chunks:
			if dropped.Load() > 0 && time.Since(lastDropLog) >= 5*time.Second {
				if n := dropped.Swap(0); n > 0 {
					slog.Warn("voice audio dropped", "chunks", n)
					lastDropLog = time.Now()
				}
			}
			pcm := conv.Write(b)
			if k := sp.Feed(pcm); k != "" && time.Since(lastHeard) >= 3*time.Second {
				lastHeard = time.Now()
				onHeard(k)
			}
		}
	}
}
