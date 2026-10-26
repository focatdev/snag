package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"snag/internal/ffmpeg"
)

type TrackMix struct {
	Index  int     `json:"index"`
	Volume float64 `json:"volume"`
	Muted  bool    `json:"muted"`
}

type ExportOptions struct {
	ClipID     string       `json:"clipId"`
	Start      float64      `json:"start"`
	End        float64      `json:"end"`
	Fast       bool         `json:"fast"`
	Format     string       `json:"format"`
	TargetMB   float64      `json:"targetMB"`
	Height     int          `json:"height"`
	FPS        int          `json:"fps"`
	Speed      float64      `json:"speed"`
	MotionBlur string       `json:"motionBlur"`
	Vertical   bool         `json:"vertical"`
	CropX      float64      `json:"cropX"`
	Tracks     []TrackMix   `json:"tracks"`
	MixAudio   bool         `json:"mixAudio"`
	Name       string       `json:"name"`
	Preview    bool         `json:"preview"`
	Cuts       [][2]float64 `json:"cuts"`
	Replace    bool         `json:"replaceOriginal"`
}

type Job struct {
	ID        string  `json:"id"`
	ClipID    string  `json:"clipId"`
	Label     string  `json:"label"`
	Kind      string  `json:"kind"`
	Status    string  `json:"status"`
	Progress  float64 `json:"progress"`
	Error     string  `json:"error"`
	Output    string  `json:"output"`
	Size      int64   `json:"size"`
	OrigSize  int64   `json:"origSize"`
	Replace   bool    `json:"replace"`
	CreatedAt int64   `json:"createdAt"`
	cancel    context.CancelFunc
	opts      ExportOptions
	src       Source
	merge     []Source
	mergeName string
}

type Source struct {
	ID       string
	Path     string
	Title    string
	Size     int64
	Duration float64
	Width    int
	Height   int
	FPS      float64
	Tracks   int
}

type Deps struct {
	Tools    func() ffmpeg.Tools
	Encoders func() []ffmpeg.Encoder
	OutDir   func() string
	TmpDir   string
	OnChange func([]Job)
	OnDone   func(Job)
}

type Manager struct {
	d     Deps
	mu    sync.Mutex
	jobs  []*Job
	queue chan *Job
	last  time.Time
}

func New(d Deps) *Manager {
	m := &Manager{d: d, queue: make(chan *Job, 256)}
	go m.worker()
	return m
}

func newID() string {
	b := make([]byte, 5)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *Manager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, len(m.jobs))
	for i, j := range m.jobs {
		out[i] = *j
		out[i].cancel = nil
	}
	return out
}

func (m *Manager) emit(force bool) {
	m.mu.Lock()
	if !force && time.Since(m.last) < 150*time.Millisecond {
		m.mu.Unlock()
		return
	}
	m.last = time.Now()
	m.mu.Unlock()
	if m.d.OnChange != nil {
		m.d.OnChange(m.List())
	}
}

func (m *Manager) Submit(src Source, o ExportOptions) (Job, error) {
	if o.End <= o.Start {
		o.Start, o.End = 0, src.Duration
	}
	if o.Speed <= 0 {
		o.Speed = 1
	}
	if o.Format == "" {
		o.Format = "mp4"
	}
	if len(o.Tracks) == 0 {
		for i := 0; i < src.Tracks; i++ {
			o.Tracks = append(o.Tracks, TrackMix{Index: i, Volume: 1})
		}
	}
	label := describe(o)
	j := &Job{ID: newID(), ClipID: o.ClipID, Kind: "export", Label: label, Status: "queued", CreatedAt: time.Now().UnixMilli(), opts: o, src: src, Replace: o.Replace && o.Format == "mp4" && !o.Preview}
	if j.Replace {
		j.OrigSize = src.Size
	}
	if o.Preview {
		j.Kind = "preview"
		j.Label = "Preview"
	}
	return m.enqueue(j), nil
}

func (m *Manager) SubmitShrink(src Source) Job {
	j := &Job{ID: newID(), ClipID: src.ID, Kind: "shrink", Label: "Shrink", Status: "queued", CreatedAt: time.Now().UnixMilli(), src: src, OrigSize: src.Size}
	return m.enqueue(j)
}

