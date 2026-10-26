package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"snag/internal/ai"
	"snag/internal/config"
	"snag/internal/ffmpeg"
	"snag/internal/jobs"
	"snag/internal/library"
	"snag/internal/wasapi"
	"snag/internal/winapi"
)

type SnagService struct {
	c *Core
}

func (s *SnagService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.c.start()
	return nil
}

type MediaInfo struct {
	Base  string `json:"base"`
	Token string `json:"token"`
}

type Bootstrap struct {
	Version  string           `json:"version"`
	DataDir  string           `json:"dataDir"`
	Settings config.Settings  `json:"settings"`
	Status   AppStatus        `json:"status"`
	Media    MediaInfo        `json:"media"`
	Monitors []winapi.Monitor `json:"monitors"`
	Encoders []ffmpeg.Encoder `json:"encoders"`
	GPUs     []winapi.GPU     `json:"gpus"`
	Update   UpdateState      `json:"update"`
	WhatsNew *WhatsNew        `json:"whatsNew"`
	Features ffmpeg.Features  `json:"features"`
	Clips    []library.Clip   `json:"clips"`
	Jobs     []jobs.Job       `json:"jobs"`
}

func (s *SnagService) GetBootstrap() Bootstrap {
	c := s.c
	return Bootstrap{
		Version:  appVersion,
		DataDir:  c.dataDir,
		Settings: masked(c.store.Get()),
		Status:   c.Status(),
		Media:    MediaInfo{Base: c.media.Base, Token: c.media.Token()},
		Monitors: winapi.Monitors(),
		Encoders: c.Encoders(),
		GPUs:     winapi.GPUs(),
		Update:   c.updateState(),
		WhatsNew: c.whatsNew(),
		Features: c.Features(),
		Clips:    c.lib.List(),
		Jobs:     c.jobs.List(),
	}
}

func (s *SnagService) GetStatus() AppStatus { return s.c.Status() }

func (s *SnagService) GetSettings() config.Settings { return masked(s.c.store.Get()) }

// the key never goes to the ui
func masked(cfg config.Settings) config.Settings {
	if cfg.AIKey != "" {
		cfg.AIKey = "saved"
	}
	return cfg
}

func (s *SnagService) SaveSettings(next config.Settings) (config.Settings, error) {
	prev := s.c.store.Get()
	next.AIKey = prev.AIKey
	// don't send a groq key to gemini
	if next.AIProvider != prev.AIProvider {
		next.AIKey = ""
	}
	saved, err := s.c.store.Set(next)
	if err != nil {
		return masked(prev), err
	}
	go s.c.applySettings(prev, saved)
	return masked(saved), nil
}

type Estimate struct {
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Kbps     int     `json:"kbps"`
	MBPerMin float64 `json:"mbPerMin"`
	BufferMB float64 `json:"bufferMB"`
}

func (s *SnagService) EstimateBitrate(cfg config.Settings) Estimate {
	cfg.Normalize()
	m := winapi.MonitorAt(cfg.Monitor)
	w, h := m.Width, m.Height
	ow, oh := cfg.OutputSize(w, h)
	kbps := cfg.BitrateKbps(ow, oh)
	total := float64(kbps)
	if cfg.CaptureSystemAudio {
		total += 160
	}
	if cfg.CaptureMic {
		total += 160
	}
	mbMin := total * 60 / 8 / 1024
	return Estimate{Width: ow, Height: oh, Kbps: kbps, MBPerMin: mbMin, BufferMB: mbMin * float64(cfg.BufferSeconds) / 60}
}

func (s *SnagService) StartBuffer()   { s.c.rec.Start() }
func (s *SnagService) StopBuffer()    { s.c.rec.Stop() }
func (s *SnagService) SaveClip()      { go s.c.saveFull() }
func (s *SnagService) SaveShortClip() { go s.c.saveShort() }
func (s *SnagService) AddBookmark()   { s.c.addMark() }

