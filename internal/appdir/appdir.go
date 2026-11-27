// Package appdir finds the folders Hopdeck keeps for each user.
package appdir

import (
	"os"
	"path/filepath"
)

// DecksEnv names an environment variable that points Hopdeck at another decks folder,
// for development and tests.
const DecksEnv = "HOPDECK_DECKS_DIR"

// Root returns the per-user application folder: %AppData%\Hopdeck on Windows, and
// $XDG_CONFIG_HOME/Hopdeck or ~/.config/Hopdeck on Linux.
func Root() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Hopdeck"), nil
}

// Decks returns the decks folder, creating it when it does not exist yet, so a person
// can drop a deck file into it. DecksEnv overrides the location.
func Decks() (string, error) {
	dir := os.Getenv(DecksEnv)
	if dir == "" {
		root, err := Root()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(root, "decks")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
