package main

import (
	"embed"
	"log"

	"github.com/SrZafkiell/hopdeck/internal/appdir"
	"github.com/SrZafkiell/hopdeck/internal/profiles"
	"github.com/SrZafkiell/hopdeck/internal/settings"
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
	settingsPath, err := appdir.SettingsFile()
	if err != nil {
		log.Fatalf("the settings file cannot be located: %v", err)
	}
	store := settings.NewStore(settingsPath)
	current, notice := store.Load()
	// Writing the settings back creates the file on the first start and keeps it in the
	// current schema version. A store that could not read the file refuses to write.
	if err := store.Save(current); err != nil {
		log.Printf("settings not saved: %v", err)
	}
	profileFiles, err := profiles.DefaultFiles()
	if err != nil {
		// Without a home folder there are no AWS files to read; the chooser then says so.
		log.Printf("the AWS configuration files cannot be located: %v", err)
	}
	decks := NewDeckService(DeckServiceOptions{
		Folder:       folder,
		Notice:       notice,
		Store:        store,
		Settings:     current,
		ProfileFiles: profileFiles,
	})

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
