package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"snag/internal/config"
	"snag/internal/ffmpeg"
	"snag/internal/jobs"
	"snag/internal/library"
	"snag/internal/mediaserver"
	"snag/internal/recorder"
	"snag/internal/tsbuf"
	"snag/internal/winapi"
)

type Core struct {
	dataDir string
	app     *application.App
	store   *config.Store
	rec     *recorder.Recorder
	lib     *library.Library
	jobs    *jobs.Manager
	media   *mediaserver.Server

	mu         sync.Mutex
	win        *application.WebviewWindow
	tray       *application.SystemTray
	trayToggle *application.MenuItem
	trayLive   bool
	tools      ffmpeg.Tools
	encoders   []ffmpeg.Encoder
	autoFixed  atomic.Bool
	features   ffmpeg.Features
	hotkeyErrs []string
	dl         downloadState
	aiQueue    chan string
	upd        updater
	ctx        context.Context
	cancel     context.CancelFunc
}

type downloadState struct {
	Active bool   `json:"active"`
	Done   int64  `json:"done"`
	Total  int64  `json:"total"`
	Error  string `json:"error"`
}

func newCore(dataDir string) (*Core, error) {
	store, err := config.Open(dataDir)
	if err != nil {
		return nil, err
	}
	c := &Core{dataDir: dataDir, store: store, aiQueue: make(chan string, 256)}
	cleanupOldExe()
	syncInstalledVersion()
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.loadToolCache()

	c.rec = recorder.New(recorder.Deps{
		DataDir:  dataDir,
		Settings: store.Get,
		Tools:    c.Tools,
		Encoders: c.Encoders,
		Features: c.Features,
		OnChange: c.pushStatus,
	})
	c.lib = library.Open(dataDir, c.Tools, func() { c.emit("clips:changed", nil) })
	c.jobs = jobs.New(jobs.Deps{
		Tools:    c.Tools,
		Encoders: c.Encoders,
		OutDir:   func() string { return c.store.Get().ExportsPath() },
		TmpDir:   filepath.Join(dataDir, "tmp"),
		OnChange: func(list []jobs.Job) { c.emit("jobs:changed", list) },
		OnDone:   c.jobDone,
	})
	c.media, err = mediaserver.Start(c.resolveMedia)
	if err != nil {
		return nil, err
	}
	// existing users skip onboarding
	if s := store.Get(); !s.Onboarded && (len(c.lib.List()) > 0 || s.LastVersion != "") {
		s.Onboarded = true
		store.Set(s)
	}
	return c, nil
}

func (c *Core) resolveMedia(kind, id string) (string, bool) {
	switch kind {
	case "clip":
		if cl, ok := c.lib.Get(id); ok {
			return cl.Path, true
		}
	case "thumb":
		return c.lib.ThumbPath(id), true
	case "sprite":
		return c.lib.SpritePath(id), true
	case "job":
		for _, j := range c.jobs.List() {
			if j.ID == id && j.Output != "" {
				return j.Output, true
			}
		}
	}
	return "", false
}

func (c *Core) Tools() ffmpeg.Tools {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tools
}

func (c *Core) Encoders() []ffmpeg.Encoder {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.encoders
}

func (c *Core) Features() ffmpeg.Features {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.features
}

// bump when Features gains a field, old caches would read it as false
const probeSchema = 2

type toolCache struct {
	FFmpeg   string           `json:"ffmpeg"`
	Version  string           `json:"version"`
	Encoders []ffmpeg.Encoder `json:"encoders"`
	Features ffmpeg.Features  `json:"features"`
	Drivers  string           `json:"drivers"`
	Schema   int              `json:"schema"`
}

// new driver, new encoder results
func driverSignature() string {
	var parts []string
	for _, g := range winapi.GPUs() {
		parts = append(parts, g.Name+" "+g.Driver)
	}
	return strings.Join(parts, "; ")
}

func anyHardware(encs []ffmpeg.Encoder) bool {
	for _, e := range encs {
		if e.Available && e.Hardware {
			return true
		}
	}
	return false
}

