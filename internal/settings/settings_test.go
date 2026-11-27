package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingFileGivesDefaults(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), FileName))
	settings, notice := store.Load()
	if notice != "" || settings.Version != Version || settings.Profiles == nil || len(settings.Profiles) != 0 {
		t.Fatalf("got %+v, %q", settings, notice)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-yet-created")
	path := filepath.Join(dir, FileName)
	store := NewStore(path)
	want := Settings{Profiles: map[string]string{"team.hopdeck.yaml": "example-dev"}}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, notice := NewStore(path).Load()
	if notice != "" || got.Version != Version || got.Profiles["team.hopdeck.yaml"] != "example-dev" {
		t.Fatalf("got %+v, %q", got, notice)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != FileName {
		t.Fatalf("Save left other files behind: %v", entries)
	}
	data, _ := os.ReadFile(path)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil || raw["version"] != float64(Version) {
		t.Fatalf("the file is not versioned JSON: %s", data)
	}
}

func TestUnusableFilesAreSetAside(t *testing.T) {
	cases := map[string]string{
		"not JSON":         "{ this is not json",
		"unknown field":    `{"version": 1, "profiles": {}, "colour": "blue"}`,
		"newer version":    `{"version": 2, "profiles": {}}`,
		"no version":       `{"profiles": {}}`,
		"two JSON values":  `{"version": 1, "profiles": {}} {"version": 1}`,
		"larger than 1MiB": `{"version": 1, "profiles": {"x": "` + strings.Repeat("a", maxSize) + `"}}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, FileName)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			store := NewStore(path)
			settings, notice := store.Load()
			if settings.Version != Version || len(settings.Profiles) != 0 {
				t.Fatalf("not the defaults: %+v", settings)
			}
			if !strings.Contains(notice, "could not be used") || !strings.Contains(notice, FileName+".set-aside-") {
				t.Fatalf("notice: %q", notice)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("the unusable file was not moved: %v", err)
			}
			aside, _ := filepath.Glob(filepath.Join(dir, FileName+".set-aside-*"))
			if len(aside) != 1 {
				t.Fatalf("set-aside files: %v", aside)
			}
			if kept, _ := os.ReadFile(aside[0]); string(kept) != content {
				t.Fatal("the set-aside file does not hold the old content")
			}
			if strings.Contains(notice, "colour") || strings.Contains(notice, "blue") {
				t.Fatalf("the notice repeats the file's content: %q", notice)
			}
			if err := store.Save(Defaults()); err != nil {
				t.Fatalf("Save after setting aside: %v", err)
			}
		})
	}
}

func TestUnreadableFileIsNeverOverwritten(t *testing.T) {
	// A folder where the file should be: it exists but cannot be read as a file.
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	settings, notice := store.Load()
	if settings.Version != Version || !strings.Contains(notice, "cannot be read") {
		t.Fatalf("got %+v, %q", settings, notice)
	}
	if err := store.Save(Defaults()); err == nil {
		t.Fatal("Save wrote over a file it could not read")
	}
}
