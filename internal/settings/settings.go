// Package settings keeps Hopdeck's own choices in one versioned JSON file in the per-user
// application folder. It holds choices only, never a credential.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SrZafkiell/hopdeck/internal/deck"
)

// Version is the schema version this build reads and writes. A later version comes with
// a migration from the earlier ones.
const Version = 1

// FileName is the settings file's name in the application folder.
const FileName = "settings.json"

// maxSize caps what is read of the settings file.
const maxSize = 1 << 20

// Settings are the person's choices.
type Settings struct {
	Version int `json:"version"`
	// Profiles maps a deck file name to the local AWS profile chosen for it.
	Profiles map[string]string `json:"profiles"`
	// Ports maps a deck file name and a tunnel name to the local port chosen for the
	// tunnel on this computer, in place of the deck file's. A tunnel that is not here
	// uses the deck file's port.
	Ports map[string]map[string]int `json:"ports"`
}

// Defaults returns empty settings of the current version.
func Defaults() Settings {
	return Settings{Version: Version, Profiles: map[string]string{}, Ports: map[string]map[string]int{}}
}

// Clone returns a deep copy, which the caller may change without touching s.
func (s Settings) Clone() Settings {
	clone := Settings{
		Version:  s.Version,
		Profiles: make(map[string]string, len(s.Profiles)),
		Ports:    make(map[string]map[string]int, len(s.Ports)),
	}
	for file, profile := range s.Profiles {
		clone.Profiles[file] = profile
	}
	for file, tunnels := range s.Ports {
		clone.Ports[file] = make(map[string]int, len(tunnels))
		for tunnel, port := range tunnels {
			clone.Ports[file][tunnel] = port
		}
	}
	return clone
}

// Store reads and writes one settings file. It is safe for concurrent use.
type Store struct {
	path string

	mu sync.Mutex
	// unreadable is set when the file existed but could not be read at all; the store
	// then refuses to save, so it never overwrites a file it could not see.
	unreadable bool
}

// NewStore returns a store for the settings file at path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads the settings. A missing file gives the defaults. A file that cannot be used
// (not JSON, an unknown field, a version this build does not read, a local port outside
// the deck file format's range, too large) is renamed
// to settings.json.set-aside-<time> and the defaults are used; the notice says so in
// plain words, for the window to show. A file that cannot be opened at all also gives the
// defaults and a notice, and the store then refuses to save over it.
func (s *Store) Load() (Settings, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := readLimited(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), ""
	}
	if err != nil {
		s.unreadable = true
		return Defaults(), fmt.Sprintf(
			"The settings file cannot be read (%v), so Hopdeck runs with default settings and will not save over it.", err)
	}

	settings, reason := decode(data)
	if reason == "" {
		return settings, ""
	}
	aside := fmt.Sprintf("%s.set-aside-%s", s.path, time.Now().Format("20060102-150405"))
	if err := os.Rename(s.path, aside); err != nil {
		s.unreadable = true
		return Defaults(), fmt.Sprintf(
			"The settings file could not be used (%s) or moved aside (%v), so Hopdeck runs with default settings and will not save over it.",
			reason, err)
	}
	return Defaults(), fmt.Sprintf(
		"The settings file could not be used (%s), so Hopdeck started with default settings. The old file was kept as %s.",
		reason, filepath.Base(aside))
}

// Save writes the settings atomically: a temporary file in the same folder, readable
// only by the user, then a rename over the old file.
func (s *Store) Save(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unreadable {
		return errors.New("the settings are not saved, because the settings file could not be read when Hopdeck started")
	}

	settings.Version = Version
	if settings.Profiles == nil {
		settings.Profiles = map[string]string{}
	}
	if settings.Ports == nil {
		settings.Ports = map[string]map[string]int{}
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, FileName+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		os.Remove(tempPath)
		return err
	}
	return nil
}

func readLimited(path string) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maxSize+1))
}

// decode reads the settings strictly. It returns the reason in words when the file
// cannot be used; the reason never repeats the content of the file.
func decode(data []byte) (Settings, string) {
	if len(data) > maxSize {
		return Settings{}, "it is larger than 1 MiB"
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var settings Settings
	if err := decoder.Decode(&settings); err != nil {
		return Settings{}, "it is not valid settings JSON"
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Settings{}, "it holds more than one JSON value"
	}
	switch {
	case settings.Version > Version:
		return Settings{}, fmt.Sprintf("it was written by a newer Hopdeck, settings version %d", settings.Version)
	case settings.Version != Version:
		return Settings{}, fmt.Sprintf("its settings version %d is not one this Hopdeck reads", settings.Version)
	}
	if settings.Profiles == nil {
		settings.Profiles = map[string]string{}
	}
	if settings.Ports == nil {
		settings.Ports = map[string]map[string]int{}
	}
	for _, tunnels := range settings.Ports {
		for _, port := range tunnels {
			if port < deck.MinLocalPort || port > deck.MaxLocalPort {
				return Settings{}, fmt.Sprintf("it holds a local port outside %d-%d", deck.MinLocalPort, deck.MaxLocalPort)
			}
		}
	}
	return settings, ""
}
