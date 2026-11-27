package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SrZafkiell/hopdeck/internal/awsapi/awssdk"
	"github.com/SrZafkiell/hopdeck/internal/deck"
	"github.com/SrZafkiell/hopdeck/internal/identity"
	"github.com/SrZafkiell/hopdeck/internal/profiles"
	"github.com/SrZafkiell/hopdeck/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// eventDecks carries a new Snapshot to the window whenever the decks folder or a choice
// changes.
const eventDecks = "decks"

// pollInterval is how often the decks folder is checked for changes.
const pollInterval = 2 * time.Second

// reasonNoProfile is the reason of every tunnel of a deck file that has no profile yet.
const reasonNoProfile = "No AWS profile is chosen for this deck file yet."

// checkTimeout bounds one identity check, so a hung network never blocks the window.
const checkTimeout = 30 * time.Second

// fallbackRegion is used for the identity check when neither the profile nor the
// tunnel gives a usable region.
const fallbackRegion = "us-east-1"

var regionPattern = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]{1,2}$`)

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
	File string `json:"file"`
	Name string `json:"name"`
	// Profile is the local AWS profile chosen for this deck file; empty when none is.
	Profile string `json:"profile"`
	// SuggestedProfile is the profile the deck file suggests; empty when it suggests none.
	SuggestedProfile string        `json:"suggestedProfile"`
	Problems         []ProblemView `json:"problems"`
	Tunnels          []TunnelView  `json:"tunnels"`
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
	DeckFile   string  `json:"deckFile"`
	Region     string  `json:"region"`
	JumpHost   string  `json:"jumpHost"`
	RemoteHost string  `json:"remoteHost"`
	RemotePort int     `json:"remotePort"`
	LocalPort  int     `json:"localPort"`
	Profile    *string `json:"profile"`
	State      string  `json:"state"`
	Reason     string  `json:"reason"`
}

// ProfileView is one AWS profile of this computer as the window lists it: its name and
// kind, never a credential.
type ProfileView struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ProfileList is what the profile chooser shows.
type ProfileList struct {
	Profiles []ProfileView `json:"profiles"`
	// Error is set when the AWS configuration files cannot be read.
	Error string `json:"error"`
}

// IdentityView is the answer of an identity check as the window shows it: the state
// (valid, expired, needs-mfa, needs-sign-in or error), the account and the caller when
// valid, and the reason when not.
type IdentityView struct {
	State   string `json:"state"`
	Account string `json:"account"`
	Caller  string `json:"caller"`
	Reason  string `json:"reason"`
}

// DeckServiceOptions are what the deck service starts with.
type DeckServiceOptions struct {
	// Folder is the decks folder.
	Folder string
	// Notice is shown in the window until the person dismisses it.
	Notice string
	// Store keeps the choices; Settings are the ones loaded at start.
	Store    *settings.Store
	Settings settings.Settings
	// ProfileFiles are the AWS configuration files the profiles come from.
	ProfileFiles profiles.Files
	// Check asks AWS who a profile's credentials belong to. When nil, the AWS SDK does,
	// with the profile from ProfileFiles; tests set a fake.
	Check func(ctx context.Context, profile, region string) identity.Result
}

// DeckService watches the decks folder, keeps the profile chosen for each deck file, and
// gives the window its content. It is the only place that turns the Go core's decks and
// profiles into what the window shows. Its exported methods other than the service
// lifecycle are what the window may call.
type DeckService struct {
	folder       string
	notice       string
	store        *settings.Store
	profileFiles profiles.Files
	check        func(ctx context.Context, profile, region string) identity.Result
	// emit sends a new snapshot to the window; main sets it once the app exists.
	emit func(Snapshot)

	// choosing serializes ChooseProfile, which reads, changes and saves the settings.
	choosing sync.Mutex

	mu        sync.Mutex
	settings  settings.Settings
	files     []deck.File
	folderErr error
	snapshot  Snapshot
	stop      context.CancelFunc
}

// NewDeckService returns a service with the given options.
func NewDeckService(options DeckServiceOptions) *DeckService {
	current := options.Settings
	if current.Profiles == nil {
		current = settings.Defaults()
	}
	check := options.Check
	if check == nil {
		check = sdkCheck(options.ProfileFiles)
	}
	return &DeckService{
		folder:       options.Folder,
		notice:       options.Notice,
		store:        options.Store,
		profileFiles: options.ProfileFiles,
		check:        check,
		settings:     current,
		snapshot:     Snapshot{Folder: options.Folder, Decks: []DeckFile{}, Notice: options.Notice},
	}
}

// Snapshot returns what the decks folder holds now.
func (s *DeckService) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

// Profiles lists the AWS profiles of this computer by name and kind, read fresh from the
// AWS configuration files each time.
func (s *DeckService) Profiles() ProfileList {
	list := ProfileList{Profiles: []ProfileView{}}
	result, err := profiles.Discover(s.profileFiles)
	if err != nil {
		list.Error = err.Error()
		return list
	}
	for _, profile := range result.Profiles {
		list.Profiles = append(list.Profiles, ProfileView{Name: profile.Name, Kind: string(profile.Kind)})
	}
	return list
}

// ChooseProfile maps a loaded deck file to one of this computer's AWS profiles, or
// removes the mapping when profile is empty. The choice is kept in the settings, never
// in the deck file, and the window gets a new snapshot.
func (s *DeckService) ChooseProfile(deckFile, profile string) error {
	s.choosing.Lock()
	defer s.choosing.Unlock()

	if profile != "" {
		known, err := s.knownProfiles()
		if err != nil {
			return fmt.Errorf("the AWS configuration files cannot be read: %w", err)
		}
		if !known[profile] {
			return fmt.Errorf("the profile %q is not in your AWS configuration files", profile)
		}
	}

	s.mu.Lock()
	if !s.loaded(deckFile) {
		s.mu.Unlock()
		return fmt.Errorf("the deck file %q is not loaded", deckFile)
	}
	next := settings.Settings{Version: s.settings.Version, Profiles: make(map[string]string, len(s.settings.Profiles)+1)}
	for file, chosen := range s.settings.Profiles {
		next.Profiles[file] = chosen
	}
	s.mu.Unlock()

	if profile == "" {
		delete(next.Profiles, deckFile)
	} else {
		next.Profiles[deckFile] = profile
	}
	if err := s.store.Save(next); err != nil {
		return fmt.Errorf("the choice could not be saved: %w", err)
	}

	s.mu.Lock()
	s.settings = next
	s.mu.Unlock()
	s.refresh()
	return nil
}

// CheckProfile asks AWS who the credentials of one of this computer's profiles belong
// to, with sts:GetCallerIdentity, which needs no permission and changes nothing. region
// is used when the profile sets none; an empty or malformed region falls back to
// us-east-1. The credentials stay in memory for the check only.
func (s *DeckService) CheckProfile(profile, region string) IdentityView {
	known, err := s.knownProfiles()
	if err != nil {
		return IdentityView{State: string(identity.Failed), Reason: "The AWS configuration files cannot be read: " + err.Error()}
	}
	if !known[profile] {
		return IdentityView{State: string(identity.Failed), Reason: fmt.Sprintf("The profile %s is not in your AWS configuration files.", profile)}
	}
	if !regionPattern.MatchString(region) {
		region = fallbackRegion
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	result := s.check(ctx, profile, region)
	return IdentityView{State: string(result.State), Account: result.Account, Caller: result.Caller, Reason: result.Reason}
}

// sdkCheck checks a profile with the AWS SDK: it loads the profile from the AWS files
// and asks STS who the caller is.
func sdkCheck(files profiles.Files) func(ctx context.Context, profile, region string) identity.Result {
	return func(ctx context.Context, profile, region string) identity.Result {
		cfg, err := awssdk.LoadProfile(ctx, profile, files, region)
		if err != nil {
			return identity.FromError(err)
		}
		return identity.Checker{}.Check(ctx, awssdk.NewSTS(cfg))
	}
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

// update takes the deck files Watch reports and refreshes the window.
func (s *DeckService) update(files []deck.File, err error) {
	s.mu.Lock()
	s.files = files
	s.folderErr = err
	s.mu.Unlock()
	s.refresh()
}

// refresh rebuilds the snapshot from the latest deck files and choices, and sends it.
func (s *DeckService) refresh() {
	// When the AWS files cannot be read, known stays nil and a chosen profile is not
	// flagged as missing; the chooser shows the error instead.
	known, _ := s.knownProfiles()
	s.mu.Lock()
	snapshot := snapshotOf(s.folder, s.files, s.folderErr, s.settings.Profiles, known)
	snapshot.Notice = s.notice
	s.snapshot = snapshot
	emit := s.emit
	s.mu.Unlock()
	if emit != nil {
		emit(snapshot)
	}
}

func (s *DeckService) knownProfiles() (map[string]bool, error) {
	result, err := profiles.Discover(s.profileFiles)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(result.Profiles))
	for _, profile := range result.Profiles {
		known[profile.Name] = true
	}
	return known, nil
}

// loaded reports whether deckFile is a deck file that loaded. The caller holds s.mu.
func (s *DeckService) loaded(deckFile string) bool {
	for _, file := range s.files {
		if file.Name == deckFile && file.Err == nil {
			return true
		}
	}
	return false
}

// snapshotOf turns the files of the decks folder and the chosen profiles into what the
// window shows. A tunnel is idle until the tunnel engine (M04) exists; its reason says
// when no profile is chosen or the chosen one is missing. known is nil when the AWS
// files could not be read.
func snapshotOf(folder string, files []deck.File, err error, chosen map[string]string, known map[string]bool) Snapshot {
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
		view.Profile = chosen[file.Name]
		view.SuggestedProfile = file.Deck.Profile

		var profile *string
		reason := ""
		switch {
		case view.Profile == "":
			reason = reasonNoProfile
		case known != nil && !known[view.Profile]:
			name := view.Profile
			profile = &name
			reason = fmt.Sprintf("The AWS profile %s chosen for this deck file is not in your AWS configuration files.", view.Profile)
		default:
			name := view.Profile
			profile = &name
		}
		for _, tunnel := range file.Deck.Tunnels {
			view.Tunnels = append(view.Tunnels, TunnelView{
				ID:         file.Name + "/" + tunnel.Name,
				Name:       tunnel.Name,
				Deck:       file.Deck.Name,
				DeckFile:   file.Name,
				Region:     tunnel.Region,
				JumpHost:   jumpHostText(tunnel.JumpHost),
				RemoteHost: tunnel.Remote.Host,
				RemotePort: tunnel.Remote.Port,
				LocalPort:  tunnel.LocalPort,
				Profile:    profile,
				State:      "idle",
				Reason:     reason,
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
