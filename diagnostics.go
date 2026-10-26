package main

import (
	"fmt"
	"runtime"
	"strings"

	"snag/internal/winapi"
)

// never include keys here, this gets pasted into public issues
func (c *Core) diagnostics() string {
	s := c.store.Get()
	st := c.Status()
	var b strings.Builder
	line := func(k string, v any) { fmt.Fprintf(&b, "%-14s %v\n", k+":", v) }
	b.WriteString("Snag diagnostics\n\n")
	line("snag", appVersion)
	line("windows", winapi.WindowsVersion())
	line("go", runtime.Version())
	line("ffmpeg", orNone(st.FFmpeg.Version))
	for _, g := range winapi.GPUs() {
		line("gpu", g.Name+", driver "+g.Driver)
	}
	for _, m := range winapi.Monitors() {
		line("display", fmt.Sprintf("%dx%d%s", m.Width, m.Height, map[bool]string{true: " (main)"}[m.Primary]))
	}
	b.WriteString("\nencoders\n")
	for _, e := range c.Encoders() {
		state := "ok"
		if !e.Available {
			state = orNone(e.Error)
		}
		fmt.Fprintf(&b, "  %-12s %s\n", e.Name, state)
	}
	r := st.Recorder
	b.WriteString("\ncapture\n")
	line("  state", r.State)
	line("  pipeline", orNone(r.Pipeline))
	line("  encoder", orNone(r.Encoder))
	line("  output", fmt.Sprintf("%dx%d at %d fps, %d kbps", r.Width, r.Height, r.FPS, r.Kbps))
	line("  tracks", strings.Join(r.Tracks, ", "))
	if r.Error != "" {
		line("  error", r.Error)
	}
	if r.Warning != "" {
		line("  warning", r.Warning)
	}
	b.WriteString("\nsettings\n")
	line("  video", fmt.Sprintf("%s %s, %s, %d fps, %s quality", s.Encoder, s.Codec, s.Resolution, s.FPS, s.Quality))
	line("  clips", fmt.Sprintf("%ds buffer, on disk %v, merge %v", s.BufferSeconds, s.BufferOnDisk, s.MergeOverlaps))
	line("  audio", fmt.Sprintf("system %v, mic %v, discord split %v", s.CaptureSystemAudio, s.CaptureMic, s.SeparateVoice))
	line("  smart mode", s.SmartMode)
	line("  ai", orNone(s.AIProvider))
	if len(st.HotkeyErrors) > 0 {
		line("  hotkeys", strings.Join(st.HotkeyErrors, "; "))
	}
	b.WriteString("\nrecent log\n")
	for _, l := range logTail.tail(40) {
		b.WriteString("  " + l + "\n")
	}
	return b.String()
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