func (s *SnagService) DownloadFFmpeg() error { return s.c.download() }

func (s *SnagService) ReprobeEncoders() []ffmpeg.Encoder {
	s.c.probeTools(true)
	return s.c.Encoders()
}

type AudioDevices struct {
	Outputs []wasapi.Device `json:"outputs"`
	Inputs  []wasapi.Device `json:"inputs"`
}

func (s *SnagService) ListAudioDevices() AudioDevices {
	out, _ := wasapi.List(false)
	in, _ := wasapi.List(true)
	if out == nil {
		out = []wasapi.Device{}
	}
	if in == nil {
		in = []wasapi.Device{}
	}
	return AudioDevices{Outputs: out, Inputs: in}
}

func (s *SnagService) ListMonitors() []winapi.Monitor { return winapi.Monitors() }

func (s *SnagService) ListClips() []library.Clip { return s.c.lib.List() }

func (s *SnagService) UpdateClip(id string, p library.Patch) (library.Clip, error) {
	return s.c.lib.Update(id, p)
}

func (s *SnagService) DeleteClips(ids []string) error { return s.c.lib.Delete(ids) }

func (s *SnagService) Rescan() {
	cfg := s.c.store.Get()
	s.c.lib.Scan(cfg.ClipsDir, cfg.ExportsPath())
}

func (s *SnagService) CopyClip(id string) error {
	c, ok := s.c.lib.Get(id)
	if !ok {
		return errors.New("clip not found")
	}
	return winapi.CopyFiles(c.Path)
}

func (s *SnagService) CopyPath(path string) error { return winapi.CopyFiles(path) }

func (s *SnagService) PreviewFileName(cfg config.Settings) string {
	cfg.Normalize()
	full := cfg.ClipPath("Roblox", cfg.BufferSeconds, time.Now()) + ".mp4"
	if rel, err := filepath.Rel(cfg.ClipsDir, full); err == nil {
		return rel
	}
	return filepath.Base(full)
}

func (s *SnagService) PreviewSound(cfg config.Settings) {
	cfg.Normalize()
	cfg.SaveSound = true
	playCue(cfg, "save")
}

func (s *SnagService) PickSoundFile() (string, error) {
	return s.c.app.Dialog.OpenFile().CanChooseFiles(true).AddFilter("WAV audio", "*.wav").SetTitle("Choose a save sound").PromptForSingleSelection()
}

// bypass the focus check, snag is focused when you click test
func (s *SnagService) TestOverlay() {
	cfg := s.c.store.Get()
	winapi.ShowToast(winapi.ToastSuccess, "Clip saved", "Roblox, 60 seconds", cfg.OverlayCorner, winapi.MonitorAt(cfg.Monitor))
}

func (s *SnagService) GetWaveform(id string, track int, buckets int) ([]float32, error) {
	return s.c.lib.Waveform(id, track, buckets)
}

func (s *SnagService) RevealClip(id string) error {
	c, ok := s.c.lib.Get(id)
	if !ok {
		return errors.New("clip not found")
	}
	return s.c.app.Env.OpenFileManager(c.Path, true)
}

func (s *SnagService) RevealPath(path string) error { return s.c.app.Env.OpenFileManager(path, true) }

func (s *SnagService) OpenClipsFolder() error {
	dir := s.c.store.Get().ClipsDir
	os.MkdirAll(dir, 0o755)
	return s.c.app.Env.OpenFileManager(dir, false)
}

func (s *SnagService) OpenPath(path string) error { return s.c.app.Env.OpenFileManager(path, false) }

func (s *SnagService) Export(o jobs.ExportOptions) (jobs.Job, error) {
	c, ok := s.c.lib.Get(o.ClipID)
	if !ok {
		return jobs.Job{}, errors.New("clip not found")
	}
	src := jobs.Source{ID: c.ID, Path: c.Path, Title: c.Title, Size: c.Size, Duration: c.Duration, Width: c.Width, Height: c.Height, FPS: c.FPS, Tracks: len(c.Tracks)}
	return s.c.jobs.Submit(src, o)
}

