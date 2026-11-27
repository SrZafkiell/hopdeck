package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/SrZafkiell/hopdeck/internal/deck"
	"github.com/SrZafkiell/hopdeck/internal/identity"
	"github.com/SrZafkiell/hopdeck/internal/profiles"
	"github.com/SrZafkiell/hopdeck/internal/settings"
)

const goodDeck = `version: 1
name: Team databases
profile: team-dev
region: us-east-1
tunnels:
  - name: orders-db
    jump_host:
      instance_id: i-1234567890abcdef0
    remote:
      host: orders.db.example.internal
      port: 3306
    local_port: 13306
  - name: web
    jump_host:
      tags:
        Role: jump-host
        Env: test
    remote:
      port: 8080
    local_port: 18080
`

// Example AWS files: the documented example access key, and made-up profile names.
const (
	awsConfig      = "[profile example-dev]\nregion = us-east-1\n\n[profile other]\nsso_session = corp\n"
	awsCredentials = "[example-dev]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n"
)

func parsedFiles(t *testing.T) []deck.File {
	t.Helper()
	good, err := deck.Parse("team.hopdeck.yaml", []byte(goodDeck))
	if err != nil {
		t.Fatal(err)
	}
	_, brokenErr := deck.Parse("broken.hopdeck.yaml", []byte("version: 2\n"))
	return []deck.File{
		{Name: "broken.hopdeck.yaml", Err: brokenErr},
		{Name: "team.hopdeck.yaml", Deck: good},
	}
}

func TestSnapshotOf(t *testing.T) {
	snapshot := snapshotOf("C:/decks", parsedFiles(t), nil, settings.Defaults(), nil)
	if snapshot.Folder != "C:/decks" || snapshot.Error != "" || len(snapshot.Decks) != 2 {
		t.Fatalf("snapshot: %+v", snapshot)
	}

	broken := snapshot.Decks[0]
	if broken.Name != "broken.hopdeck.yaml" || len(broken.Tunnels) != 0 || len(broken.Problems) != 1 ||
		broken.Problems[0].Line != 1 || broken.Problems[0].Field != "version" {
		t.Errorf("broken file: %+v", broken)
	}

	team := snapshot.Decks[1]
	if team.Name != "Team databases" || team.SuggestedProfile != "team-dev" || team.Profile != "" ||
		len(team.Problems) != 0 || len(team.Tunnels) != 2 {
		t.Fatalf("good file: %+v", team)
	}
	orders, web := team.Tunnels[0], team.Tunnels[1]
	if orders.ID != "team.hopdeck.yaml/orders-db" || orders.Deck != "Team databases" || orders.DeckFile != "team.hopdeck.yaml" ||
		orders.JumpHost != "i-1234567890abcdef0" || orders.RemoteHost != "orders.db.example.internal" ||
		orders.RemotePort != 3306 || orders.LocalPort != 13306 || orders.DeckPort != 13306 || orders.State != "idle" || orders.Profile != nil {
		t.Errorf("orders-db: %+v", orders)
	}
	if orders.Reason != reasonNoProfile {
		t.Errorf("a tunnel without a profile must say so: %q", orders.Reason)
	}
	if web.JumpHost != "tag:Env=test, tag:Role=jump-host" || web.RemoteHost != "" || web.Region != "us-east-1" {
		t.Errorf("web: %+v", web)
	}
}

func TestSnapshotOfAppliesTheChosenProfile(t *testing.T) {
	chosen := settings.Defaults()
	chosen.Profiles["team.hopdeck.yaml"] = "example-dev"

	snapshot := snapshotOf("C:/decks", parsedFiles(t), nil, chosen, map[string]bool{"example-dev": true})
	team := snapshot.Decks[1]
	for _, tunnel := range team.Tunnels {
		if tunnel.Profile == nil || *tunnel.Profile != "example-dev" || tunnel.Reason != "" {
			t.Errorf("%s: profile %v, reason %q", tunnel.Name, tunnel.Profile, tunnel.Reason)
		}
	}
	if team.Profile != "example-dev" {
		t.Errorf("deck profile: %q", team.Profile)
	}

	missing := snapshotOf("C:/decks", parsedFiles(t), nil, chosen, map[string]bool{"other": true})
	tunnel := missing.Decks[1].Tunnels[0]
	if tunnel.Profile == nil || *tunnel.Profile != "example-dev" ||
		!strings.Contains(tunnel.Reason, "example-dev") || !strings.Contains(tunnel.Reason, "not in your AWS configuration files") {
		t.Errorf("a missing profile: %v, %q", tunnel.Profile, tunnel.Reason)
	}

	unknown := snapshotOf("C:/decks", parsedFiles(t), nil, chosen, nil)
	if reason := unknown.Decks[1].Tunnels[0].Reason; reason != "" {
		t.Errorf("unreadable AWS files must not flag the profile as missing: %q", reason)
	}
}

