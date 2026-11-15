package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/vmaerten/video-downloader/internal/jobs"
	"github.com/vmaerten/video-downloader/internal/ytdlp"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	// An app opened from the Finder gets a bare PATH, without Homebrew. yt-dlp
	// also looks up ffmpeg and deno through this PATH.
	os.Setenv("PATH", "/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))

	store, err := newStore()
	if err != nil {
		log.Fatal(err)
	}

	app := application.New(application.Options{
		Name:     "Épuisette",
		Services: []application.Service{application.NewService(&Service{jobs: store})},
		Assets: application.AssetOptions{
			// Unlike AssetFileServerFS, this also serves /wails/runtime.js,
			// which lets the page work without npm or a bundler.
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Menu.Set(applicationMenu())

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Épuisette",
		Width:  960,
		Height: 820,
		URL:    "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// newStore downloads into the user's cache folder and moves finished files to ~/Downloads.
func newStore() (*jobs.Store, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return jobs.New(
		filepath.Join(cache, "fr.vmaerten.epuisette", "jobs"),
		filepath.Join(home, "Downloads"),
		ytdlp.Download,
	)
}
