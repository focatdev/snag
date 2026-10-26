package config

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Hotkeys struct {
	SaveClip     string `json:"saveClip"`
	SaveShort    string `json:"saveShort"`
	Screenshot   string `json:"screenshot"`
	Bookmark     string `json:"bookmark"`
	ToggleBuffer string `json:"toggleBuffer"`
}

type Settings struct {
	ClipsDir       string `json:"clipsDir"`
	ExportsDir     string `json:"exportsDir"`
	OrganizeByGame bool   `json:"organizeByGame"`
	FileTemplate   string `json:"fileTemplate"`
	FFmpegPath     string `json:"ffmpegPath"`
	FFmpegDir      string `json:"ffmpegDir"`

	Encoder    string `json:"encoder"`
	Codec      string `json:"codec"`
	FPS        int    `json:"fps"`
	Resolution string `json:"resolution"`
	Quality    string `json:"quality"`
	Monitor    int    `json:"monitor"`
	DrawCursor bool   `json:"drawCursor"`

	BufferSeconds int  `json:"bufferSeconds"`
	ShortSeconds  int  `json:"shortSeconds"`
	BufferOnDisk  bool `json:"bufferOnDisk"`
	MergeOverlaps bool `json:"mergeOverlaps"`

	CaptureSystemAudio bool   `json:"captureSystemAudio"`
	CaptureMic         bool   `json:"captureMic"`
	SeparateVoice      bool   `json:"separateVoice"`
	DropSilentMic      bool   `json:"dropSilentMic"`
	SystemDeviceID     string `json:"systemDeviceId"`
	MicDeviceID        string `json:"micDeviceId"`

	Hotkeys Hotkeys `json:"hotkeys"`

	SmartMode       bool   `json:"smartMode"`
	AutoStartBuffer bool   `json:"autoStartBuffer"`
	SaveSound       bool   `json:"saveSound"`
	SaveSoundName   string `json:"saveSoundName"`
	SoundVolume     int    `json:"soundVolume"`
	CustomSound     string `json:"customSound"`

	ShowOverlay   bool   `json:"showOverlay"`
	OverlayCorner string `json:"overlayCorner"`
	CopyExports   bool   `json:"copyExports"`

	StorageBudgetEnabled bool `json:"storageBudgetEnabled"`
	StorageBudgetGB      int  `json:"storageBudgetGB"`
	AutoShrinkDays       int  `json:"autoShrinkDays"`
	CopyScreenshots      bool `json:"copyScreenshots"`

	CatboxAutoUpload  bool   `json:"catboxAutoUpload"`
	CatboxDeleteAfter bool   `json:"catboxDeleteAfter"`
	CatboxUserhash    string `json:"catboxUserhash"`
	Uploader          string `json:"uploader"`

	AIProvider   string `json:"aiProvider"`
	AIKey        string `json:"aiKey"`
	AIModel      string `json:"aiModel"`
	AIAutoTitle  bool   `json:"aiAutoTitle"`
	AITranscribe bool   `json:"aiTranscribe"`

	LaunchOnStartup bool `json:"launchOnStartup"`
	StartMinimized  bool `json:"startMinimized"`
	CloseToTray     bool `json:"closeToTray"`

	AutoUpdate  bool   `json:"autoUpdate"`
	Onboarded   bool   `json:"onboarded"`
	LastVersion string `json:"lastVersion"`
}

func Defaults() Settings {
	home, _ := os.UserHomeDir()
	return Settings{
		ClipsDir:           filepath.Join(home, "Videos", "Snag"),
		FileTemplate:       DefaultTemplate,
		ShortSeconds:       15,
		MergeOverlaps:      true,
		DropSilentMic:      true,
		CopyScreenshots:    true,
		AutoUpdate:         true,
		SaveSoundName:      "pop",
		SoundVolume:        70,
		ShowOverlay:        true,
		OverlayCorner:      "top-right",
		CopyExports:        true,
		Encoder:            "auto",
		Codec:              "h264",
		FPS:                60,
		Resolution:         "native",
		Quality:            "medium",
		DrawCursor:         true,
		BufferSeconds:      60,
		CaptureSystemAudio: true,
		CaptureMic:         false,
		Hotkeys: Hotkeys{
			SaveClip:     "F8",
			SaveShort:    "Shift+F8",
			Screenshot:   "Ctrl+F8",
			Bookmark:     "Alt+F8",
			ToggleBuffer: "Ctrl+Alt+F8",
		},
		SmartMode:            false,
		AutoStartBuffer:      true,
		SaveSound:            true,
		StorageBudgetEnabled: false,
		StorageBudgetGB:      20,
		CloseToTray:          true,
	}
}

func (s Settings) CaptureKey() string {
	b, _ := json.Marshal([]any{s.FFmpegPath, s.Encoder, s.Codec, s.FPS, s.Resolution, s.Quality, s.Monitor, s.DrawCursor,
		s.CaptureSystemAudio, s.CaptureMic, s.SystemDeviceID, s.MicDeviceID, s.SeparateVoice, s.BufferOnDisk})
	return string(b)
}