func (s *SnagService) ListJobs() []jobs.Job { return s.c.jobs.List() }
func (s *SnagService) CancelJob(id string)  { s.c.jobs.Cancel(id) }
func (s *SnagService) ClearJobs()           { s.c.jobs.ClearFinished() }

func (s *SnagService) PickFolder(start string) (string, error) {
	d := s.c.app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).SetTitle("Choose folder")
	if start != "" {
		d = d.SetDirectory(start)
	}
	return d.PromptForSingleSelection()
}

func (s *SnagService) PickFFmpeg() (string, error) {
	return s.c.app.Dialog.OpenFile().CanChooseFiles(true).AddFilter("ffmpeg.exe", "ffmpeg.exe").SetTitle("Locate ffmpeg.exe").PromptForSingleSelection()
}

func (s *SnagService) Quit() { s.c.app.Quit() }

func (s *SnagService) ShrinkClips(ids []string) int {
	n := 0
	for _, id := range ids {
		if src, ok := s.c.shrinkSource(id); ok {
			s.c.jobs.SubmitShrink(src)
			n++
		}
	}
	return n
}

func (s *SnagService) MergeClips(ids []string, name string) (jobs.Job, error) {
	var srcs []jobs.Source
	for _, id := range ids {
		if src, ok := s.c.shrinkSource(id); ok {
			srcs = append(srcs, src)
		}
	}
	return s.c.jobs.SubmitMerge(srcs, name)
}

func (s *SnagService) TakeScreenshot() { go s.c.screenshot() }

func (s *SnagService) DescribeClip(id string) (library.Clip, error) { return s.c.describeClip(id) }

func (s *SnagService) TranscribeClip(id string) (library.Clip, error) { return s.c.transcribeClip(id) }

func (s *SnagService) AIProviders() []ai.Provider { return ai.Providers }

func (s *SnagService) SetAIKey(key string) error {
	enc, err := winapi.Protect(strings.TrimSpace(key))
	if err != nil {
		return err
	}
	cfg := s.c.store.Get()
	cfg.AIKey = enc
	_, err = s.c.store.Set(cfg)
	return err
}

func (s *SnagService) UploadClip(id string) (jobs.Job, error) { return s.c.uploadClip(id, "") }

func (s *SnagService) CopyText(text string) error { return winapi.CopyText(text) }

func (s *SnagService) RepairClip(id string) (library.Clip, error) { return s.c.repairClip(id) }

func (s *SnagService) ListUploaders() []UploaderInfo { return s.c.listUploaders() }

func (s *SnagService) ImportUploader() (*UploaderInfo, error) {
	p, err := s.c.app.Dialog.OpenFile().CanChooseFiles(true).AddFilter("ShareX custom uploader", "*.sxcu;*.json").SetTitle("Import a custom uploader").PromptForSingleSelection()
	if err != nil || p == "" {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	u, err := s.c.importUploader(b)
	return &u, err
}

func (s *SnagService) ImportUploaderText(text string) (UploaderInfo, error) {
	return s.c.importUploader([]byte(text))
}

func (s *SnagService) DeleteUploader(id string) error { return s.c.deleteUploader(id) }

func (s *SnagService) CheckUpdate() (UpdateState, error) {
	err := s.c.checkUpdate(true)
	return s.c.updateState(), err
}

func (s *SnagService) RestartToUpdate() error { return s.c.restartForUpdate() }

func (s *SnagService) Diagnostics() string { return s.c.diagnostics() }

func (s *SnagService) OpenLogFolder() error {
	return s.c.app.Env.OpenFileManager(logPath(s.c.dataDir), true)
}

func (s *SnagService) GetEncoders() []ffmpeg.Encoder { return s.c.Encoders() }
