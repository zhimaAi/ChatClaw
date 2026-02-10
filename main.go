package main

import (
	"embed"
	_ "embed"
	"log"
	"runtime"

	"willclaw/internal/bootstrap"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/sysicon.png
var sysIconDefault []byte

//go:embed build/appicon.png
var sysIconWindows []byte

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU() / 2)
}

func main() {
	// On macOS the white template icon works perfectly;
	// on Windows we need the dark-outlined variant for taskbar contrast.
	icon := sysIconDefault
	if runtime.GOOS == "windows" {
		icon = sysIconWindows
	}

	app, cleanup, err := bootstrap.NewApp(bootstrap.Options{
		Assets: assets,
		Icon:   icon,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
