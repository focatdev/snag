package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIconPNG []byte

//go:embed build/tray.png
var trayIdlePNG []byte

//go:embed build/tray-live.png
var trayLivePNG []byte

func dataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	dir := filepath.Join(base, "Snag")
	os.MkdirAll(dir, 0o755)
	return dir
}

func main() {
	dir := dataDir()
	setupLog(dir)
	core, err := newCore(dir)
	if err != nil {
		log.Fatal(err)
	}

	app := application.New(application.Options{
		Name:        "Snag",
		Description: "Lightweight replay buffer and clip editor",
		Icon:        appIconPNG,
		Services: []application.Service{
			application.NewService(&SnagService{c: core}),
		},
		Assets: application.AssetOptions{
			Handler:        application.AssetFileServerFS(assets),
			DisableLogging: true,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
			WebviewUserDataPath:           filepath.Join(dir, "webview"),
			// ui is embedded, a disk cache is pointless
			AdditionalBrowserArgs: []string{"--disk-cache-size=8388608"},
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               "gg.snag.desktop",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { core.openWindow() },
		},
		OnShutdown: core.shutdown,
	})
	core.app = app
	core.setupTray()
	core.registerHotkeys()

	launchedHidden := slices.Contains(os.Args[1:], "--tray")
	if !launchedHidden && !core.store.Get().StartMinimized {
		core.openWindow()
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
