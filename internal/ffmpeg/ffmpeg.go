package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Tools struct {
	FFmpeg  string `json:"ffmpeg"`
	FFprobe string `json:"ffprobe"`
	Version string `json:"version"`
}

func exeName(n string) string {
	if isWindows {
		return n + ".exe"
	}
	return n
}

func Locate(override, installDir string) (Tools, error) {
	var candidates []string
	if override != "" {
		if st, err := os.Stat(override); err == nil && st.IsDir() {
			candidates = append(candidates, filepath.Join(override, exeName("ffmpeg")))
		} else {
			candidates = append(candidates, override)
		}
	}
	candidates = append(candidates, filepath.Join(installDir, exeName("ffmpeg")))
	if p, err := exec.LookPath(exeName("ffmpeg")); err == nil {
		candidates = append(candidates, p)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err != nil {
			continue
		}
		t := Tools{FFmpeg: c}
		probe := filepath.Join(filepath.Dir(c), exeName("ffprobe"))
		if _, err := os.Stat(probe); err == nil {
			t.FFprobe = probe
		} else if p, err := exec.LookPath(exeName("ffprobe")); err == nil {
			t.FFprobe = p
		}
		t.Version = version(c)
		return t, nil
	}
	return Tools{}, errors.New("ffmpeg not found")
}

func version(path string) string {
	out, err := Output(context.Background(), path, "-hide_banner", "-version")
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	f := strings.Fields(line)
	if len(f) >= 3 {
		return f[2]
	}
	return strings.TrimSpace(line)
}

func Command(ctx context.Context, bin string, lowPriority bool, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	hide(cmd, lowPriority)
	cmd.WaitDelay = 3 * time.Second
	return cmd
}

func Output(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := Command(ctx, bin, false, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, Tail(stderr.String(), 600))
	}
	return out, nil
}

// below normal priority, exports shouldn't cost game frames
func Run(ctx context.Context, bin string, args ...string) error {
	cmd := Command(ctx, bin, true, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg: %s", Tail(stderr.String(), 600))
	}
	return nil
}

func RunProgress(ctx context.Context, bin string, duration float64, onProgress func(float64), args ...string) error {
	full := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error", "-progress", "pipe:1", "-nostats"}, args...)
	cmd := Command(ctx, bin, true, full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch k {
		case "out_time_us", "out_time_ms":
			if us, err := strconv.ParseFloat(v, 64); err == nil && duration > 0 && onProgress != nil {
				p := us / 1e6 / duration
				if p > 1 {
					p = 1
				}
				if p >= 0 {
					onProgress(p)
				}
			}
		case "progress":
			if v == "end" && onProgress != nil {
				onProgress(1)
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg: %s", Tail(stderr.String(), 600))
	}
	return nil
}

func Tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = "..." + s[len(s)-n:]
	}
	if s == "" {
		return "unknown error"
	}
	return s
}

type MediaInfo struct {
	Duration    float64
	Width       int
	Height      int
	FPS         float64
	VideoCodec  string
	AudioTracks []string
	Size        int64
}