func (c *Core) cachePath() string { return filepath.Join(c.dataDir, "probe.json") }

func (c *Core) loadToolCache() {
	s := c.store.Get()
	t, err := ffmpeg.Locate(s.FFmpegPath, s.FFmpegInstallDir(c.dataDir))
	if err != nil {
		return
	}
	c.tools = t
	os.Remove(t.FFmpeg + ".old")
	os.Remove(t.FFprobe + ".old")
	var tc toolCache
	if b, err := os.ReadFile(c.cachePath()); err == nil && json.Unmarshal(b, &tc) == nil {
		// cpu-only results get retested, the gpu may have just been busy
		if tc.Schema == probeSchema && tc.FFmpeg == t.FFmpeg && tc.Version == t.Version && tc.Drivers == driverSignature() && anyHardware(tc.Encoders) {
			c.encoders, c.features = tc.Encoders, tc.Features
		}
	}
}

// test encodes are slow, cache per ffmpeg build
func (c *Core) probeTools(force bool) {
	s := c.store.Get()
	t, err := ffmpeg.Locate(s.FFmpegPath, s.FFmpegInstallDir(c.dataDir))
	c.mu.Lock()
	c.tools = t
	have := len(c.encoders) > 0
	c.mu.Unlock()
	if err != nil || (have && !force) {
		c.pushStatus()
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 3*time.Minute)
	defer cancel()
	feats := ffmpeg.DetectFeatures(ctx, t.FFmpeg)
	encs := ffmpeg.ProbeEncoders(ctx, t.FFmpeg)
	c.mu.Lock()
	c.encoders, c.features = encs, feats
	c.mu.Unlock()
	b, _ := json.MarshalIndent(toolCache{FFmpeg: t.FFmpeg, Version: t.Version, Encoders: encs, Features: feats, Drivers: driverSignature(), Schema: probeSchema}, "", " ")
	config.WriteFileAtomic(c.cachePath(), b)
	// only touch ffmpeg we installed ourselves
	if nvencTooOld(encs) && s.FFmpegPath == "" && !c.autoFixed.Swap(true) {
		c.mu.Lock()
		busy := c.dl.Active
		c.mu.Unlock()
		if !busy {
			c.toast("info", "This FFmpeg needs a newer NVIDIA driver than yours, getting an older one that works")
			go c.download()
		}
	}
	if st := c.rec.Status(); st.State == recorder.Buffering && s.Encoder == "auto" && anyHardware(encs) {
		for _, e := range encs {
			if e.Name == st.Encoder && !e.Hardware {
				go c.rec.Restart()
				break
			}
		}
	}
	c.pushStatus()
}

func (c *Core) emit(name string, data any) {
	if c.app == nil {
		return
	}
	c.app.Event.Emit(name, data)
}

func (c *Core) toast(kind, msg string) {
	if kind == "error" {
		slog.Error("toast", "msg", msg)
	}
	c.emit("toast", map[string]string{"kind": kind, "message": msg})
}

// popup is for when you're in a game, in-app toasts cover the rest
func (c *Core) windowFocused() bool {
	c.mu.Lock()
	w := c.win
	c.mu.Unlock()
	return w != nil && w.IsVisible() && !w.IsMinimised() && w.IsFocused()
}

type AppStatus struct {
	Recorder     recorder.Status `json:"recorder"`
	Usage        library.Usage   `json:"usage"`
	FFmpeg       ffmpeg.Tools    `json:"ffmpeg"`
	FFmpegReady  bool            `json:"ffmpegReady"`
	Download     downloadState   `json:"download"`
	HotkeyErrors []string        `json:"hotkeyErrors"`
	ActiveJobs   int             `json:"activeJobs"`
}

func (c *Core) Status() AppStatus {
	s := c.store.Get()
	c.mu.Lock()
	st := AppStatus{FFmpeg: c.tools, FFmpegReady: c.tools.FFmpeg != "", Download: c.dl, HotkeyErrors: c.hotkeyErrs}
	c.mu.Unlock()
	st.Recorder = c.rec.Status()
	st.Usage = c.lib.Usage(s.ClipsDir, s)
	for _, j := range c.jobs.List() {
		if j.Status == "queued" || j.Status == "running" {
			st.ActiveJobs++
		}
	}
	if st.HotkeyErrors == nil {
		st.HotkeyErrors = []string{}
	}
	return st
}

