package main

import (
	"embed"
	_ "embed"
	"log"
	"runtime"

	"willchat/internal/bootstrap"
	"willchat/internal/services/settings"
	"willchat/internal/sqlite"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/sysicon.png
var iconPNG []byte

// Windows tray icons are most reliable with .ico
//
//go:embed build/windows/icon.ico
var iconICO []byte

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU() / 2)

	// application.RegisterEvent[string]("time")
}

func main() {
	appIcon := iconPNG
	if runtime.GOOS == "windows" && len(iconICO) > 0 {
		appIcon = iconICO
	}

	app, err := bootstrap.NewApp(bootstrap.Options{
		Assets: assets,
		Icon:   appIcon,
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := sqlite.Init(app); err != nil {
		log.Fatal("sqlite init failed:", err)
	}
	if err := settings.InitCache(app); err != nil {
		log.Fatal("settings cache init failed:", err)
	}
	defer sqlite.Close(app)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
