package main

import (
	"embed"
	_ "embed"
	"log"

	"willchat/internal/sqlite"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/sysicon.png
var icon []byte

func init() {
	// application.RegisterEvent[string]("time")
}

func main() {
	app := application.New(application.Options{
		Name:        "WillChat",
		Description: "A demo of using raw HTML & CSS",
		Services: []application.Service{
			application.NewService(&GreetService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "main",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})

	// 创建系统托盘
	systrayMenu := app.NewMenu()
	systrayMenu.Add("Show").OnClick(func(ctx *application.Context) {
		mainWindow.Show()
	})
	systrayMenu.Add("Quit").OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	app.SystemTray.New().SetIcon(icon).SetMenu(systrayMenu)

	if err := sqlite.Init(app); err != nil {
		log.Fatal("sqlite init failed:", err)
	}
	defer sqlite.Close(app)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
