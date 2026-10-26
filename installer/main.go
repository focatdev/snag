package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:ui
var assets embed.FS

//go:embed payload/Snag.exe
var payload []byte

//go:embed icon.png
var iconPNG []byte

func main() {
	// double-clicking the copy in the install folder passes no flag
	uninstall := slices.Contains(os.Args[1:], "--uninstall")
	if exe, err := os.Executable(); err == nil && strings.Contains(strings.ToLower(filepath.Base(exe)), "uninstall") {
		uninstall = true
	}
	svc := &SetupService{uninstall: uninstall}

	app := application.New(application.Options{
		Name:        "Snag Setup",
		Description: "Installs Snag",
		Icon:        iconPNG,
		Services:    []application.Service{application.NewService(svc)},
		Assets: application.AssetOptions{
			Handler:        application.AssetFileServerFS(assets),
			DisableLogging: true,
		},
		Windows: application.WindowsOptions{
			WebviewUserDataPath: filepath.Join(os.TempDir(), "snag-setup-webview"),
		},
	})
	svc.app = app

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                      "Snag Setup",
		Width:                      760,
		Height:                     460,
		DisableResize:              true,
		Frameless:                  true,
		InitialPosition:            application.WindowCentered,
		BackgroundColour:           application.NewRGB(0x0e, 0x0f, 0x11),
		URL:                        "/installer.html",
		Windows:                    application.WindowsWindow{Theme: application.Dark},
		DefaultContextMenuDisabled: true,
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
