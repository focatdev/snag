package recorder

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"snag/internal/config"
	"snag/internal/ffmpeg"
	"snag/internal/gamedetect"
	"snag/internal/tsbuf"
	"snag/internal/wasapi"
	"snag/internal/winapi"
)

type State string

const (
	Idle      State = "idle"
	Starting  State = "starting"
	Buffering State = "buffering"
	Failed    State = "error"
)

type Status struct {
	State        State    `json:"state"`
	Error        string   `json:"error"`
	Warning      string   `json:"warning"`
	Encoder      string   `json:"encoder"`
	EncoderLabel string   `json:"encoderLabel"`
	Pipeline     string   `json:"pipeline"`
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	FPS          int      `json:"fps"`
	Kbps         int      `json:"kbps"`
	BufferedSec  float64  `json:"bufferedSec"`
	BufferTarget int      `json:"bufferTarget"`
	BufferBytes  int      `json:"bufferBytes"`
	Tracks       []string `json:"tracks"`
	StartedAt    int64    `json:"startedAt"`
	Game         string   `json:"game"`
	SmartWaiting bool     `json:"smartWaiting"`
	Paused       bool     `json:"paused"`
	CPU          float64  `json:"cpu"`
	MemBytes     uint64   `json:"memBytes"`
	Saving       bool     `json:"saving"`
	CanSave      bool     `json:"canSave"`
}

type Deps struct {
	DataDir  string
	Settings func() config.Settings
	Tools    func() ffmpeg.Tools
	Encoders func() []ffmpeg.Encoder
	Features func() ffmpeg.Features
	OnChange func()
}

type Recorder struct {
	d    Deps
	ring *tsbuf.Ring

	mu        sync.Mutex
	state     State
	lastErr   string
	warning   string
	cancel    context.CancelFunc
	spec      captureSpec
	tracks    []string
	startedAt time.Time
	ffPID     int
	want      bool
	paused    bool
	bySmart   bool
	waiting   bool
	game      string
	saving    atomic.Bool
	micActive atomic.Int64
	session   int
	last      lastSave
	attemptN  int

	statMu   sync.Mutex
	lastCPU  uint64
	lastTick time.Time
	cpu      float64
	mem      uint64

	saveMu sync.Mutex
}

func New(d Deps) *Recorder {
	return &Recorder{d: d, ring: tsbuf.NewRing(time.Duration(d.Settings().BufferSeconds) * time.Second), state: Idle}
}

func (r *Recorder) notify() {
	if r.d.OnChange != nil {
		r.d.OnChange()
	}
}

func (r *Recorder) Status() Status {
	secs, bytes := r.ring.Buffered()
	r.mu.Lock()
	defer r.mu.Unlock()
	st := Status{
		State: r.state, Error: r.lastErr, Warning: r.warning,
		Encoder: r.spec.Encoder, Pipeline: string(r.spec.Pipeline),
		Width: r.spec.OutW, Height: r.spec.OutH, FPS: r.spec.FPS, Kbps: r.spec.Kbps,
		BufferedSec: secs, BufferBytes: bytes, BufferTarget: r.d.Settings().BufferSeconds,
		Tracks: r.tracks, Game: r.game, SmartWaiting: r.waiting, Paused: r.paused,
		Saving: r.saving.Load(), CanSave: bytes > 0,
	}
	for _, e := range ffmpeg.Candidates {
		if e.Name == st.Encoder {
			st.EncoderLabel = e.Label
		}
	}
	if !r.startedAt.IsZero() && r.state == Buffering {
		st.StartedAt = r.startedAt.UnixMilli()
	}
	r.statMu.Lock()
	st.CPU, st.MemBytes = r.cpu, r.mem
	r.statMu.Unlock()
	return st
}

func (r *Recorder) SampleUsage() {
	pids := []int{os.Getpid()}
	r.mu.Lock()
	if r.ffPID != 0 {
		pids = append(pids, r.ffPID)
	}
	r.mu.Unlock()
	var cpu, mem uint64
	for _, p := range pids {
		c, m, err := winapi.ProcessUsage(p)
		if err == nil {
			cpu += c
			mem += m
		}
	}
	r.statMu.Lock()
	defer r.statMu.Unlock()
	now := time.Now()
	if !r.lastTick.IsZero() && cpu >= r.lastCPU {
		el := now.Sub(r.lastTick).Seconds() * 1e7 * float64(runtime.NumCPU())
		if el > 0 {
			r.cpu = float64(cpu-r.lastCPU) / el * 100
		}
	}
	r.lastCPU, r.lastTick, r.mem = cpu, now, mem
}

func (r *Recorder) SetBufferSeconds(n int) {
	r.ring.SetMaxDuration(time.Duration(n) * time.Second)
}

