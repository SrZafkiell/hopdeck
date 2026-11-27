package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SrZafkiell/hopdeck/internal/deck"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// eventDecks carries a new Snapshot to the window whenever the decks folder changes.
const eventDecks = "decks"

// pollInterval is how often the decks folder is checked for changes.
const pollInterval = 2 * time.Second

func init() {
	application.RegisterEvent[Snapshot](eventDecks)
}

// Snapshot is what the window shows of the decks folder.
type Snapshot struct {
	// Folder is the decks folder, so the window can say where deck files go.
	Folder string `json:"folder"`
	// Error is set when the folder itself cannot be read.
	Error string     `json:"error"`
	Decks []DeckFile `json:"decks"`
	// Notice tells the person about something Hopdeck did on its own, such as setting a
	// damaged settings file aside; empty when there is nothing to tell.
	Notice string `json:"notice"`
}

// DeckFile is one deck file: its tunnels, or the problems that keep it from loading.
type DeckFile struct {
	File     string        `json:"file"`
	Name     string        `json:"name"`
	Problems []ProblemView `json:"problems"`
	Tunnels  []TunnelView  `json:"tunnels"`
}

// ProblemView is one problem of a deck file that cannot be used.
type ProblemView struct {
	Line  int    `json:"line"`
	Field string `json:"field"`
	Text  string `json:"text"`
}

// TunnelView is one tunnel as the window shows it.
type TunnelView struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Deck       string  `json:"deck"`
	Region     string  `json:"region"`
	JumpHost   string  `json:"jumpHost"`
	RemoteHost string  `json:"remoteHost"`
	RemotePort int     `json:"remotePort"`
	LocalPort  int     `json:"localPort"`
	Profile    *string `json:"profile"`
	State      string  `json:"state"`
	Reason     string  `json:"reason"`
}

// DeckService watches the decks folder and gives the window its content. It is the only
// place that turns the Go core's decks into what the window shows.
type DeckService struct {
	folder string
	notice string
	// emit sends a new snapshot to the window; main sets it once the app exists.
	emit func(Snapshot)

	mu       sync.Mutex
	snapshot Snapshot
	stop     context.CancelFunc
}

// NewDeckService returns a service for the given decks folder. The notice, if any, is
// shown in the window until the person dismisses it.
func NewDeckService(folder, notice string) *DeckService {
	return &DeckService{
		folder:   folder,
		notice:   notice,
		snapshot: Snapshot{Folder: folder, Decks: []DeckFile{}, Notice: notice},
	}
}

// Snapshot returns what the decks folder holds now.
func (s *DeckService) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

// ServiceStartup starts watching the decks folder when the app starts.
func (s *DeckService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.stop = cancel
	s.mu.Unlock()
	go deck.Watch(ctx, s.folder, pollInterval, s.update)
	return nil
}

// ServiceShutdown stops watching when the app ends.
func (s *DeckService) ServiceShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		s.stop()
	}
	return nil
}

func (s *DeckService) update(files []deck.File, err error) {
	snapshot := snapshotOf(s.folder, files, err)
	snapshot.Notice = s.notice
	s.mu.Lock()
	s.snapshot = snapshot
	emit := s.emit
	s.mu.Unlock()
	if emit != nil {
		emit(snapshot)
	}
}

// snapshotOf turns the files of the decks folder into what the window shows. A tunnel
// has no profile and is idle until profile mapping (M02-06) and the tunnel engine (M04)
// exist.
func snapshotOf(folder string, files []deck.File, err error) Snapshot {
	snapshot := Snapshot{Folder: folder, Decks: []DeckFile{}}
	if err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}
	for _, file := range files {
		view := DeckFile{File: file.Name, Name: file.Name, Problems: []ProblemView{}, Tunnels: []TunnelView{}}
		if file.Err != nil {
			view.Problems = problemsOf(file.Err)
			snapshot.Decks = append(snapshot.Decks, view)
			continue
		}
		view.Name = file.Deck.Name
		for _, tunnel := range file.Deck.Tunnels {
			view.Tunnels = append(view.Tunnels, TunnelView{
				ID:         file.Name + "/" + tunnel.Name,
				Name:       tunnel.Name,
				Deck:       file.Deck.Name,
				Region:     tunnel.Region,
				JumpHost:   jumpHostText(tunnel.JumpHost),
				RemoteHost: tunnel.Remote.Host,
				RemotePort: tunnel.Remote.Port,
				LocalPort:  tunnel.LocalPort,
				State:      "idle",
			})
		}
		snapshot.Decks = append(snapshot.Decks, view)
	}
	return snapshot
}

func problemsOf(err error) []ProblemView {
	var deckErr *deck.Error
	if !errors.As(err, &deckErr) {
		return []ProblemView{{Text: err.Error()}}
	}
	problems := make([]ProblemView, 0, len(deckErr.Problems))
	for _, problem := range deckErr.Problems {
		problems = append(problems, ProblemView{Line: problem.Line, Field: problem.Field, Text: problem.Text})
	}
	return problems
}

// jumpHostText writes a jump host the way the window shows it: the instance ID, or the
// tags as tag:Name=value, sorted by name.
func jumpHostText(jumpHost deck.JumpHost) string {
	if jumpHost.InstanceID != "" {
		return jumpHost.InstanceID
	}
	names := make([]string, 0, len(jumpHost.Tags))
	for name := range jumpHost.Tags {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("tag:%s=%s", name, jumpHost.Tags[name]))
	}
	return strings.Join(parts, ", ")
}