func (c *Core) pushStatus() {
	c.updateTray()
	c.mu.Lock()
	open := c.win != nil
	c.mu.Unlock()
	if open {
		c.emit("status", c.Status())
	}
}

func (c *Core) start() {
	// installer only writes the setting
	if c.store.Get().LaunchOnStartup {
		if on, err := c.app.Autostart.IsEnabled(); err == nil && !on {
			c.app.Autostart.EnableWithOptions(application.AutostartOptions{Arguments: []string{"--tray"}})
		}
	}
	go func() {
		c.probeTools(false)
		s := c.store.Get()
		c.lib.Scan(s.ClipsDir, s.ExportsPath())
		if s.AutoStartBuffer && !s.SmartMode && c.Tools().FFmpeg != "" {
			c.rec.Start()
		}
	}()
	go c.rec.SmartLoop(c.ctx)
	go c.shrinkLoop()
	go c.updateLoop()
	go c.aiWorker()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-t.C:
				c.rec.SampleUsage()
				c.pushStatus()
			}
		}
	}()
}

func (c *Core) shutdown() {
	c.cancel()
	c.rec.Stop()
	time.Sleep(200 * time.Millisecond)
}

func (c *Core) overlay(kind winapi.ToastKind, title, sub string) {
	if s := c.store.Get(); s.ShowOverlay && !c.windowFocused() {
		winapi.ShowToast(kind, title, sub, s.OverlayCorner, winapi.MonitorAt(s.Monitor))
	}
}

func (c *Core) overlayProgress(title, sub string, p float64) {
	if s := c.store.Get(); s.ShowOverlay && !c.windowFocused() {
		winapi.ShowToastProgress(title, sub, p, s.OverlayCorner, winapi.MonitorAt(s.Monitor))
	}
}

func (c *Core) saveFull()  { c.saveClip(c.store.Get().BufferSeconds) }
func (c *Core) saveShort() { c.saveClip(c.store.Get().ShortSeconds) }