func (r *Recorder) Start() {
	r.mu.Lock()
	r.paused = false
	r.bySmart = false
	r.mu.Unlock()
	r.start()
}

func (r *Recorder) start() {
	r.mu.Lock()
	if r.state == Starting || r.state == Buffering {
		r.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.state, r.lastErr, r.warning, r.want = Starting, "", "", true
	r.mu.Unlock()
	r.notify()
	go r.loop(ctx)
}

// keep the buffer so a late save still works
func (r *Recorder) Stop() {
	r.mu.Lock()
	r.paused = true
	r.mu.Unlock()
	r.stop()
}

func (r *Recorder) stop() {
	r.mu.Lock()
	r.want = false
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.mu.Unlock()
}

func (r *Recorder) Restart() {
	r.mu.Lock()
	running := r.state == Starting || r.state == Buffering
	r.mu.Unlock()
	if !running {
		return
	}
	r.stop()
	for i := 0; i < 50; i++ {
		r.mu.Lock()
		s := r.state
		r.mu.Unlock()
		if s != Starting && s != Buffering {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	r.start()
}

func (r *Recorder) Toggle() {
	r.mu.Lock()
	running := r.state == Starting || r.state == Buffering
	r.mu.Unlock()
	if running {
		r.Stop()
	} else {
		r.Start()
	}
}

func (r *Recorder) fail(msg string) {
	r.mu.Lock()
	r.state, r.lastErr, r.ffPID = Failed, msg, 0
	r.mu.Unlock()
	r.notify()
}

func (r *Recorder) loop(ctx context.Context) {
	restarts := 0
	for {
		started, err := r.run(ctx)
		if ctx.Err() != nil {
			r.mu.Lock()
			r.state, r.ffPID = Idle, 0
			r.mu.Unlock()
			r.notify()
			return
		}
		if started {
			restarts = 0
		}
		if err != nil {
			slog.Warn("capture stopped", "err", err)
		}
		restarts++
		if restarts > 3 {
			r.fail(err.Error())
			return
		}
		r.mu.Lock()
		r.state, r.warning = Starting, "Capture stopped, restarting it"
		r.mu.Unlock()
		r.notify()
		select {
		case <-ctx.Done():
			continue
		case <-time.After(2 * time.Second):
		}
	}
}

// gpu capture support varies by driver
func (r *Recorder) run(ctx context.Context) (bool, error) {
	s := r.d.Settings()
	tools := r.d.Tools()
	if tools.FFmpeg == "" {
		return false, errors.New("FFmpeg is not installed. Open Settings to download it")
	}
	enc := chooseEncoder(s.Encoder, s.Codec, r.d.Encoders())

	m := winapi.MonitorAt(s.Monitor)
	srcW, srcH, left, top := m.Width, m.Height, m.Left, m.Top
	outW, outH := s.OutputSize(srcW, srcH)
	spec := captureSpec{
		Encoder: enc, Monitor: s.Monitor, MonLeft: left, MonTop: top,
		SrcW: srcW, SrcH: srcH, OutW: outW, OutH: outH,
		FPS: s.FPS, Kbps: s.BitrateKbps(outW, outH), Cursor: s.DrawCursor,
	}
	r.ring.SetMaxDuration(time.Duration(s.BufferSeconds) * time.Second)
	if s.BufferOnDisk {
		dir := filepath.Join(s.ClipsDir, ".snag-buffer")
		r.ring.SetDisk(dir)
		hideFile(dir)
	} else {
		r.ring.SetDisk("")
	}

	var lastErr error
	for _, p := range pipelines(r.d.Features(), enc, outW != srcW || outH != srcH) {
		spec.Pipeline = p
		started, err := r.attempt(ctx, spec, s)
		if started || ctx.Err() != nil {
			return started, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no capture method worked")
	}
	return false, lastErr
}

type source struct {
	title string
	cap   *wasapi.Capture
	pipe  *winapi.Pipe
	ch    chan []byte
}

func (r *Recorder) attempt(parent context.Context, spec captureSpec, s config.Settings) (bool, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	r.mu.Lock()
	r.attemptN++
	n := r.attemptN
	r.mu.Unlock()

	var sources []*source
	var warnings []string
	add := func(title string, opener func(out func([]byte)) (*wasapi.Capture, error)) error {
		src := &source{title: title, ch: make(chan []byte, 400)}
		c, err := opener(func(b []byte) {
			if title == "Mic" && loud(b, src) {
				r.micActive.Store(time.Now().UnixNano())
			}
			select {
			case src.ch <- b:
			default:
			}
		})
		if err != nil {
			return err
		}
		p, err := winapi.NewPipe(fmt.Sprintf("snag-%d-%d-%s", os.Getpid(), n, strings.ToLower(title)))
		if err != nil {
			return fmt.Errorf("%s audio pipe failed: %w", title, err)
		}
		src.cap, src.pipe = c, p
		sources = append(sources, src)
		return nil
	}
	device := func(title, dev string, loopback bool) {
		err := add(title, func(out func([]byte)) (*wasapi.Capture, error) { return wasapi.Open(ctx, dev, loopback, out) })
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s audio unavailable: %v", title, err))
		}
	}
	if s.CaptureSystemAudio {
		split := false
		if s.SeparateVoice {
			if pid := winapi.RootProcess("Discord.exe", "DiscordCanary.exe", "DiscordPTB.exe"); pid > 0 {
				errGame := add("Game", func(out func([]byte)) (*wasapi.Capture, error) { return wasapi.OpenProcess(ctx, pid, false, out) })
				if errGame == nil {
					split = true
					if err := add("Voice", func(out func([]byte)) (*wasapi.Capture, error) { return wasapi.OpenProcess(ctx, pid, true, out) }); err != nil {
						warnings = append(warnings, "Discord track unavailable")
					}
				} else {
					warnings = append(warnings, errGame.Error())
				}
			}
		}
		if !split {
			device("Game", s.SystemDeviceID, true)
		}
	}
	if s.CaptureMic {
		device("Mic", s.MicDeviceID, false)
	}
	defer func() {
		for _, src := range sources {
			src.pipe.Close()
		}
	}()

	spec.Audio = nil
	tracks := []string{}
	for _, src := range sources {
		f := src.cap.Format
		spec.Audio = append(spec.Audio, audioInput{Title: src.title, PipeName: src.pipe.Name, SampleFmt: f.SampleFmt, SampleRate: f.SampleRate, Channels: f.Channels, Mono: src.title == "Mic"})
		tracks = append(tracks, src.title)
	}

	cmd := ffmpeg.Command(ctx, r.d.Tools().FFmpeg, false, buildArgs(spec)...)
	stderr := ffmpeg.NewTail(4096)
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	if err := cmd.Start(); err != nil {
		return false, err
	}

	for _, src := range sources {
		go func(src *source) {
			if src.pipe.Accept() != nil {
				return
			}
			src.cap.Start()
			for {
				select {
				case <-ctx.Done():
					return
				case b := <-src.ch:
					if _, err := src.pipe.Write(b); err != nil {
						return
					}
				}
			}
		}(src)
	}

	r.ring.Reset()
	r.mu.Lock()
	r.session++
	r.mu.Unlock()
	fed := make(chan error, 1)
	go func() { fed <- r.ring.Feed(stdout) }()
	exited := make(chan error, 1)
	go func() {
		<-fed
		exited <- cmd.Wait()
	}()

	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
wait:
	for {
		select {
		case <-exited:
			return false, fmt.Errorf("%s capture failed: %s", spec.Pipeline, ffmpeg.Tail(stderr.String(), 400))
		case <-deadline.C:
			cancel()
			<-exited
			return false, fmt.Errorf("%s capture produced no video: %s", spec.Pipeline, ffmpeg.Tail(stderr.String(), 400))
		case <-tick.C:
			if _, b := r.ring.Buffered(); b > 0 {
				break wait
			}
		case <-ctx.Done():
			<-exited
			return false, ctx.Err()
		}
	}

	r.mu.Lock()
	r.state, r.spec, r.tracks, r.startedAt = Buffering, spec, tracks, time.Now()
	r.warning = strings.Join(warnings, ". ")
	if cmd.Process != nil {
		r.ffPID = cmd.Process.Pid
	}
	r.mu.Unlock()
	r.notify()

	err = <-exited
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	return true, fmt.Errorf("capture exited: %s", ffmpeg.Tail(stderr.String(), 400))
}

func (r *Recorder) AddMark() bool { return r.ring.AddMark() }

type SaveResult struct {
	Path     string
	Game     string
	Marks    []float64
	Tracks   []string
	Encoder  string
	Replaced string
}

type lastSave struct {
	path     string
	startPTS int64
	at       time.Time
	session  int
}

func (r *Recorder) Save(seconds int) (SaveResult, error) {
	if !r.saving.CompareAndSwap(false, true) {
		return SaveResult{}, errors.New("a clip is already being saved")
	}
	defer r.saving.Store(false)
	r.notify()
	defer r.notify()
	r.saveMu.Lock()
	defer r.saveMu.Unlock()

	tools := r.d.Tools()
	game := gamedetect.Recent(2 * time.Minute)
	name := game.Name
	if name == "" {
		name = "Desktop"
	}
	r.mu.Lock()
	r.game = name
	tracks := append([]string(nil), r.tracks...)
	codec := r.spec.Encoder
	r.mu.Unlock()

	now := time.Now()
	length := seconds
	if held, _ := r.ring.Buffered(); held > 0 && int(held+0.5) < length {
		length = int(held + 0.5)
	}
	cfg := r.d.Settings()
	since := int64(-1)
	r.mu.Lock()
	prev := r.last
	session := r.session
	r.mu.Unlock()
	// second press inside the window extends the previous clip
	merging := false
	if cfg.MergeOverlaps && prev.path != "" && prev.session == session && time.Since(prev.at) < time.Duration(cfg.BufferSeconds)*time.Second && r.ring.Contains(prev.startPTS) {
		if _, err := os.Stat(prev.path); err == nil {
			since, merging = prev.startPTS, true
		}
	}
	base := cfg.ClipPath(name, length, now)
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		return SaveResult{}, err
	}
	// stage next to the output, not in %TEMP% on C:
	tmp := filepath.Join(filepath.Dir(base), ".snag-"+now.Format("20060102-150405.000")+".ts")
	f, err := os.Create(tmp)
	if err != nil {
		return SaveResult{}, err
	}
	info, err := r.ring.Snapshot(f, float64(seconds), since)
	f.Close()
	defer os.Remove(tmp)
	if err != nil {
		return SaveResult{}, err
	}

	if merging {
		base = cfg.ClipPath(name, int(info.Seconds+0.5), now)
	}
	out := uniquePath(base + ".mp4")
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", tmp, "-map", "0:v"}
	silentMic := cfg.DropSilentMic && time.Since(time.Unix(0, r.micActive.Load())) > time.Duration(info.Seconds+1)*time.Second
	kept := []string{}
	for i, t := range tracks {
		if t == "Mic" && silentMic {
			continue
		}
		args = append(args, "-map", fmt.Sprintf("0:a:%d", i))
		kept = append(kept, t)
	}
	args = append(args, "-c", "copy", "-avoid_negative_ts", "make_zero", "-movflags", "+faststart")
	if strings.HasPrefix(codec, "hevc") || codec == "libx265" {
		args = append(args, "-tag:v", "hvc1")
	}
	for i, t := range kept {
		args = append(args, fmt.Sprintf("-metadata:s:a:%d", i), "title="+t)
	}
	args = append(args, "-metadata", "title="+name, out)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := ffmpeg.Run(ctx, tools.FFmpeg, args...); err != nil {
		os.Remove(out)
		return SaveResult{}, err
	}
	res := SaveResult{Path: out, Game: name, Marks: info.Marks, Tracks: kept, Encoder: codec}
	if merging {
		os.Remove(prev.path)
		res.Replaced = prev.path
	}
	r.mu.Lock()
	r.last = lastSave{path: out, startPTS: info.StartPTS, at: now, session: session}
	r.mu.Unlock()
	return res, nil
}

// grace period for alt-tabbing to discord
func (r *Recorder) SmartLoop(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	var lastGame time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s := r.d.Settings()
		info := gamedetect.Current()
		r.mu.Lock()
		if info.IsGame {
			r.game = info.Name
		}
		state, paused, bySmart := r.state, r.paused, r.bySmart
		if !s.SmartMode {
			changed := r.waiting
			r.waiting = false
			r.mu.Unlock()
			if changed {
				r.notify()
			}
			continue
		}
		r.mu.Unlock()

		if info.IsGame {
			lastGame = time.Now()
		}
		running := state == Starting || state == Buffering
		switch {
		case info.IsGame && !running && !paused && r.d.Tools().FFmpeg != "":
			r.mu.Lock()
			r.bySmart, r.waiting = true, false
			r.mu.Unlock()
			r.start()
		case !info.IsGame && running && bySmart && time.Since(lastGame) > 45*time.Second:
			r.stop()
			r.mu.Lock()
			r.waiting = true
			r.mu.Unlock()
			r.notify()
		case !running && !info.IsGame:
			r.mu.Lock()
			changed := !r.waiting
			r.waiting = true
			r.mu.Unlock()
			if changed {
				r.notify()
			}
		}
	}
}

func uniquePath(p string) string {
	if _, err := os.Stat(p); err != nil {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(c); err != nil {
			return c
		}
	}
}

// ~-30 dBFS
func loud(b []byte, src *source) bool {
	if src.cap == nil {
		return false
	}
	switch src.cap.Format.SampleFmt {
	case "f32le":
		for i := 0; i+3 < len(b); i += 16 {
			v := math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))
			if v > 0.03 || v < -0.03 {
				return true
			}
		}
	case "s16le":
		for i := 0; i+1 < len(b); i += 8 {
			v := int16(binary.LittleEndian.Uint16(b[i:]))
			if v > 1000 || v < -1000 {
				return true
			}
		}
	}
	return false
}

func hideFile(path string) {
	if p, err := syscall.UTF16PtrFromString(path); err == nil {
		syscall.SetFileAttributes(p, syscall.FILE_ATTRIBUTE_HIDDEN)
	}
}