func TestSnapshotOfAppliesTheChosenPort(t *testing.T) {
	chosen := settings.Defaults()
	chosen.Ports["team.hopdeck.yaml"] = map[string]int{"orders-db": 13307}
	chosen.Ports["broken.hopdeck.yaml"] = map[string]int{"orders-db": 13308}

	tunnels := snapshotOf("C:/decks", parsedFiles(t), nil, chosen, nil).Decks[1].Tunnels
	if orders := tunnels[0]; orders.LocalPort != 13307 || orders.DeckPort != 13306 {
		t.Errorf("orders-db: local %d, deck %d", orders.LocalPort, orders.DeckPort)
	}
	if web := tunnels[1]; web.LocalPort != 18080 || web.DeckPort != 18080 {
		t.Errorf("web: local %d, deck %d", web.LocalPort, web.DeckPort)
	}
}

func TestSnapshotOfAnUnreadableFolder(t *testing.T) {
	snapshot := snapshotOf("C:/decks", nil, errors.New("access is denied"), settings.Defaults(), nil)
	if snapshot.Error != "access is denied" || snapshot.Decks == nil || len(snapshot.Decks) != 0 {
		t.Fatalf("snapshot: %+v", snapshot)
	}
}

func TestSnapshotOfAPlainError(t *testing.T) {
	snapshot := snapshotOf("C:/decks", []deck.File{{Name: "x.hopdeck.yaml", Err: errors.New("permission denied")}}, nil, settings.Defaults(), nil)
	problems := snapshot.Decks[0].Problems
	if len(problems) != 1 || problems[0].Text != "permission denied" || problems[0].Line != 0 {
		t.Fatalf("problems: %+v", problems)
	}
}

func TestNoticeStaysInEverySnapshot(t *testing.T) {
	service := NewDeckService(DeckServiceOptions{Folder: "C:/decks", Notice: "The settings file could not be used."})
	if got := service.Snapshot().Notice; got != "The settings file could not be used." {
		t.Fatalf("first snapshot: %q", got)
	}
	var emitted Snapshot
	service.emit = func(snapshot Snapshot) { emitted = snapshot }
	service.update(nil, nil)
	if service.Snapshot().Notice == "" || emitted.Notice == "" {
		t.Fatalf("the notice was lost on update: %+v", emitted)
	}
}

// fixture builds a service over a real decks folder, real example AWS files and a real
// settings file in a temporary folder.
type fixture struct {
	service      *DeckService
	deckPath     string
	settingsPath string
	emitted      []Snapshot
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	decks := filepath.Join(dir, "decks")
	if err := os.Mkdir(decks, 0o700); err != nil {
		t.Fatal(err)
	}
	f := &fixture{deckPath: filepath.Join(decks, "team.hopdeck.yaml"), settingsPath: filepath.Join(dir, "settings.json")}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(f.deckPath, goodDeck)
	awsFiles := profiles.Files{Config: filepath.Join(dir, "config"), Credentials: filepath.Join(dir, "credentials")}
	write(awsFiles.Config, awsConfig)
	write(awsFiles.Credentials, awsCredentials)

	f.service = NewDeckService(DeckServiceOptions{
		Folder:       decks,
		Store:        settings.NewStore(f.settingsPath),
		Settings:     settings.Defaults(),
		ProfileFiles: awsFiles,
	})
	f.service.emit = func(snapshot Snapshot) { f.emitted = append(f.emitted, snapshot) }
	f.service.update(deck.ReadFolder(decks))
	return f
}

func (f *fixture) savedChoices(t *testing.T) map[string]string {
	t.Helper()
	return f.saved(t).Profiles
}

func (f *fixture) saved(t *testing.T) settings.Settings {
	t.Helper()
	loaded, notice := settings.NewStore(f.settingsPath).Load()
	if notice != "" {
		t.Fatalf("settings: %s", notice)
	}
	return loaded
}

// tunnelView returns a tunnel of the team deck file from the service's latest snapshot.
func (f *fixture) tunnelView(t *testing.T, name string) TunnelView {
	t.Helper()
	for _, tunnel := range f.service.Snapshot().Decks[0].Tunnels {
		if tunnel.Name == name {
			return tunnel
		}
	}
	t.Fatalf("no tunnel %s", name)
	return TunnelView{}
}

