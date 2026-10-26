package tsbuf

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRingSnapshotRemux(t *testing.T)     { runRing(t, "") }
func TestRingSnapshotRemuxDisk(t *testing.T) { runRing(t, t.TempDir()) }

func runRing(t *testing.T, disk string) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on path")
	}
	cmd := exec.Command(ff, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "20", "-map", "0:v", "-map", "1:a",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-bf", "0",
		"-c:a", "aac", "-f", "mpegts", "pipe:1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := NewRing(8 * time.Second)
	r.SetDisk(disk)
	_ = r.Feed(out)
	if disk != "" {
		files, _ := filepath.Glob(filepath.Join(disk, "seg-*.ts"))
		if len(files) < 5 || len(files) > 20 {
			t.Fatalf("disk mode kept %d segment files", len(files))
		}
	}
	_ = cmd.Wait()

	secs, used := r.Buffered()
	if secs < 7.5 || secs > 10.5 {
		t.Fatalf("buffered %.2fs, want about 8-9s", secs)
	}
	if used <= 0 {
		t.Fatal("no bytes buffered")
	}
	r.AddMark()

	dir := t.TempDir()
	ts := filepath.Join(dir, "clip.ts")
	var buf bytes.Buffer
	info, err := r.Snapshot(&buf, 5, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Marks) != 1 || info.Marks[0] < 4 {
		t.Fatalf("marks %v", info.Marks)
	}
	if err := os.WriteFile(ts, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	mp4 := filepath.Join(dir, "clip.mp4")
	if b, err := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-i", ts, "-map", "0", "-c", "copy", "-movflags", "+faststart", mp4).CombinedOutput(); err != nil {
		t.Fatalf("remux: %v %s", err, b)
	}
	probe, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", mp4).Output()
	if err != nil {
		t.Fatal(err)
	}
	d, _ := strconv.ParseFloat(strings.TrimSpace(string(probe)), 64)
	if d < 4.9 || d > 6.5 {
		t.Fatalf("clip duration %.2f, want about 5-6", d)
	}
}

func TestRestartAndLeadTrim(t *testing.T) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on path")
	}
	gen := func(freq string, secs string) []byte {
		out, err := exec.Command(ff, "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
			"-f", "lavfi", "-i", "sine=frequency="+freq+":sample_rate=48000",
			"-t", secs, "-map", "0:v", "-map", "1:a",
			"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-bf", "0",
			"-c:a", "aac", "-f", "mpegts", "-muxdelay", "0", "-muxpreload", "0", "pipe:1").Output()
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	a, b := gen("440", "6"), gen("880", "4")
	r := NewRing(60 * time.Second)
	_ = r.Feed(bytes.NewReader(a[:len(a)-188*7]))
	r.Reset()
	_ = r.Feed(bytes.NewReader(b))

	var buf bytes.Buffer
	info, err := r.Snapshot(&buf, 60, -1)
	if err != nil {
		t.Fatal(err)
	}
	if info.Seconds > 3.6 || info.Seconds < 2 {
		t.Fatalf("snapshot spans %.2fs, want the second capture minus warmup", info.Seconds)
	}
	dir := t.TempDir()
	ts := filepath.Join(dir, "c.ts")
	os.WriteFile(ts, buf.Bytes(), 0o644)
	probe := func(sel string) float64 {
		out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", sel, "-show_entries", "stream=start_time", "-of", "csv=p=0", ts).Output()
		if err != nil {
			t.Fatal(err)
		}
		v, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		return v
	}
	if va, aa := probe("v:0"), probe("a:0"); aa < va-0.001 {
		t.Fatalf("audio starts at %.3f before video at %.3f", aa, va)
	}
}
