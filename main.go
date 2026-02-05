package main

import (
	"embed"
	_ "embed"
	"log"
	"runtime"

	"willchat/internal/bootstrap"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/sysicon.png
var icon []byte

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU() / 2)
}

func main() {
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
