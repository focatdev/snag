package recorder

import (
	"slices"
	"strings"
	"testing"

	"snag/internal/ffmpeg"
)

func TestPipelines(t *testing.T) {
	all := ffmpeg.Features{Initialised: true, DDAGrab: true, ScaleCUDA: true, GfxCapture: true}
	cases := []struct {
		name    string
		f       ffmpeg.Features
		enc     string
		scaling bool
		wgc     bool
		want    []pipeline
	}{
		{"native nvenc", all, "h264_nvenc", false, true, []pipeline{pipeGPU, pipeSystem, pipeGDI}},
		{"scaled nvenc win11", all, "h264_nvenc", true, true, []pipeline{pipeWGC, pipeCUDA, pipeSystem, pipeGDI}},
		{"scaled nvenc win10", all, "h264_nvenc", true, false, []pipeline{pipeCUDA, pipeSystem, pipeGDI}},
		{"scaled amf win11", all, "h264_amf", true, true, []pipeline{pipeWGC, pipeSystem, pipeGDI}},
		{"scaled amf win10", all, "h264_amf", true, false, []pipeline{pipeSystem, pipeGDI}},
		{"scaled x264", all, "libx264", true, true, []pipeline{pipeSystem, pipeGDI}},
		{"old ffmpeg", ffmpeg.Features{Initialised: true, DDAGrab: true}, "h264_nvenc", true, true, []pipeline{pipeSystem, pipeGDI}},
		{"not probed yet", ffmpeg.Features{}, "h264_nvenc", true, true, []pipeline{pipeSystem, pipeGDI}},
		{"no ddagrab", ffmpeg.Features{Initialised: true, GfxCapture: true}, "h264_nvenc", true, true, []pipeline{pipeWGC, pipeGDI}},
	}
	for _, c := range cases {
		if got := pipelines(c.f, c.enc, c.scaling, c.wgc); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWGCUsable(t *testing.T) {
	cases := []struct {
		build, fps int
		want       bool
	}{
		{19045, 60, false},
		{22631, 60, true},
		{22631, 144, false},
		{26100, 144, true},
	}
	for _, c := range cases {
		if got := wgcUsable(c.build, c.fps); got != c.want {
			t.Errorf("wgcUsable(%d, %d) = %v, want %v", c.build, c.fps, got, c.want)
		}
	}
}

func TestBuildArgsScaling(t *testing.T) {
	base := captureSpec{Encoder: "h264_nvenc", MonHandle: 65537, SrcW: 1920, SrcH: 1080, OutW: 1280, OutH: 720, FPS: 60, Kbps: 7000, Cursor: true}
	cases := []struct {
		p      pipeline
		enc    string
		input  string
		filter string
	}{
		{pipeWGC, "h264_nvenc", "gfxcapture=hmonitor=65537:capture_cursor=1:max_framerate=60:width=1280:height=720:resize_mode=scale", ""},
		{pipeWGC, "h264_qsv", "gfxcapture=", "hwmap=derive_device=qsv,format=qsv"},
		{pipeCUDA, "h264_nvenc", "ddagrab=", "hwdownload,format=bgra,hwupload_cuda,scale_cuda=1280:720:interp_algo=bilinear"},
		{pipeSystem, "h264_nvenc", "ddagrab=", "hwdownload,format=bgra,scale=1280:720:flags=bilinear,format=nv12"},
	}
	for _, c := range cases {
		s := base
		s.Pipeline, s.Encoder = c.p, c.enc
		a := buildArgs(s)
		if got := argAfter(a, "-i"); !strings.HasPrefix(got, c.input) {
			t.Errorf("%s %s: input %q, want prefix %q", c.p, c.enc, got, c.input)
		}
		if got := argAfter(a, "-vf"); got != c.filter {
			t.Errorf("%s %s: filter %q, want %q", c.p, c.enc, got, c.filter)
		}
	}
}

func argAfter(a []string, flag string) string {
	if i := slices.Index(a, flag); i >= 0 && i+1 < len(a) {
		return a[i+1]
	}
	return ""
}
