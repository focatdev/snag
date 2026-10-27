package recorder

import (
	"fmt"
	"strconv"
	"strings"

	"snag/internal/ffmpeg"
)

type pipeline string

const (
	pipeGPU    pipeline = "gpu"
	pipeWGC    pipeline = "wgc"
	pipeCUDA   pipeline = "cuda"
	pipeSystem pipeline = "system"
	pipeGDI    pipeline = "gdi"
)

type audioInput struct {
	Title      string
	PipeName   string
	SampleFmt  string
	SampleRate int
	Channels   int
	Mono       bool
}

type captureSpec struct {
	Pipeline   pipeline
	Encoder    string
	Monitor    int
	MonLeft    int
	MonTop     int
	MonHandle  uintptr
	SrcW, SrcH int
	OutW, OutH int
	FPS        int
	Kbps       int
	Cursor     bool
	Audio      []audioInput
}

func vendorOf(enc string) string {
	switch {
	case strings.HasSuffix(enc, "_nvenc"):
		return "nvidia"
	case strings.HasSuffix(enc, "_amf"):
		return "amd"
	case strings.HasSuffix(enc, "_qsv"):
		return "intel"
	}
	return "cpu"
}

func chooseEncoder(pref, codec string, list []ffmpeg.Encoder) string {
	avail := map[string]bool{}
	for _, e := range list {
		if e.Available {
			avail[e.Name] = true
		}
	}
	if pref != "" && pref != "auto" && (avail[pref] || len(list) == 0) {
		return pref
	}
	order := []string{"nvenc", "amf", "qsv"}
	for _, c := range []string{codec, "h264"} {
		for _, v := range order {
			if n := c + "_" + v; avail[n] {
				return n
			}
		}
	}
	if codec == "hevc" && avail["libx265"] {
		return "libx265"
	}
	return "libx264"
}

func encoderArgs(enc string, fps, kbps int) []string {
	g := strconv.Itoa(fps)
	b := strconv.Itoa(kbps) + "k"
	max := strconv.Itoa(kbps*3/2) + "k"
	buf := strconv.Itoa(kbps*2) + "k"
	a := []string{"-c:v", enc}
	switch vendorOf(enc) {
	case "nvidia":
		a = append(a, "-preset", "p4", "-tune", "hq", "-rc", "vbr", "-b:v", b, "-maxrate", max, "-bufsize", buf, "-spatial-aq", "1", "-forced-idr", "1")
		if strings.HasPrefix(enc, "h264") {
			a = append(a, "-profile:v", "high")
		}
	case "amd":
		a = append(a, "-usage", "transcoding", "-quality", "balanced", "-rc", "vbr_peak", "-b:v", b, "-maxrate", max, "-bufsize", buf)
	case "intel":
		a = append(a, "-preset", "faster", "-b:v", b, "-maxrate", max, "-bufsize", buf)
	default:
		a = append(a, "-preset", "ultrafast", "-tune", "zerolatency", "-b:v", b, "-maxrate", max, "-bufsize", buf)
	}
	return append(a, "-g", g, "-bf", "0")
}

func buildArgs(s captureSpec) []string {
	cursor := "0"
	if s.Cursor {
		cursor = "1"
	}
	fps := strconv.Itoa(s.FPS)
	a := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}

	switch s.Pipeline {
	case pipeGDI:
		a = append(a, "-f", "gdigrab", "-framerate", fps, "-draw_mouse", cursor,
			"-offset_x", strconv.Itoa(s.MonLeft), "-offset_y", strconv.Itoa(s.MonTop),
			"-video_size", fmt.Sprintf("%dx%d", s.SrcW, s.SrcH), "-i", "desktop")
	case pipeWGC:
		a = append(a, "-f", "lavfi", "-i", fmt.Sprintf("gfxcapture=hmonitor=%d:capture_cursor=%s:max_framerate=%s:width=%d:height=%d:resize_mode=scale", s.MonHandle, cursor, fps, s.OutW, s.OutH))
	default:
		a = append(a, "-f", "lavfi", "-i", fmt.Sprintf("ddagrab=output_idx=%d:framerate=%s:draw_mouse=%s:dup_frames=1", s.Monitor, fps, cursor))
	}
	for _, in := range s.Audio {
		a = append(a, "-thread_queue_size", "4096", "-f", in.SampleFmt, "-ar", strconv.Itoa(in.SampleRate),
			"-ac", strconv.Itoa(in.Channels), "-i", in.PipeName)
	}

	a = append(a, "-map", "0:v")
	for i := range s.Audio {
		a = append(a, "-map", fmt.Sprintf("%d:a", i+1))
	}

	scale := ""
	if s.OutH != s.SrcH || s.OutW != s.SrcW {
		scale = fmt.Sprintf("scale=%d:%d:flags=bilinear,", s.OutW, s.OutH)
	}
	pixfmt := "nv12"
	if vendorOf(s.Encoder) == "cpu" {
		pixfmt = "yuv420p"
	}
	switch s.Pipeline {
	case pipeGPU, pipeWGC:
		if vendorOf(s.Encoder) == "intel" {
			a = append(a, "-vf", "hwmap=derive_device=qsv,format=qsv")
		}
	case pipeCUDA:
		// d3d11 frames can't be mapped to cuda, so this still pays for one download
		a = append(a, "-vf", fmt.Sprintf("hwdownload,format=bgra,hwupload_cuda,scale_cuda=%d:%d:interp_algo=bilinear", s.OutW, s.OutH))
	case pipeSystem:
		a = append(a, "-vf", "hwdownload,format=bgra,"+scale+"format="+pixfmt)
	case pipeGDI:
		a = append(a, "-vf", scale+"format="+pixfmt)
	}

	a = append(a, encoderArgs(s.Encoder, s.FPS, s.Kbps)...)
	if len(s.Audio) > 0 {
		a = append(a, "-c:a", "aac", "-b:a", "160k", "-ar", "48000", "-ac", "2")
		for i, in := range s.Audio {
			// mono mic, half the size
			if in.Mono {
				a = append(a, fmt.Sprintf("-ac:a:%d", i), "1", fmt.Sprintf("-b:a:%d", i), "64k")
			}
		}
	}
	return append(a, "-f", "mpegts", "-muxdelay", "0", "-muxpreload", "0", "-flush_packets", "1", "pipe:1")
}

// wgc can hide its yellow border from 20348, past 60 fps needs 26100
func wgcUsable(build, fps int) bool {
	return build >= 26100 || build >= 20348 && fps <= 60
}

// scale_d3d11 is skipped, nvidia drivers reject the encoder textures it asks for
func pipelines(f ffmpeg.Features, enc string, scaling, wgc bool) []pipeline {
	var out []pipeline
	hw := vendorOf(enc) != "cpu"
	if scaling && hw && wgc && f.GfxCapture {
		out = append(out, pipeWGC)
	}
	if f.DDAGrab || !f.Initialised {
		switch {
		case hw && !scaling:
			out = append(out, pipeGPU)
		case scaling && vendorOf(enc) == "nvidia" && f.ScaleCUDA:
			out = append(out, pipeCUDA)
		}
		out = append(out, pipeSystem)
	}
	return append(out, pipeGDI)
}