func (c *Core) saveClip(seconds int) {
	s := c.store.Get()
	res, err := c.rec.Save(seconds)
	if err != nil {
		playCue(s, "error")
		msg := err.Error()
		if errors.Is(err, tsbuf.ErrEmpty) {
			msg = "Nothing captured yet. Start capturing first"
		}
		c.toast("error", msg)
		c.overlay(winapi.ToastError, "Clip not saved", msg)
		return
	}
	playCue(s, "save")
	var clip library.Clip
	if prev, ok := c.lib.FindByPath(res.Replaced); res.Replaced != "" && ok {
		clip, _ = c.lib.ReplaceFile(prev.ID, res.Path)
		clip, _ = c.lib.Edit(prev.ID, func(cl *library.Clip) { cl.Markers, cl.Tracks, cl.CreatedAt = res.Marks, res.Tracks, time.Now() })
	} else {
		clip = c.lib.Add(library.NewClip{Path: res.Path, Game: res.Game, Markers: res.Marks, Tracks: res.Tracks})
	}
	c.queueAI(clip.ID)
	if s.CatboxAutoUpload {
		c.overlayProgress("Clip saved", "Uploading", 0)
		if _, err := c.uploadClip(clip.ID, "Clip saved"); err != nil {
			c.overlay(winapi.ToastError, "Clip saved, not uploaded", err.Error())
		}
	} else {
		c.overlay(winapi.ToastSuccess, "Clip saved", fmt.Sprintf("%s, %.0f seconds", res.Game, clip.Duration))
	}
	c.emit("clip:saved", clip)
	if s.StorageBudgetEnabled {
		if n := c.lib.Prune(int64(s.StorageBudgetGB)<<30, s.KeepFolderClips); n > 0 {
			c.toast("info", fmt.Sprintf("Storage budget reached, removed %d old clip%s", n, plural(n)))
		}
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (c *Core) addMark() {
	if c.rec.AddMark() {
		playCue(c.store.Get(), "mark")
		c.toast("info", "Bookmark added")
		c.overlay(winapi.ToastInfo, "Bookmark added", "Marked on your next clip")
	}
}

func (c *Core) registerHotkeys() {
	s := c.store.Get()
	gs := c.app.GlobalShortcut
	gs.UnregisterAll()
	var errs []string
	bind := func(label, accel string, fn func()) {
		if strings.TrimSpace(accel) == "" {
			return
		}
		if err := gs.Register(accel, func() { go fn() }); err != nil {
			errs = append(errs, fmt.Sprintf("%s (%s): %v", label, accel, err))
		}
	}
	bind("Save clip", s.Hotkeys.SaveClip, c.saveFull)
	bind("Save short clip", s.Hotkeys.SaveShort, c.saveShort)
	bind("Bookmark", s.Hotkeys.Bookmark, c.addMark)
	bind("Screenshot", s.Hotkeys.Screenshot, c.screenshot)
	bind("Start or stop capturing", s.Hotkeys.ToggleBuffer, c.rec.Toggle)
	c.mu.Lock()
	c.hotkeyErrs = errs
	c.mu.Unlock()
	if len(errs) > 0 {
		slog.Warn("hotkeys", "errors", errs)
	}
}

func (c *Core) applySettings(prev, next config.Settings) {
	if prev.Hotkeys != next.Hotkeys {
		c.registerHotkeys()
	}
	if prev.FFmpegDir != next.FFmpegDir {
		c.moveFFmpeg(prev.FFmpegInstallDir(c.dataDir), next.FFmpegInstallDir(c.dataDir))
	} else if prev.FFmpegPath != next.FFmpegPath {
		c.probeTools(true)
	}
	if prev.BufferSeconds != next.BufferSeconds {
		c.rec.SetBufferSeconds(next.BufferSeconds)
	}
	if prev.CaptureKey() != next.CaptureKey() {
		go c.rec.Restart()
	}
	if prev.LaunchOnStartup != next.LaunchOnStartup {
		if next.LaunchOnStartup {
			c.app.Autostart.EnableWithOptions(application.AutostartOptions{Arguments: []string{"--tray"}})
		} else {
			c.app.Autostart.Disable()
		}
	}
	if prev.ClipsDir != next.ClipsDir || prev.ExportsPath() != next.ExportsPath() {
		go c.lib.Scan(next.ClipsDir, next.ExportsPath())
	}
	if next.StorageBudgetEnabled {
		go c.lib.Prune(int64(next.StorageBudgetGB)<<30, next.KeepFolderClips)
	}
	if prev.SmartMode && !next.SmartMode && next.AutoStartBuffer {
		c.rec.Start()
	}
	c.pushStatus()
}

// ffmpeg built against newer nvenc headers than the driver supports
func nvencTooOld(encs []ffmpeg.Encoder) bool {
	old := false
	for _, e := range encs {
		if e.Vendor != "nvidia" {
			continue
		}
		if e.Available {
			return false
		}
		if strings.HasPrefix(e.Error, "NVIDIA driver too old") {
			old = true
		}
	}
	return old
}

func ceilingFor(t ffmpeg.Tools, encs []ffmpeg.Encoder) int {
	if !nvencTooOld(encs) {
		return 0
	}
	if v := ffmpeg.StableVersion(t.Version); v > 0 {
		return v
	}
	return 1 << 20
}

func (c *Core) download() error {
	c.mu.Lock()
	if c.dl.Active {
		c.mu.Unlock()
		return errors.New("download already running")
	}
	c.dl = downloadState{Active: true}
	below := ceilingFor(c.tools, c.encoders)
	c.mu.Unlock()
	c.pushStatus()

	nvidia := false
	for _, g := range winapi.GPUs() {
		nvidia = nvidia || g.Vendor == "nvidia"
	}
	_, err := ffmpeg.DownloadFitting(c.ctx, c.store.Get().FFmpegInstallDir(c.dataDir), below, nvidia, func(done, total int64) {
		c.mu.Lock()
		c.dl.Done, c.dl.Total = done, total
		c.mu.Unlock()
		c.emit("ffmpeg:progress", map[string]int64{"done": done, "total": total})
	})
	if err == nil {
		c.probeTools(true)
	}
	c.mu.Lock()
	c.dl.Active = false
	if err != nil {
		c.dl.Error = err.Error()
	}
	c.mu.Unlock()
	if err != nil {
		c.pushStatus()
		return err
	}
	if nvencTooOld(c.Encoders()) {
		c.toast("error", "No FFmpeg build works with this NVIDIA driver yet, capture stays on the CPU until you update it")
	}
	s := c.store.Get()
	c.lib.Scan(s.ClipsDir, s.ExportsPath())
	if s.AutoStartBuffer && !s.SmartMode && c.rec.Status().State == recorder.Idle {
		c.rec.Start()
	}
	return nil
}

func (c *Core) openWindow() {
	c.mu.Lock()
	if c.win != nil {
		w := c.win
		c.mu.Unlock()
		w.UnMinimise()
		w.Show()
		w.Focus()
		return
	}
	w := c.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "Snag",
		Width:            1280,
		Height:           800,
		MinWidth:         980,
		MinHeight:        620,
		Frameless:        true,
		InitialPosition:  application.WindowCentered,
		BackgroundColour: application.NewRGB(0x13, 0x14, 0x16),
		URL:              "/",
		Windows: application.WindowsWindow{
			Theme: application.Dark,
		},
		DefaultContextMenuDisabled: true,
	})
	c.win = w
	c.mu.Unlock()

	// destroying the webview on close frees most of the ram
	w.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent) {
		c.mu.Lock()
		if c.win == w {
			c.win = nil
		}
		c.mu.Unlock()
		if !c.store.Get().CloseToTray {
			go c.app.Quit()
		}
	})
}

