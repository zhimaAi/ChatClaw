package main

import (
	"embed"
	_ "embed"
	"log"
	"runtime"

	"willchat/internal/bootstrap"
	"willchat/internal/sqlite"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/sysicon.png
var icon []byte

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU() / 2)

	// application.RegisterEvent[string]("time")
}

func main() {
	app, err := bootstrap.NewApp(bootstrap.Options{
		Assets: assets,
		Icon:   icon,
		// Locale 为空时自动检测系统语言
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := sqlite.Init(app); err != nil {
		log.Fatal("sqlite init failed:", err)
	}
	defer sqlite.Close(app)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