func TestChooseProfile(t *testing.T) {
	f := newFixture(t)
	deckBefore, _ := os.ReadFile(f.deckPath)

	if err := f.service.ChooseProfile("team.hopdeck.yaml", "example-dev"); err != nil {
		t.Fatalf("ChooseProfile: %v", err)
	}
	last := f.emitted[len(f.emitted)-1]
	if p := last.Decks[0].Tunnels[0].Profile; p == nil || *p != "example-dev" || last.Decks[0].Tunnels[0].Reason != "" {
		t.Fatalf("the window did not get the choice: %+v", last.Decks[0])
	}
	if got := f.savedChoices(t)["team.hopdeck.yaml"]; got != "example-dev" {
		t.Fatalf("the settings file holds %q", got)
	}
	if deckAfter, _ := os.ReadFile(f.deckPath); string(deckAfter) != string(deckBefore) {
		t.Fatal("the deck file was changed")
	}

	if err := f.service.ChooseProfile("team.hopdeck.yaml", ""); err != nil {
		t.Fatalf("clearing the choice: %v", err)
	}
	if _, ok := f.savedChoices(t)["team.hopdeck.yaml"]; ok {
		t.Fatal("the cleared choice is still saved")
	}
	if reason := f.service.Snapshot().Decks[0].Tunnels[0].Reason; reason != reasonNoProfile {
		t.Fatalf("after clearing: %q", reason)
	}
}

