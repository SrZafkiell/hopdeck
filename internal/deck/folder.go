package deck

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// File is one deck file of a folder: its deck, or the reason it cannot be used.
type File struct {
	// Name is the file name, without the folder.
	Name string
	// Deck is nil when Err is set.
	Deck *Deck
	Err  error
}

// IsDeckFile reports whether a file name has the deck file suffix, in any letter case.
func IsDeckFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), Suffix)
}

// ReadFolder loads every deck file of dir, sorted by file name. A file that cannot be
// used comes back with its error, so the other files keep working. Only an unreadable
// folder is an error of its own.
func ReadFolder(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, entry := range entries {
		if !IsDeckFile(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		// Stat follows links; only regular files are read, so a pipe or a device that
		// carries the suffix cannot block the reader.
		info, err := os.Stat(path)
		if err != nil {
			files = append(files, File{Name: entry.Name(), Err: err})
			continue
		}
		if info.IsDir() {
			continue
		}
		if !info.Mode().IsRegular() {
			files = append(files, File{Name: entry.Name(), Err: fmt.Errorf("%s is not a regular file", entry.Name())})
			continue
		}
		deck, err := Load(path)
		files = append(files, File{Name: entry.Name(), Deck: deck, Err: err})
	}
	return files, nil
}

// Watch calls onChange with the deck files of dir once at the start, then each time a
// deck file is added, changed or removed, until ctx ends. It polls every interval: a
// decks folder holds a handful of small files, and polling behaves the same on every
// system and on network drives. A folder that cannot be read is reported through err,
// and again when that changes.
func Watch(ctx context.Context, dir string, interval time.Duration, onChange func(files []File, err error)) {
	last := "\x00never"
	check := func() {
		state := fingerprint(dir)
		if state == last {
			return
		}
		last = state
		onChange(ReadFolder(dir))
	}
	check()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

// fingerprint describes the deck files of dir by name, size and modification time, so
// a change shows as a different string. An unreadable folder gives its error instead.
func fingerprint(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "error: " + err.Error()
	}
	var b strings.Builder
	for _, entry := range entries {
		if !IsDeckFile(entry.Name()) {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, entry.Name()))
		if err != nil {
			fmt.Fprintf(&b, "%s|error\n", entry.Name())
			continue
		}
		fmt.Fprintf(&b, "%s|%d|%d\n", entry.Name(), info.Size(), info.ModTime().UnixNano())
	}
	return b.String()
}