func Probe(ctx context.Context, ffprobe, file string) (MediaInfo, error) {
	var mi MediaInfo
	if ffprobe == "" {
		return mi, errors.New("ffprobe not found")
	}
	out, err := Output(ctx, ffprobe, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", file)
	if err != nil {
		return mi, err
	}
	var res struct {
		Format struct {
			Duration string `json:"duration"`
			Size     string `json:"size"`
		} `json:"format"`
		Streams []struct {
			CodecType  string            `json:"codec_type"`
			CodecName  string            `json:"codec_name"`
			Width      int               `json:"width"`
			Height     int               `json:"height"`
			AvgFrame   string            `json:"avg_frame_rate"`
			RFrameRate string            `json:"r_frame_rate"`
			Tags       map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return mi, err
	}
	mi.Duration, _ = strconv.ParseFloat(res.Format.Duration, 64)
	mi.Size, _ = strconv.ParseInt(res.Format.Size, 10, 64)
	for _, s := range res.Streams {
		switch s.CodecType {
		case "video":
			if mi.VideoCodec != "" {
				continue
			}
			mi.VideoCodec, mi.Width, mi.Height = s.CodecName, s.Width, s.Height
			mi.FPS = ratio(s.AvgFrame)
			if mi.FPS == 0 {
				mi.FPS = ratio(s.RFrameRate)
			}
		case "audio":
			name := s.Tags["title"]
			if name == "" {
				name = fmt.Sprintf("Track %d", len(mi.AudioTracks)+1)
			}
			mi.AudioTracks = append(mi.AudioTracks, name)
		}
	}
	return mi, nil
}

func ratio(s string) float64 {
	a, b, ok := strings.Cut(s, "/")
	if !ok {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	n, _ := strconv.ParseFloat(a, 64)
	d, _ := strconv.ParseFloat(b, 64)
	if d == 0 {
		return 0
	}
	return n / d
}

type Encoder struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Codec     string `json:"codec"`
	Vendor    string `json:"vendor"`
	Hardware  bool   `json:"hardware"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

var Candidates = []Encoder{
	{Name: "h264_nvenc", Label: "NVENC H.264", Codec: "h264", Vendor: "nvidia", Hardware: true},
	{Name: "hevc_nvenc", Label: "NVENC HEVC", Codec: "hevc", Vendor: "nvidia", Hardware: true},
	{Name: "h264_amf", Label: "AMF H.264", Codec: "h264", Vendor: "amd", Hardware: true},
	{Name: "hevc_amf", Label: "AMF HEVC", Codec: "hevc", Vendor: "amd", Hardware: true},
	{Name: "h264_qsv", Label: "Quick Sync H.264", Codec: "h264", Vendor: "intel", Hardware: true},
	{Name: "hevc_qsv", Label: "Quick Sync HEVC", Codec: "hevc", Vendor: "intel", Hardware: true},
	{Name: "libx264", Label: "x264 (CPU)", Codec: "h264", Vendor: "cpu"},
	{Name: "libx265", Label: "x265 (CPU)", Codec: "hevc", Vendor: "cpu"},
	{Name: "av1_nvenc", Label: "NVENC AV1", Codec: "av1", Vendor: "nvidia", Hardware: true},
	{Name: "av1_amf", Label: "AMF AV1", Codec: "av1", Vendor: "amd", Hardware: true},
	{Name: "av1_qsv", Label: "Quick Sync AV1", Codec: "av1", Vendor: "intel", Hardware: true},
	{Name: "libsvtav1", Label: "SVT-AV1 (CPU)", Codec: "av1", Vendor: "cpu"},
}

// -encoders lists what's compiled in, not what the driver can do
func ProbeEncoders(ctx context.Context, bin string) []Encoder {
	out := make([]Encoder, len(Candidates))
	copy(out, Candidates)
	groups := map[string][]*Encoder{}
	for i := range out {
		groups[out[i].Vendor] = append(groups[out[i].Vendor], &out[i])
	}
	var wg sync.WaitGroup
	for vendor, list := range groups {
		wg.Add(1)
		go func(vendor string, list []*Encoder) {
			defer wg.Done()
			// consumer cards cap concurrent sessions
			if vendor != "cpu" {
				for _, e := range list {
					probeOne(ctx, bin, e)
					if e.Available || !vendorMissing(e.Error) {
						continue
					}
					for _, rest := range list {
						if rest != e && rest.Error == "" && !rest.Available {
							rest.Error = e.Error
						}
					}
					return
				}
				return
			}
			var inner sync.WaitGroup
			for _, e := range list {
				inner.Add(1)
				go func(e *Encoder) {
					defer inner.Done()
					probeOne(ctx, bin, e)
				}(e)
			}
			inner.Wait()
		}(vendor, list)
	}
	wg.Wait()
	return out
}

// nvidia bug 5830620: b-frames trip an invalid level error on some drivers
func HWArgs(enc string) []string {
	if strings.HasSuffix(enc, "_nvenc") || strings.HasSuffix(enc, "_amf") || strings.HasSuffix(enc, "_qsv") {
		return []string{"-bf", "0"}
	}
	return nil
}

func testEncode(ctx context.Context, bin, enc, level string) (string, error) {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", level, "-f", "lavfi", "-i", "color=c=black:s=1280x720:r=30",
		"-frames:v", "8", "-pix_fmt", "yuv420p", "-c:v", enc}
	args = append(args, HWArgs(enc)...)
	args = append(args, "-f", "null", "-")
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := Command(c, bin, true, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil && c.Err() != nil {
		err = c.Err()
	}
	return stderr.String(), err
}

func probeOne(ctx context.Context, bin string, e *Encoder) {
	if e.Error != "" {
		return
	}
	for attempt := 0; attempt < 2; attempt++ {
		log, err := testEncode(ctx, bin, e.Name, "error")
		if err == nil {
			e.Available, e.Error, e.Detail = true, "", ""
			return
		}
		if errors.Is(err, context.DeadlineExceeded) {
			e.Error = "Timed out"
		} else {
			e.Error = Explain(log)
		}
		if !strings.Contains(e.Error, "busy") && e.Error != "Timed out" {
			break
		}
	}
	// for bug reports
	if e.Hardware && !vendorMissing(e.Error) {
		log, _ := testEncode(ctx, bin, e.Name, "verbose")
		e.Detail = Tail(strings.TrimSpace(log), 6000)
	}
}

var minDriver = regexp.MustCompile(`(?i)minimum required nvidia driver for nvenc is ([\d.]+)`)
var apiFound = regexp.MustCompile(`(?i)required: ([\d.]+) found: ([\d.]+)`)

func vendorMissing(msg string) bool {
	return strings.HasPrefix(msg, "No NVIDIA driver") || strings.HasPrefix(msg, "No AMD driver") || strings.HasPrefix(msg, "No Intel GPU") ||
		strings.HasPrefix(msg, "NVIDIA driver too old")
}

func Explain(raw string) string {
	low := strings.ToLower(raw)
	switch {
	case strings.Contains(low, "deadline exceeded") || strings.Contains(low, "context canceled"):
		return "Timed out"
	case strings.Contains(low, "unknown encoder") || strings.Contains(low, "encoder not found"):
		return "Not in this FFmpeg build"
	case minDriver.MatchString(raw):
		return "NVIDIA driver too old, needs " + minDriver.FindStringSubmatch(raw)[1] + " or newer"
	case strings.Contains(low, "required nvenc api version") || strings.Contains(low, "driver does not support the required nvenc api"):
		if m := apiFound.FindStringSubmatch(raw); m != nil {
			return "NVIDIA driver too old, it speaks NVENC " + m[2] + " and this FFmpeg needs " + m[1]
		}
		return "NVIDIA driver too old for this FFmpeg"
	case strings.Contains(low, "nvcuda.dll") || strings.Contains(low, "nvencodeapi64.dll") || strings.Contains(low, "cuinit"):
		return "No NVIDIA driver found"
	case strings.Contains(low, "no capable devices") || strings.Contains(low, "no nvenc capable devices"):
		return "This GPU can't encode this format"
	case strings.Contains(low, "incompatible client key") || strings.Contains(low, "openencodesessionex failed: out of memory"):
		return "Encoder busy, another app is using it"
	case strings.Contains(low, "amfrt64.dll") || strings.Contains(low, "amf") && strings.Contains(low, "failed to open"):
		return "No AMD driver found"
	case strings.Contains(low, "mfx session") || strings.Contains(low, "libmfx") || strings.Contains(low, "vpl"):
		return "No Intel GPU found"
	}
	return keyLine(strings.TrimPrefix(raw, "ffmpeg: "))
}

var ctxPrefix = regexp.MustCompile(`^\[([^\]@]+?)\s*@\s*[0-9a-fA-Fx]+\]\s*`)

var generic = []string{"terminating thread", "task finished", "error while opening encoder", "could not open encoder", "error sending frames",
	"nothing was written", "conversion failed", "error while filtering", "error while processing", "exiting normally"}

// the real error is above the generic "terminating thread" lines
func keyLine(raw string) string {
	var fallback string
	for _, l := range strings.Split(strings.TrimSpace(raw), "\n") {
		l = strings.TrimSpace(l)
		low := strings.ToLower(l)
		if l == "" || slices.ContainsFunc(generic, func(g string) bool { return strings.Contains(low, g) }) {
			continue
		}
		m := ctxPrefix.FindStringSubmatch(l)
		text := ctxPrefix.ReplaceAllString(l, "")
		if len(text) > 160 {
			text = text[:160]
		}
		if m != nil && strings.ContainsAny(m[1], "_") && !strings.Contains(m[1], "#") {
			return text
		}
		if fallback == "" {
			fallback = text
		}
	}
	if fallback != "" {
		return fallback
	}
	return "Test encode failed"
}

type Features struct {
	DDAGrab     bool `json:"ddagrab"`
	ScaleD3D11  bool `json:"scaleD3d11"`
	GDIGrab     bool `json:"gdigrab"`
	ScaleCUDA   bool `json:"scaleCuda"`
	GfxCapture  bool `json:"gfxcapture"`
	LibWebP     bool `json:"libwebp"`
	Initialised bool `json:"initialised"`
}

func DetectFeatures(ctx context.Context, bin string) Features {
	f := Features{Initialised: true}
	if out, err := Output(ctx, bin, "-hide_banner", "-filters"); err == nil {
		s := string(out)
		f.DDAGrab = strings.Contains(s, " ddagrab ")
		f.ScaleD3D11 = strings.Contains(s, " scale_d3d11 ")
		f.ScaleCUDA = strings.Contains(s, " scale_cuda ")
		f.GfxCapture = strings.Contains(s, " gfxcapture ")
	}
	if out, err := Output(ctx, bin, "-hide_banner", "-devices"); err == nil {
		f.GDIGrab = strings.Contains(string(out), "gdigrab")
	}
	return f
}

type TailBuffer struct {
	mu  sync.Mutex
	buf []byte
	n   int
}

func NewTail(n int) *TailBuffer { return &TailBuffer{n: n} }

func (t *TailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.n {
		t.buf = t.buf[len(t.buf)-t.n:]
	}
	t.mu.Unlock()
	return len(p), nil
}

func (t *TailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

var _ io.Writer = (*TailBuffer)(nil)