func (m *Manager) SubmitMerge(srcs []Source, name string) (Job, error) {
	if len(srcs) < 2 {
		return Job{}, errors.New("pick at least two clips")
	}
	j := &Job{ID: newID(), ClipID: srcs[0].ID, Kind: "merge", Label: fmt.Sprintf("Merge %d clips", len(srcs)), Status: "queued", CreatedAt: time.Now().UnixMilli(), merge: srcs, mergeName: name}
	return m.enqueue(j), nil
}

// uploads shouldn't queue behind a 5 minute shrink
type Task func(ctx context.Context, progress func(float64)) (string, error)

func (m *Manager) SubmitTask(kind, clipID, label string, fn Task) Job {
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{ID: newID(), ClipID: clipID, Kind: kind, Label: label, Status: "running", CreatedAt: time.Now().UnixMilli(), cancel: cancel}
	m.mu.Lock()
	m.jobs = append([]*Job{j}, m.jobs...)
	if len(m.jobs) > 40 {
		m.jobs = m.jobs[:40]
	}
	m.mu.Unlock()
	m.emit(true)
	go func() {
		out, err := fn(ctx, func(p float64) { m.setProgress(j, p) })
		cancel()
		m.mu.Lock()
		j.cancel = nil
		switch {
		case errors.Is(err, context.Canceled):
			j.Status = "canceled"
		case err != nil:
			j.Status, j.Error = "failed", err.Error()
		default:
			j.Status, j.Progress, j.Output = "done", 1, out
		}
		m.mu.Unlock()
		m.emit(true)
	}()
	return *j
}

func (m *Manager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.Status == "queued" || j.Status == "running" {
			return true
		}
	}
	return false
}

func (m *Manager) enqueue(j *Job) Job {
	m.mu.Lock()
	m.jobs = append([]*Job{j}, m.jobs...)
	if len(m.jobs) > 40 {
		m.jobs = m.jobs[:40]
	}
	m.mu.Unlock()
	m.queue <- j
	m.emit(true)
	return *j
}

func (m *Manager) Cancel(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			if j.cancel != nil {
				j.cancel()
			}
			if j.Status == "queued" {
				j.Status = "canceled"
			}
		}
	}
}

func (m *Manager) ClearFinished() {
	m.mu.Lock()
	kept := m.jobs[:0]
	for _, j := range m.jobs {
		if j.Status == "queued" || j.Status == "running" {
			kept = append(kept, j)
		}
	}
	m.jobs = kept
	m.mu.Unlock()
	m.emit(true)
}

func (m *Manager) worker() {
	for j := range m.queue {
		m.mu.Lock()
		if j.Status != "queued" {
			m.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		j.cancel = cancel
		j.Status = "running"
		m.mu.Unlock()
		m.emit(true)

		out, err := m.run(ctx, j)
		cancel()

		m.mu.Lock()
		j.cancel = nil
		switch {
		case errors.Is(err, context.Canceled):
			j.Status = "canceled"
			os.Remove(out)
		case err != nil:
			j.Status, j.Error = "failed", err.Error()
			os.Remove(out)
		default:
			j.Status, j.Progress, j.Output = "done", 1, out
			if st, err := os.Stat(out); err == nil {
				j.Size = st.Size()
			}
		}
		done := *j
		m.mu.Unlock()
		m.emit(true)
		if done.Status == "done" && m.d.OnDone != nil {
			m.d.OnDone(done)
		}
	}
}

func (m *Manager) setProgress(j *Job, p float64) {
	m.mu.Lock()
	j.Progress = p
	m.mu.Unlock()
	m.emit(false)
}

func describe(o ExportOptions) string {
	parts := []string{}
	switch {
	case o.Format == "gif":
		parts = append(parts, "GIF")
	case o.TargetMB > 0:
		parts = append(parts, fmt.Sprintf("Fit %g MB", o.TargetMB))
	case o.Fast:
		parts = append(parts, "Trim")
	default:
		parts = append(parts, "Export")
	}
	if o.MotionBlur != "" && o.MotionBlur != "off" {
		parts = append(parts, "blur")
	}
	if o.Vertical {
		parts = append(parts, "vertical")
	}
	if len(o.Cuts) > 0 {
		parts = append(parts, "cut")
	}
	if o.Speed != 1 {
		parts = append(parts, strconv.FormatFloat(o.Speed, 'f', -1, 64)+"x")
	}
	return strings.Join(parts, ", ")
}

func (m *Manager) outputPath(j *Job) string {
	o := j.opts
	ext := ".mp4"
	if o.Format == "gif" {
		ext = ".gif"
	}
	if o.Preview {
		os.MkdirAll(m.d.TmpDir, 0o755)
		return filepath.Join(m.d.TmpDir, "preview-"+j.ID+ext)
	}
	dir := m.d.OutDir()
	os.MkdirAll(dir, 0o755)
	name := strings.TrimSpace(o.Name)
	if name == "" {
		name = j.src.Title + " (" + strings.ToLower(j.Label) + ")"
	}
	name = strings.Map(func(c rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, c) || c < 32 {
			return -1
		}
		return c
	}, name)
	p := filepath.Join(dir, name+ext)
	for i := 2; ; i++ {
		if _, err := os.Stat(p); err != nil {
			return p
		}
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", name, i, ext))
	}
}