func (s *Settings) Normalize() {
	d := Defaults()
	if s.ClipsDir == "" {
		s.ClipsDir = d.ClipsDir
	}
	if s.Encoder == "" {
		s.Encoder = "auto"
	}
	if strings.TrimSpace(s.FileTemplate) == "" {
		s.FileTemplate = DefaultTemplate
	}
	switch s.OverlayCorner {
	case "top-left", "top-right", "bottom-left", "bottom-right":
	default:
		s.OverlayCorner = "top-right"
	}
	switch s.SaveSoundName {
	case "snap", "chime", "pop", "shutter", "custom":
	default:
		s.SaveSoundName = "pop"
	}
	if s.SoundVolume < 0 || s.SoundVolume > 100 {
		s.SoundVolume = 70
	}
	switch s.AIProvider {
	case "", "openai", "groq", "gemini", "anthropic":
	default:
		s.AIProvider = ""
	}
	switch s.AutoShrinkDays {
	case 0, 3, 7, 14, 30:
	default:
		s.AutoShrinkDays = 0
	}
	if s.ShortSeconds < 5 {
		s.ShortSeconds = 5
	}
	if s.Codec != "h264" && s.Codec != "hevc" {
		s.Codec = "h264"
	}
	switch s.FPS {
	case 30, 60, 120, 144:
	default:
		s.FPS = 60
	}
	switch s.Resolution {
	case "native", "1440", "1080", "720":
	default:
		s.Resolution = "native"
	}
	switch s.Quality {
	case "low", "medium", "high", "ultra":
	default:
		s.Quality = "medium"
	}
	if s.BufferSeconds < 10 {
		s.BufferSeconds = 10
	}
	if s.BufferSeconds > 300 {
		s.BufferSeconds = 300
	}
	if s.ShortSeconds > s.BufferSeconds {
		s.ShortSeconds = s.BufferSeconds
	}
	if s.StorageBudgetGB < 1 {
		s.StorageBudgetGB = 1
	}
	if s.Monitor < 0 {
		s.Monitor = 0
	}
}

func (s Settings) OutputSize(srcW, srcH int) (int, int) {
	target := 0
	switch s.Resolution {
	case "1440":
		target = 1440
	case "1080":
		target = 1080
	case "720":
		target = 720
	}
	if target == 0 || srcH <= target || srcH == 0 {
		return srcW, srcH
	}
	w := int(math.Round(float64(srcW)*float64(target)/float64(srcH)/2)) * 2
	return w, target
}

// bits per pixel drop as resolution goes up
func (s Settings) BitrateKbps(w, h int) int {
	base := map[string]float64{"low": 6000, "medium": 10000, "high": 16000, "ultra": 24000}[s.Quality]
	if base == 0 {
		base = 10000
	}
	if w <= 0 || h <= 0 {
		w, h = 1920, 1080
	}
	px := float64(w*h) / (1920 * 1080)
	fps := float64(s.FPS) / 60
	br := base * math.Pow(px, 0.75) * math.Pow(fps, 0.6)
	if s.Codec == "hevc" {
		br *= 0.7
	}
	return int(math.Round(br/100) * 100)
}

type Store struct {
	mu   sync.Mutex
	path string
	cur  Settings
}

func Open(dataDir string) (*Store, error) {
	st := &Store{path: filepath.Join(dataDir, "settings.json"), cur: Defaults()}
	b, err := os.ReadFile(st.path)
	if err == nil {
		s := Defaults()
		if json.Unmarshal(b, &s) == nil {
			st.cur = s
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	st.cur.Normalize()
	return st, nil
}

func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.cur
}

func (st *Store) Set(s Settings) (Settings, error) {
	s.Normalize()
	st.mu.Lock()
	defer st.mu.Unlock()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return st.cur, err
	}
	if err := WriteFileAtomic(st.path, b); err != nil {
		return st.cur, err
	}
	st.cur = s
	return s, nil
}

func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

const DefaultTemplate = "{game} {date} {time}"

// empty means clips\Exports
func (s Settings) ExportsPath() string {
	if strings.TrimSpace(s.ExportsDir) != "" {
		return s.ExportsDir
	}
	return filepath.Join(s.ClipsDir, "Exports")
}

func SafeName(v string) string {
	v = strings.Map(func(c rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, c) || c < 32 {
			return -1
		}
		return c
	}, v)
	v = strings.Trim(strings.TrimSpace(v), ".")
	if v == "" {
		return "Clip"
	}
	return v
}

func RenderName(template, game string, seconds int, when time.Time) string {
	if game == "" {
		game = "Desktop"
	}
	r := strings.NewReplacer(
		"{game}", game,
		"{date}", when.Format("2006-01-02"),
		"{time}", when.Format("15-04-05"),
		"{length}", strconv.Itoa(seconds)+"s",
		"{year}", when.Format("2006"),
		"{month}", when.Format("01"),
		"{day}", when.Format("02"),
	)
	return SafeName(r.Replace(template))
}

func (s Settings) ClipPath(game string, seconds int, when time.Time) string {
	dir := s.ClipsDir
	if s.OrganizeByGame {
		g := game
		if g == "" {
			g = "Desktop"
		}
		dir = filepath.Join(dir, SafeName(g))
	}
	return filepath.Join(dir, RenderName(s.FileTemplate, game, seconds, when))
}

func (s Settings) FFmpegInstallDir(dataDir string) string {
	if strings.TrimSpace(s.FFmpegDir) != "" {
		return s.FFmpegDir
	}
	return filepath.Join(dataDir, "bin")
}
