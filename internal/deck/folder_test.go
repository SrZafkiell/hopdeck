package deck

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadFolder(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "b-team"+Suffix, minimal)
	write(t, dir, "a-broken"+Suffix, edit(minimal, "port: 5432", "port: 70000"))
	write(t, dir, "notes.yaml", minimal)
	write(t, dir, "UPPER.HOPDECK.YAML", minimal)
	if err := os.Mkdir(filepath.Join(dir, "folder"+Suffix), 0o700); err != nil {
		t.Fatal(err)
	}

	files, err := ReadFolder(dir)
	if err != nil {
		t.Fatalf("ReadFolder: %v", err)
	}
	var names []string
	for _, file := range files {
		names = append(names, file.Name)
	}
	if got := strings.Join(names, ","); got != "UPPER.HOPDECK.YAML,a-broken.hopdeck.yaml,b-team.hopdeck.yaml" {
		t.Fatalf("files: %s", got)
	}
	broken, good := files[1], files[2]
	if broken.Deck != nil || broken.Err == nil || !strings.Contains(broken.Err.Error(), "a-broken.hopdeck.yaml:10:") {
		t.Errorf("the broken file: %+v", broken)
	}
	if good.Err != nil || good.Deck == nil || good.Deck.Tunnels[0].Name != "db" {
		t.Errorf("the good file: %+v", good)
	}
}

func TestReadFolderMissing(t *testing.T) {
	if _, err := ReadFolder(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing folder gave no error")
	}
}

func TestWatch(t *testing.T) {
	dir := t.TempDir()
	changes := make(chan []File, 10)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, dir, 10*time.Millisecond, func(files []File, err error) {
			if err != nil {
				t.Errorf("Watch: %v", err)
			}
			changes <- files
		})
		close(done)
	}()

	expect := func(what string, want int) []File {
		t.Helper()
		select {
		case files := <-changes:
			if len(files) != want {
				t.Fatalf("%s: got %d files, want %d", what, len(files), want)
			}
			return files
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: no change reported", what)
			return nil
		}
	}

	expect("start", 0)
	write(t, dir, "team"+Suffix, minimal)
	if files := expect("added", 1); files[0].Err != nil {
		t.Fatalf("added file: %v", files[0].Err)
	}
	write(t, dir, "team"+Suffix, minimal+"owner: ops\n")
	if files := expect("changed", 1); files[0].Err == nil {
		t.Fatal("the changed file should be refused")
	}
	write(t, dir, "ignored.txt", "not a deck file")
	if err := os.Remove(filepath.Join(dir, "team"+Suffix)); err != nil {
		t.Fatal(err)
	}
	expect("removed", 0)

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not stop when its context ended")
	}
	select {
	case files := <-changes:
		t.Fatalf("a change was reported for a file that is not a deck file: %+v", files)
	default:
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
