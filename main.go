package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	// An app opened from the Finder gets a bare PATH, without Homebrew. yt-dlp
	// also looks up ffmpeg and deno through this PATH.
	os.Setenv("PATH", "/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))

	app := application.New(application.Options{
		Name:     "video-downloader",
		Services: []application.Service{application.NewService(&Service{})},
		Assets: application.AssetOptions{
			// Unlike AssetFileServerFS, this also serves /wails/runtime.js,
			// which lets the page work without npm or a bundler.
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "video-downloader",
		Width:  960,
		Height: 820,
		URL:    "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