func (m *Manager) exportEncoder() string {
	avail := map[string]bool{}
	for _, e := range m.d.Encoders() {
		if e.Available {
			avail[e.Name] = true
		}
	}
	for _, n := range []string{"h264_nvenc", "h264_amf", "h264_qsv"} {
		if avail[n] {
			return n
		}
	}
	return "libx264"
}

func (m *Manager) run(ctx context.Context, j *Job) (string, error) {
	t := m.d.Tools()
	if t.FFmpeg == "" {
		return "", errors.New("FFmpeg is not installed")
	}
	switch j.Kind {
	case "shrink":
		return m.runShrink(ctx, j)
	case "merge":
		return m.runMerge(ctx, j)
	}
	out := m.outputPath(j)
	o := j.opts
	if o.Preview {
		mid := o.Start
		if o.End-o.Start > 3 {
			mid = o.Start + (o.End-o.Start)/2 - 1.5
		}
		o.Start, o.End = mid, math.Min(o.End, mid+3)
		o.TargetMB = 0
	}
	outDur := keptLength(o) / o.Speed

	if o.Fast && canCopy(o) {
		args := copyArgs(j.src, o, out)
		return out, ffmpeg.RunProgress(ctx, t.FFmpeg, outDur, func(p float64) { m.setProgress(j, p) }, args...)
	}

	enc := m.exportEncoder()
	kbps := 0
	if o.TargetMB > 0 {
		kbps = targetKbps(o.TargetMB, outDur, audioKbps(o))
	}
	args := encodeArgs(j.src, o, enc, kbps, out)
	err := ffmpeg.RunProgress(ctx, t.FFmpeg, outDur, func(p float64) { m.setProgress(j, p) }, args...)
	if err != nil && enc != "libx264" && ctx.Err() == nil {
		// hw encoders choke on odd sizes
		enc = "libx264"
		args = encodeArgs(j.src, o, enc, kbps, out)
		err = ffmpeg.RunProgress(ctx, t.FFmpeg, outDur, func(p float64) { m.setProgress(j, p) }, args...)
	}
	if err != nil {
		return out, err
	}
	if o.TargetMB > 0 {
		limit := int64(o.TargetMB * 1024 * 1024)
		if st, err := os.Stat(out); err == nil && st.Size() > limit {
			kbps = int(float64(kbps) * float64(limit) / float64(st.Size()) * 0.92)
			m.setProgress(j, 0)
			args = encodeArgs(j.src, o, enc, kbps, out)
			if err := ffmpeg.RunProgress(ctx, t.FFmpeg, outDur, func(p float64) { m.setProgress(j, p) }, args...); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

func canCopy(o ExportOptions) bool {
	if o.Format != "mp4" || o.TargetMB > 0 || o.Height > 0 || o.FPS > 0 || o.Speed != 1 || o.Vertical || len(o.Cuts) > 0 {
		return false
	}
	if o.MotionBlur != "" && o.MotionBlur != "off" {
		return false
	}
	for _, t := range o.Tracks {
		if !t.Muted && math.Abs(t.Volume-1) > 0.01 {
			return false
		}
	}
	return !o.MixAudio || activeTracks(o) <= 1
}

func activeTracks(o ExportOptions) int {
	n := 0
	for _, t := range o.Tracks {
		if !t.Muted {
			n++
		}
	}
	return n
}

func audioKbps(o ExportOptions) int {
	if o.Format == "gif" || activeTracks(o) == 0 {
		return 0
	}
	if o.MixAudio {
		return 128
	}
	return 128 * activeTracks(o)
}

func targetKbps(mb, dur float64, audio int) int {
	if dur <= 0 {
		dur = 1
	}
	total := mb * 1024 * 1024 * 8 / 1000 / dur * 0.94
	v := int(total) - audio
	if v < 150 {
		v = 150
	}
	return v
}

func f2(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }

func copyArgs(src Source, o ExportOptions, out string) []string {
	a := []string{"-y", "-ss", f2(o.Start), "-to", f2(o.End), "-i", src.Path, "-map", "0:v:0"}
	for _, t := range o.Tracks {
		if !t.Muted && t.Index < src.Tracks {
			a = append(a, "-map", fmt.Sprintf("0:a:%d", t.Index))
		}
	}
	return append(a, "-c", "copy", "-avoid_negative_ts", "make_zero", "-movflags", "+faststart", out)
}

func blurFrames(level string, srcFPS, outFPS float64) int {
	if outFPS <= 0 {
		outFPS = srcFPS
	}
	ratio := srcFPS / outFPS
	shutter := map[string]float64{"subtle": 0.5, "medium": 1, "heavy": 1.75}[level]
	n := int(math.Round(ratio * shutter))
	base := map[string]int{"subtle": 2, "medium": 3, "heavy": 5}[level]
	if n < base {
		n = base
	}
	if n > 16 {
		n = 16
	}
	return n
}

// cuts are absolute, output is relative to o.Start
func keepSegments(o ExportOptions) [][2]float64 {
	dur := o.End - o.Start
	var cuts [][2]float64
	for _, c := range o.Cuts {
		a, b := math.Max(0, c[0]-o.Start), math.Min(dur, c[1]-o.Start)
		if b-a > 0.05 {
			cuts = append(cuts, [2]float64{a, b})
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i][0] < cuts[j][0] })
	var keep [][2]float64
	pos := 0.0
	for _, c := range cuts {
		if c[0] > pos+0.05 {
			keep = append(keep, [2]float64{pos, c[0]})
		}
		pos = math.Max(pos, c[1])
	}
	if dur > pos+0.05 {
		keep = append(keep, [2]float64{pos, dur})
	}
	if len(keep) == 0 {
		keep = [][2]float64{{0, dur}}
	}
	return keep
}

func keptLength(o ExportOptions) float64 {
	total := 0.0
	for _, k := range keepSegments(o) {
		total += k[1] - k[0]
	}
	return total
}

func encodeArgs(src Source, o ExportOptions, enc string, kbps int, out string) []string {
	a := []string{"-y", "-hwaccel", "auto", "-ss", f2(o.Start), "-t", f2(o.End - o.Start), "-i", src.Path}

	srcFPS := src.FPS
	if srcFPS <= 0 {
		srcFPS = 60
	}
	outFPS := float64(o.FPS)
	if outFPS <= 0 || outFPS > srcFPS {
		outFPS = srcFPS
	}
	height := o.Height
	if kbps > 0 && height == 0 {
		switch {
		case kbps < 900 && src.Height > 480:
			height = 480
		case kbps < 2200 && src.Height > 720:
			height = 720
		case kbps < 4500 && src.Height > 1080:
			height = 1080
		}
	}

	var active []TrackMix
	for _, t := range o.Tracks {
		if !t.Muted && t.Index < src.Tracks {
			active = append(active, t)
		}
	}
	if o.Format == "gif" {
		active = nil
	}

	var fc []string
	vin := "[0:v]"
	ain := make([]string, len(active))
	for i, t := range active {
		ain[i] = fmt.Sprintf("[0:a:%d]", t.Index)
	}
	if segs := keepSegments(o); len(segs) > 1 {
		var cat string
		for i, sg := range segs {
			fc = append(fc, fmt.Sprintf("[0:v]trim=start=%s:end=%s,setpts=PTS-STARTPTS[sv%d]", f2(sg[0]), f2(sg[1]), i))
			cat += fmt.Sprintf("[sv%d]", i)
			for k, t := range active {
				fc = append(fc, fmt.Sprintf("[0:a:%d]atrim=start=%s:end=%s,asetpts=PTS-STARTPTS[sa%d_%d]", t.Index, f2(sg[0]), f2(sg[1]), i, k))
				cat += fmt.Sprintf("[sa%d_%d]", i, k)
			}
		}
		outs := "[cv]"
		for k := range active {
			outs += fmt.Sprintf("[ca%d]", k)
			ain[k] = fmt.Sprintf("[ca%d]", k)
		}
		fc = append(fc, fmt.Sprintf("%sconcat=n=%d:v=1:a=%d%s", cat, len(segs), len(active), outs))
		vin = "[cv]"
	}

	var vf []string
	if o.MotionBlur != "" && o.MotionBlur != "off" {
		n := blurFrames(o.MotionBlur, srcFPS, outFPS)
		vf = append(vf, fmt.Sprintf("tmix=frames=%d:weights=%s", n, strings.TrimSpace(strings.Repeat("1 ", n))))
	}
	if o.Speed != 1 {
		vf = append(vf, fmt.Sprintf("setpts=PTS/%s", strconv.FormatFloat(o.Speed, 'f', -1, 64)))
	}
	if outFPS != srcFPS || o.MotionBlur != "" && o.MotionBlur != "off" || o.Speed != 1 {
		vf = append(vf, "fps="+strconv.FormatFloat(outFPS, 'f', -1, 64))
	}
	if o.Vertical {
		cx := math.Max(0, math.Min(1, o.CropX))
		vf = append(vf, fmt.Sprintf("crop=w=trunc(ih*9/16/2)*2:h=ih:x=(iw-trunc(ih*9/16/2)*2)*%s:y=0", f2(cx)))
	}

	if o.Format == "gif" {
		gh := height
		if gh == 0 || gh > 480 {
			gh = 360
		}
		gfps := math.Min(outFPS, 20)
		chain := append(vf, fmt.Sprintf("fps=%s,scale=-2:%d:flags=lanczos,split[ga][gb];[ga]palettegen=stats_mode=diff[gp];[gb][gp]paletteuse=dither=bayer:bayer_scale=4", strconv.FormatFloat(gfps, 'f', -1, 64), gh))
		fc = append(fc, vin+strings.Join(chain, ","))
		return append(a, "-filter_complex", strings.Join(fc, ";"), "-an", "-loop", "0", out)
	}

	if height > 0 && height < src.Height {
		vf = append(vf, fmt.Sprintf("scale=-2:%d:flags=lanczos", height))
	}
	vf = append(vf, "format=yuv420p")
	fc = append(fc, vin+strings.Join(vf, ",")+"[vout]")
	maps := []string{"-map", "[vout]"}

	tempo := ""
	if o.Speed != 1 {
		tempo = atempo(o.Speed)
	}
	if len(active) > 0 {
		if o.MixAudio && len(active) > 1 {
			var ins string
			for i, t := range active {
				fc = append(fc, fmt.Sprintf("%svolume=%s[a%d]", ain[i], f2(t.Volume), i))
				ins += fmt.Sprintf("[a%d]", i)
			}
			mix := fmt.Sprintf("%samix=inputs=%d:normalize=0:dropout_transition=0", ins, len(active))
			if tempo != "" {
				mix += "," + tempo
			}
			fc = append(fc, mix+"[aout]")
			maps = append(maps, "-map", "[aout]")
		} else {
			for i, t := range active {
				chain := fmt.Sprintf("%svolume=%s", ain[i], f2(t.Volume))
				if tempo != "" {
					chain += "," + tempo
				}
				fc = append(fc, fmt.Sprintf("%s[aout%d]", chain, i))
				maps = append(maps, "-map", fmt.Sprintf("[aout%d]", i))
			}
		}
	}
	a = append(a, "-filter_complex", strings.Join(fc, ";"))
	a = append(a, maps...)
	a = append(a, videoEncoder(enc, kbps)...)
	if len(active) > 0 {
		a = append(a, "-c:a", "aac", "-b:a", "128k")
	}
	return append(a, "-movflags", "+faststart", out)
}

func (m *Manager) available() map[string]bool {
	avail := map[string]bool{}
	for _, e := range m.d.Encoders() {
		if e.Available {
			avail[e.Name] = true
		}
	}
	return avail
}

func (m *Manager) runShrink(ctx context.Context, j *Job) (string, error) {
	t := m.d.Tools()
	src := j.src
	out := filepath.Join(filepath.Dir(src.Path), ".snag-shrink-"+j.ID+".mp4")
	avail := m.available()
	type try struct {
		enc  string
		args []string
	}
	tries := []try{
		{"av1_nvenc", []string{"-preset", "p5", "-rc", "vbr", "-cq", "33", "-b:v", "0"}},
		{"av1_amf", []string{"-quality", "quality", "-rc", "qvbr", "-qvbr_quality_level", "30"}},
		{"av1_qsv", []string{"-preset", "slower", "-global_quality", "30"}},
		{"hevc_nvenc", []string{"-preset", "p5", "-rc", "vbr", "-cq", "29", "-b:v", "0", "-tag:v", "hvc1"}},
		{"hevc_amf", []string{"-quality", "quality", "-rc", "qvbr", "-qvbr_quality_level", "26", "-tag:v", "hvc1"}},
		{"hevc_qsv", []string{"-preset", "slower", "-global_quality", "27", "-tag:v", "hvc1"}},
		{"libsvtav1", []string{"-preset", "10", "-crf", "36"}},
	}
	var lastErr error
	for _, tr := range tries {
		if len(avail) > 0 && !avail[tr.enc] {
			continue
		}
		args := []string{"-y", "-hwaccel", "auto", "-i", src.Path, "-map", "0", "-c:v", tr.enc}
		args = append(args, tr.args...)
		args = append(args, ffmpeg.HWArgs(tr.enc)...)
		args = append(args, "-c:a", "copy", "-movflags", "+faststart", out)
		err := ffmpeg.RunProgress(ctx, t.FFmpeg, src.Duration, func(p float64) { m.setProgress(j, p) }, args...)
		if err == nil {
			return out, nil
		}
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no encoder can shrink clips on this PC")
	}
	return out, lastErr
}

func (m *Manager) runMerge(ctx context.Context, j *Job) (string, error) {
	t := m.d.Tools()
	srcs := j.merge
	dir := m.d.OutDir()
	os.MkdirAll(dir, 0o755)
	name := strings.TrimSpace(j.mergeName)
	if name == "" {
		name = "Montage " + time.Now().Format("2006-01-02 15-04-05")
	}
	out := filepath.Join(dir, name+".mp4")
	for i := 2; ; i++ {
		if _, err := os.Stat(out); err != nil {
			break
		}
		out = filepath.Join(dir, fmt.Sprintf("%s (%d).mp4", name, i))
	}
	w, h, fps := srcs[0].Width, srcs[0].Height, srcs[0].FPS
	if w <= 0 || h <= 0 {
		w, h = 1920, 1080
	}
	if fps <= 0 {
		fps = 60
	}
	var args []string
	var fc []string
	var cat string
	total := 0.0
	args = append(args, "-y")
	for _, s := range srcs {
		args = append(args, "-hwaccel", "auto", "-i", s.Path)
	}
	for i, s := range srcs {
		total += s.Duration
		fc = append(fc, fmt.Sprintf("[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%s,format=yuv420p[v%d]", i, w, h, w, h, strconv.FormatFloat(fps, 'f', -1, 64), i))
		switch {
		case s.Tracks > 1:
			var ins string
			for k := 0; k < s.Tracks; k++ {
				ins += fmt.Sprintf("[%d:a:%d]", i, k)
			}
			fc = append(fc, fmt.Sprintf("%samix=inputs=%d:normalize=0:dropout_transition=0,aresample=48000,aformat=channel_layouts=stereo[a%d]", ins, s.Tracks, i))
		case s.Tracks == 1:
			fc = append(fc, fmt.Sprintf("[%d:a:0]aresample=48000,aformat=channel_layouts=stereo[a%d]", i, i))
		default:
			fc = append(fc, fmt.Sprintf("anullsrc=r=48000:cl=stereo,atrim=duration=%s[a%d]", f2(s.Duration), i))
		}
		cat += fmt.Sprintf("[v%d][a%d]", i, i)
	}
	fc = append(fc, fmt.Sprintf("%sconcat=n=%d:v=1:a=1[vout][aout]", cat, len(srcs)))
	args = append(args, "-filter_complex", strings.Join(fc, ";"), "-map", "[vout]", "-map", "[aout]")
	enc := m.exportEncoder()
	build := func(enc string) []string {
		a := append([]string(nil), args...)
		a = append(a, videoEncoder(enc, 0)...)
		return append(a, "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", out)
	}
	err := ffmpeg.RunProgress(ctx, t.FFmpeg, total, func(p float64) { m.setProgress(j, p) }, build(enc)...)
	if err != nil && enc != "libx264" && ctx.Err() == nil {
		err = ffmpeg.RunProgress(ctx, t.FFmpeg, total, func(p float64) { m.setProgress(j, p) }, build("libx264")...)
	}
	return out, err
}

func atempo(speed float64) string {
	var parts []string
	for speed > 2 {
		parts = append(parts, "atempo=2")
		speed /= 2
	}
	for speed < 0.5 {
		parts = append(parts, "atempo=0.5")
		speed /= 0.5
	}
	parts = append(parts, "atempo="+strconv.FormatFloat(speed, 'f', 4, 64))
	return strings.Join(parts, ",")
}

func videoEncoder(enc string, kbps int) []string {
	a := []string{"-c:v", enc}
	if kbps > 0 {
		b := strconv.Itoa(kbps) + "k"
		max := strconv.Itoa(kbps*6/5) + "k"
		buf := strconv.Itoa(kbps*2) + "k"
		switch {
		case strings.HasSuffix(enc, "_nvenc"):
			a = append(a, "-preset", "p6", "-rc", "vbr", "-multipass", "fullres", "-b:v", b, "-maxrate", max, "-bufsize", buf)
		case strings.HasSuffix(enc, "_amf"):
			a = append(a, "-quality", "quality", "-rc", "vbr_peak", "-b:v", b, "-maxrate", max, "-bufsize", buf)
		case strings.HasSuffix(enc, "_qsv"):
			a = append(a, "-preset", "slower", "-b:v", b, "-maxrate", max, "-bufsize", buf)
		default:
			a = append(a, "-preset", "medium", "-b:v", b, "-maxrate", max, "-bufsize", buf)
		}
	} else {
		switch {
		case strings.HasSuffix(enc, "_nvenc"):
			a = append(a, "-preset", "p5", "-rc", "vbr", "-cq", "21", "-b:v", "0", "-maxrate", "60M", "-bufsize", "120M")
		case strings.HasSuffix(enc, "_amf"):
			a = append(a, "-quality", "quality", "-rc", "qvbr", "-qvbr_quality_level", "22", "-b:v", "20M", "-maxrate", "60M")
		case strings.HasSuffix(enc, "_qsv"):
			a = append(a, "-preset", "slower", "-global_quality", "22")
		default:
			a = append(a, "-preset", "veryfast", "-crf", "20")
		}
	}
	if strings.HasPrefix(enc, "h264") || enc == "libx264" {
		a = append(a, "-profile:v", "high")
	}
	return append(a, ffmpeg.HWArgs(enc)...)
}
