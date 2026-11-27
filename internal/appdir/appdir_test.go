package appdir

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDecksDefaultsToTheUserConfigFolder(t *testing.T) {
	base := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", base)
	} else {
		t.Setenv("XDG_CONFIG_HOME", base)
	}
	t.Setenv(DecksEnv, "")

	dir, err := Decks()
	if err != nil {
		t.Fatalf("Decks: %v", err)
	}
	want := filepath.Join(base, "Hopdeck", "decks")
	if dir != want {
		t.Fatalf("got %s, want %s", dir, want)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("the folder was not created: %v", err)
	}
}

func TestSettingsFileSitsInTheApplicationFolder(t *testing.T) {
	base := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", base)
	} else {
		t.Setenv("XDG_CONFIG_HOME", base)
	}
	t.Setenv(SettingsEnv, "")

	path, err := SettingsFile()
	if err != nil || path != filepath.Join(base, "Hopdeck", "settings.json") {
		t.Fatalf("got %s, %v", path, err)
	}

	other := filepath.Join(t.TempDir(), "other.json")
	t.Setenv(SettingsEnv, other)
	if path, err := SettingsFile(); err != nil || path != other {
		t.Fatalf("override: got %s, %v", path, err)
	}
}

func TestDecksFollowsTheEnvironmentVariable(t *testing.T) {
	want := filepath.Join(t.TempDir(), "elsewhere")
	t.Setenv(DecksEnv, want)

	dir, err := Decks()
	if err != nil || dir != want {
		t.Fatalf("got %s, %v; want %s", dir, err, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("the folder was not created: %v", err)
	}
}
