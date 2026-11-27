package main

import (
	"embed"
	"log"

	"github.com/SrZafkiell/hopdeck/internal/appdir"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// The built front end is embedded, so the executable needs no files next to it.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	folder, err := appdir.Decks()
	if err != nil {
		log.Fatalf("the decks folder cannot be created: %v", err)
	}
	decks := NewDeckService(folder)

	app := application.New(application.Options{
		Name:        "Hopdeck",
		Description: "AWS Session Manager tunnels from a shared deck file",
		Services: []application.Service{
			application.NewService(decks),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})
	// Set before Run, so before the service starts watching and can emit.
	decks.emit = func(snapshot Snapshot) {
		app.Event.Emit(eventDecks, snapshot)
	}

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Hopdeck",
		Width:  1000,
		Height: 640,
		URL:    "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