func TestChooseProfileRefuses(t *testing.T) {
	f := newFixture(t)
	cases := map[string][2]string{
		"a profile this computer does not have": {"team.hopdeck.yaml", "not-here"},
		"a deck file that is not loaded":        {"missing.hopdeck.yaml", "example-dev"},
	}
	for name, args := range cases {
		if err := f.service.ChooseProfile(args[0], args[1]); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if len(f.savedChoices(t)) != 0 {
		t.Fatalf("a refused choice was saved: %v", f.savedChoices(t))
	}
}

func TestProfilesListsNamesAndKindsOnly(t *testing.T) {
	f := newFixture(t)
	list := f.service.Profiles()
	if list.Error != "" || len(list.Profiles) != 2 ||
		list.Profiles[0] != (ProfileView{Name: "example-dev", Kind: "keys"}) ||
		list.Profiles[1] != (ProfileView{Name: "other", Kind: "identity-center"}) {
		t.Fatalf("profiles: %+v", list)
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "AKIAIOSFODNN7EXAMPLE") || strings.Contains(string(data), "wJalrXUtnFEMI") {
		t.Fatalf("the list carries a credential: %s", data)
	}
}

func TestCheckProfile(t *testing.T) {
	f := newFixture(t)
	var asked []string
	f.service.check = func(_ context.Context, profile, region string) identity.Result {
		asked = append(asked, profile+"@"+region)
		return identity.Result{State: identity.Valid, Account: "123456789012", Caller: "arn:aws:iam::123456789012:user/example-user"}
	}

	view := f.service.CheckProfile("example-dev", "eu-west-1")
	if view.State != "valid" || view.Account != "123456789012" || view.Caller == "" || view.Reason != "" {
		t.Fatalf("got %+v", view)
	}
	f.service.CheckProfile("example-dev", "not a region")
	if len(asked) != 2 || asked[0] != "example-dev@eu-west-1" || asked[1] != "example-dev@"+fallbackRegion {
		t.Fatalf("asked %v", asked)
	}

	unknown := f.service.CheckProfile("not-here", "us-east-1")
	if unknown.State != "error" || !strings.Contains(unknown.Reason, "not-here") || len(asked) != 2 {
		t.Fatalf("an unknown profile: %+v, asked %v", unknown, asked)
	}
}

func TestCheckProfilePassesTheStateOn(t *testing.T) {
	f := newFixture(t)
	f.service.check = func(context.Context, string, string) identity.Result {
		return identity.Result{State: identity.NeedsSignIn, Reason: "The IAM Identity Center sign-in of this profile has expired or has not happened yet."}
	}
	view := f.service.CheckProfile("other", "us-east-1")
	if view.State != "needs-sign-in" || view.Account != "" || !strings.Contains(view.Reason, "sign-in") {
		t.Fatalf("got %+v", view)
	}
}

func TestChoosePort(t *testing.T) {
	f := newFixture(t)
	deckBefore, _ := os.ReadFile(f.deckPath)

	if err := f.service.ChoosePort("team.hopdeck.yaml", "orders-db", 13307); err != nil {
		t.Fatalf("ChoosePort: %v", err)
	}
	last := f.emitted[len(f.emitted)-1].Decks[0].Tunnels[0]
	if last.Name != "orders-db" || last.LocalPort != 13307 || last.DeckPort != 13306 {
		t.Fatalf("the window did not get the choice: %+v", last)
	}
	if got := f.saved(t).Ports["team.hopdeck.yaml"]["orders-db"]; got != 13307 {
		t.Fatalf("the settings file holds %d", got)
	}
	if deckAfter, _ := os.ReadFile(f.deckPath); string(deckAfter) != string(deckBefore) {
		t.Fatal("the deck file was changed")
	}

	// Choosing a profile afterwards keeps the port.
	if err := f.service.ChooseProfile("team.hopdeck.yaml", "example-dev"); err != nil {
		t.Fatal(err)
	}
	if saved := f.saved(t); saved.Ports["team.hopdeck.yaml"]["orders-db"] != 13307 || saved.Profiles["team.hopdeck.yaml"] != "example-dev" {
		t.Fatalf("a profile choice lost the port: %+v", saved)
	}

	// The deck file's own port, and 0, go back to the deck file's.
	for _, back := range []int{13306, 0} {
		if err := f.service.ChoosePort("team.hopdeck.yaml", "orders-db", 13309); err != nil {
			t.Fatal(err)
		}
		if err := f.service.ChoosePort("team.hopdeck.yaml", "orders-db", back); err != nil {
			t.Fatalf("going back with %d: %v", back, err)
		}
		if ports := f.saved(t).Ports; len(ports) != 0 {
			t.Fatalf("going back with %d left %v", back, ports)
		}
		if orders := f.tunnelView(t, "orders-db"); orders.LocalPort != 13306 {
			t.Fatalf("going back with %d: local port %d", back, orders.LocalPort)
		}
	}
}

func TestChoosePortRefuses(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name, file, tunnel string
		port               int
		says               string
	}{
		{"a port below 1024", "team.hopdeck.yaml", "orders-db", 80, "between 1024 and 65535"},
		{"a port above 65535", "team.hopdeck.yaml", "orders-db", 70000, "between 1024 and 65535"},
		{"the port of another tunnel", "team.hopdeck.yaml", "orders-db", 18080, "the tunnel web of the deck Team databases"},
		{"a tunnel that does not exist", "team.hopdeck.yaml", "nope", 13307, "not in a loaded deck file"},
		{"a deck file that is not loaded", "missing.hopdeck.yaml", "orders-db", 13307, "not in a loaded deck file"},
	}
	for _, tc := range cases {
		err := f.service.ChoosePort(tc.file, tc.tunnel, tc.port)
		if err == nil || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	if ports := f.saved(t).Ports; len(ports) != 0 {
		t.Fatalf("a refused choice was saved: %v", ports)
	}

	// A port chosen for one tunnel is refused for another one.
	if err := f.service.ChoosePort("team.hopdeck.yaml", "orders-db", 13307); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ChoosePort("team.hopdeck.yaml", "web", 13307); err == nil || !strings.Contains(err.Error(), "orders-db") {
		t.Fatalf("a port chosen for orders-db was given to web: %v", err)
	}
}

// TestCheckPort holds a real port on 127.0.0.1, moves a tunnel onto it and checks it.
func TestCheckPort(t *testing.T) {
	f := newFixture(t)
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port

	if err := f.service.ChoosePort("team.hopdeck.yaml", "orders-db", port); err != nil {
		t.Fatal(err)
	}
	// The next port belongs to the other tunnel, so it is never suggested.
	if err := f.service.ChoosePort("team.hopdeck.yaml", "web", port+1); err != nil {
		t.Fatal(err)
	}

	busy := f.service.CheckPort("team.hopdeck.yaml", "orders-db")
	if busy.Free || busy.Port != port || !strings.Contains(busy.Reason, "Port "+strconv.Itoa(port)+" ") {
		t.Fatalf("a held port: %+v", busy)
	}
	if busy.Suggestion <= port+1 || busy.Suggestion > port+20 {
		t.Fatalf("suggestion %d for %d", busy.Suggestion, port)
	}

	held.Close()
	free := f.service.CheckPort("team.hopdeck.yaml", "orders-db")
	if !free.Free || free.Port != port || free.Reason != "" || free.Suggestion != 0 {
		t.Fatalf("a released port: %+v", free)
	}

	unknown := f.service.CheckPort("team.hopdeck.yaml", "nope")
	if unknown.Free || !strings.Contains(unknown.Reason, "nope") {
		t.Fatalf("an unknown tunnel: %+v", unknown)
	}
}