func (c *Core) setupTray() {
	tray := c.app.SystemTray.New()
	tray.SetIcon(trayIdlePNG)
	tray.SetTooltip("Snag")
	menu := c.app.NewMenu()
	menu.Add("Open Snag").OnClick(func(*application.Context) { c.openWindow() })
	menu.Add("Save clip").OnClick(func(*application.Context) { go c.saveFull() })
	menu.AddSeparator()
	c.trayToggle = menu.Add("Start capturing").OnClick(func(*application.Context) { go c.rec.Toggle() })
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) { c.app.Quit() })
	tray.SetMenu(menu)
	tray.OnClick(func() { c.openWindow() })
	c.tray = tray
}

func (c *Core) updateTray() {
	c.mu.Lock()
	tray, item := c.tray, c.trayToggle
	c.mu.Unlock()
	if tray == nil {
		return
	}
	st := c.rec.Status()
	live := st.State == recorder.Buffering || st.State == recorder.Starting
	c.mu.Lock()
	changed := live != c.trayLive
	c.trayLive = live
	c.mu.Unlock()
	if !changed {
		return
	}
	if live {
		tray.SetIcon(trayLivePNG)
		tray.SetTooltip("Snag: capturing")
		item.SetLabel("Stop capturing")
	} else {
		tray.SetIcon(trayIdlePNG)
		tray.SetTooltip("Snag: paused")
		item.SetLabel("Start capturing")
	}
}

// can't move ffmpeg.exe while capture has it open
func (c *Core) moveFFmpeg(from, to string) {
	st := c.rec.Status()
	running := st.State == recorder.Buffering || st.State == recorder.Starting
	if running {
		c.rec.Stop()
		time.Sleep(800 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(from, "ffmpeg.exe")); err == nil {
		c.toast("info", "Moving FFmpeg to the new folder")
		if err := ffmpeg.Move(from, to); err != nil {
			c.toast("error", "Could not move FFmpeg: "+err.Error())
		} else {
			c.toast("success", "FFmpeg moved")
		}
	}
	c.probeTools(true)
	if running {
		c.rec.Start()
	}
}
