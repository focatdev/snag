package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"snag/internal/config"
	"snag/internal/ffmpeg"
	"snag/internal/winapi"
)

// set by build.bat
var version = "0.1.0"

const (
	uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Snag`
	runKey       = `Software\Microsoft\Windows\CurrentVersion\Run`
	shellFolders = `Software\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders`
)

type SetupService struct {
	app       *application.App
	uninstall bool
	mu        sync.Mutex
	busy      bool
	installed string
}

type Paths struct {
	App    string `json:"app"`
	Clips  string `json:"clips"`
	FFmpeg string `json:"ffmpeg"`
}

type Info struct {
	Version      string         `json:"version"`
	Installed    string         `json:"installed"`
	Autostart    bool           `json:"autostart"`
	Uninstall    bool           `json:"uninstall"`
	Update       bool           `json:"update"`
	Drives       []winapi.Drive `json:"drives"`
	Home         string         `json:"home"`
	LocalAppData string         `json:"localAppData"`
	Existing     Paths          `json:"existing"`
	PayloadBytes int64          `json:"payloadBytes"`
	ClipCount    int            `json:"clipCount"`
	ClipBytes    int64          `json:"clipBytes"`
}

func localAppData() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	d, _ := os.UserCacheDir()
	return d
}

func dataDir() string { return filepath.Join(localAppData(), "Snag") }

func defaultFFmpegDir() string { return filepath.Join(dataDir(), "bin") }

type trackedClip struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func trackedClips() []trackedClip {
	var list []trackedClip
	if b, err := os.ReadFile(filepath.Join(dataDir(), "library.json")); err == nil {
		json.Unmarshal(b, &list)
	}
	return list
}

func (s *SetupService) Info() Info {
	home, _ := os.UserHomeDir()
	info := Info{Version: version, Uninstall: s.uninstall, Drives: winapi.Drives(), Home: home, LocalAppData: localAppData(), PayloadBytes: int64(len(payload))}
	if loc, err := winapi.RegReadString(uninstallKey, "InstallLocation"); err == nil && loc != "" {
		info.Update = true
		info.Existing.App = loc
		info.Installed, _ = winapi.RegReadString(uninstallKey, "DisplayVersion")
	}
	if store, err := config.Open(dataDir()); err == nil {
		cfg := store.Get()
		if _, err := os.Stat(filepath.Join(dataDir(), "settings.json")); err == nil {
			info.Existing.Clips = cfg.ClipsDir
			info.Autostart = cfg.LaunchOnStartup
			info.Existing.FFmpeg = cfg.FFmpegInstallDir(dataDir())
		}
	}
	for _, c := range trackedClips() {
		info.ClipCount++
		info.ClipBytes += c.Size
	}
	if info.Drives == nil {
		info.Drives = []winapi.Drive{}
	}
	return info
}

func (s *SetupService) PickFolder(start string) (string, error) {
	d := s.app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).SetTitle("Choose folder")
	if start != "" {
		if _, err := os.Stat(start); err == nil {
			d = d.SetDirectory(start)
		}
	}
	return d.PromptForSingleSelection()
}

type Options struct {
	Paths            Paths `json:"paths"`
	DownloadFFmpeg   bool  `json:"downloadFFmpeg"`
	StartWithWindows bool  `json:"startWithWindows"`
	DesktopShortcut  bool  `json:"desktopShortcut"`
	StartMenu        bool  `json:"startMenu"`
}

type Progress struct {
	Step  string  `json:"step"`
	Pct   float64 `json:"pct"`
	Note  string  `json:"note"`
	Done  bool    `json:"done"`
	Error string  `json:"error"`
}

func (s *SetupService) emit(p Progress) { s.app.Event.Emit("setup:progress", p) }

func desktopDir() string {
	if d, err := winapi.RegReadString(shellFolders, "Desktop"); err == nil && d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Desktop")
}

func startMenuLink() string {
	return filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Snag.lnk")
}

func copySelf(dst string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Clean(self), filepath.Clean(dst)) {
		return nil
	}
	b, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o755)
}

func (s *SetupService) Install(o Options) error {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return errors.New("already running")
	}
	s.busy = true
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			s.busy = false
			s.mu.Unlock()
		}()
		if err := s.install(o); err != nil {
			s.emit(Progress{Step: "error", Error: err.Error()})
			return
		}
		s.emit(Progress{Step: "done", Pct: 1, Done: true})
	}()
	return nil
}

func (s *SetupService) install(o Options) error {
	p := o.Paths
	if p.App == "" || p.Clips == "" || p.FFmpeg == "" {
		return errors.New("pick a folder for everything first")
	}

	s.emit(Progress{Step: "files", Pct: 0.05, Note: "Closing Snag if it is open"})
	winapi.CloseProcess("Snag.exe")

	s.emit(Progress{Step: "files", Pct: 0.12, Note: "Copying Snag"})
	if err := os.MkdirAll(p.App, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", p.App, err)
	}
	exe := filepath.Join(p.App, "Snag.exe")
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, payload, 0o755); err != nil {
		return fmt.Errorf("could not write Snag.exe: %w", err)
	}
	// running exes can be renamed but not overwritten
	if _, err := os.Stat(exe); err == nil {
		os.Remove(exe + ".old")
		os.Rename(exe, exe+".old")
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("Snag.exe is still in use, close it and try again: %w", err)
	}
	uninstaller := filepath.Join(p.App, "Uninstall Snag.exe")
	if err := copySelf(uninstaller); err != nil {
		return fmt.Errorf("could not copy the uninstaller: %w", err)
	}

	s.emit(Progress{Step: "settings", Pct: 0.3, Note: "Saving your folders"})
	if err := os.MkdirAll(p.Clips, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", p.Clips, err)
	}
	store, err := config.Open(dataDir())
	if err != nil {
		return err
	}
	cfg := store.Get()
	cfg.ClipsDir = p.Clips
	cfg.FFmpegDir = ""
	if !strings.EqualFold(filepath.Clean(p.FFmpeg), filepath.Clean(defaultFFmpegDir())) {
		cfg.FFmpegDir = p.FFmpeg
	}
	cfg.LaunchOnStartup = o.StartWithWindows
	if _, err := store.Set(cfg); err != nil {
		return err
	}
	if o.StartWithWindows {
		winapi.RegWrite(runKey, winapi.RegValue{Name: "snag", Str: `"` + exe + `" --tray`})
	} else {
		winapi.RegDeleteValue(runKey, "snag")
	}

	s.emit(Progress{Step: "shortcuts", Pct: 0.4, Note: "Adding shortcuts"})
	if o.StartMenu {
		if err := winapi.CreateShortcut(startMenuLink(), exe, p.App, ""); err != nil {
			s.emit(Progress{Step: "shortcuts", Pct: 0.42, Note: "Start menu shortcut skipped"})
		}
	}
	if o.DesktopShortcut {
		winapi.CreateShortcut(filepath.Join(desktopDir(), "Snag.lnk"), exe, p.App, "")
	}

	if o.DownloadFFmpeg {
		if _, err := ffmpeg.Locate("", p.FFmpeg); err != nil {
			s.emit(Progress{Step: "ffmpeg", Pct: 0.45, Note: "Finding the latest FFmpeg"})
			nvidia := false
			for _, g := range winapi.GPUs() {
				nvidia = nvidia || g.Vendor == "nvidia"
			}
			_, err := ffmpeg.DownloadFitting(context.Background(), p.FFmpeg, 0, nvidia, func(done, total int64) {
				if total > 0 {
					s.emit(Progress{Step: "ffmpeg", Pct: 0.45 + 0.5*float64(done)/float64(total), Note: fmt.Sprintf("Downloading FFmpeg, %d of %d MB", done>>20, total>>20)})
				}
			})
			if err != nil {
				// snag offers the download again later
				s.emit(Progress{Step: "ffmpeg", Pct: 0.95, Note: "FFmpeg download failed, Snag will offer it again"})
			}
		}
	}

	s.emit(Progress{Step: "register", Pct: 0.97, Note: "Registering with Windows"})
	err = winapi.RegWrite(uninstallKey,
		winapi.RegValue{Name: "DisplayName", Str: "Snag"},
		winapi.RegValue{Name: "DisplayVersion", Str: version},
		winapi.RegValue{Name: "Publisher", Str: "Snag"},
		winapi.RegValue{Name: "DisplayIcon", Str: exe},
		winapi.RegValue{Name: "InstallLocation", Str: p.App},
		winapi.RegValue{Name: "UninstallString", Str: `"` + uninstaller + `" --uninstall`},
		winapi.RegValue{Name: "NoModify", Num: 1, IsNum: true},
		winapi.RegValue{Name: "NoRepair", Num: 1, IsNum: true},
		winapi.RegValue{Name: "EstimatedSize", Num: uint32(len(payload)/1024) * 2, IsNum: true},
	)
	if err != nil {
		return fmt.Errorf("could not register the uninstaller: %w", err)
	}
	s.mu.Lock()
	s.installed = exe
	s.mu.Unlock()
	return nil
}

func (s *SetupService) Launch() error {
	s.mu.Lock()
	exe := s.installed
	s.mu.Unlock()
	if exe == "" {
		return errors.New("Snag is not installed yet")
	}
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return err
	}
	s.app.Quit()
	return nil
}

func (s *SetupService) Quit() { s.app.Quit() }

type UninstallOptions struct {
	RemoveClips bool `json:"removeClips"`
	RemoveData  bool `json:"removeData"`
}

func (s *SetupService) Uninstall(o UninstallOptions) error {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return errors.New("already running")
	}
	s.busy = true
	s.mu.Unlock()
	go func() {
		s.uninstallNow(o)
		s.emit(Progress{Step: "done", Pct: 1, Done: true})
	}()
	return nil
}

func (s *SetupService) uninstallNow(o UninstallOptions) {
	s.emit(Progress{Step: "files", Pct: 0.1, Note: "Closing Snag"})
	winapi.CloseProcess("Snag.exe")

	appDir, _ := winapi.RegReadString(uninstallKey, "InstallLocation")
	if appDir == "" {
		if self, err := os.Executable(); err == nil {
			appDir = filepath.Dir(self)
		}
	}
	store, _ := config.Open(dataDir())
	var cfg config.Settings
	if store != nil {
		cfg = store.Get()
	}

	s.emit(Progress{Step: "shortcuts", Pct: 0.3, Note: "Removing shortcuts"})
	os.Remove(startMenuLink())
	os.Remove(filepath.Join(desktopDir(), "Snag.lnk"))
	winapi.RegDeleteValue(runKey, "snag")
	winapi.RegDeleteTree(uninstallKey)

	if o.RemoveClips {
		s.emit(Progress{Step: "settings", Pct: 0.5, Note: "Deleting clips"})
		// only remove files we recorded, people pick folders like Videos
		dirs := map[string]bool{}
		for _, c := range trackedClips() {
			os.Remove(c.Path)
			dirs[filepath.Dir(c.Path)] = true
		}
		for d := range dirs {
			os.Remove(d)
		}
		if cfg.ClipsDir != "" {
			os.Remove(cfg.ExportsPath())
			os.Remove(cfg.ClipsDir)
		}
	}

	if o.RemoveData {
		s.emit(Progress{Step: "settings", Pct: 0.7, Note: "Deleting settings and thumbnails"})
		if cfg.FFmpegDir != "" {
			os.Remove(filepath.Join(cfg.FFmpegDir, "ffmpeg.exe"))
			os.Remove(filepath.Join(cfg.FFmpegDir, "ffprobe.exe"))
			os.Remove(cfg.FFmpegDir)
		}
		os.RemoveAll(dataDir())
	}

	s.emit(Progress{Step: "register", Pct: 0.9, Note: "Removing Snag"})
	if appDir != "" {
		os.Remove(filepath.Join(appDir, "Snag.exe"))
		ours := true
		if entries, err := os.ReadDir(appDir); err == nil {
			for _, e := range entries {
				if !strings.EqualFold(e.Name(), "Uninstall Snag.exe") {
					ours = false
				}
			}
		}
		if ours {
			winapi.RunAfterExit(`rmdir /S /Q "` + appDir + `"`)
		} else {
			winapi.RunAfterExit(`del /F /Q "` + filepath.Join(appDir, "Uninstall Snag.exe") + `"`)
		}
	}
}
