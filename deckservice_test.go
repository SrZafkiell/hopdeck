package main

import (
	"errors"
	"testing"

	"github.com/SrZafkiell/hopdeck/internal/deck"
)

const goodDeck = `version: 1
name: Team databases
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

func TestSnapshotOf(t *testing.T) {
	good, err := deck.Parse("team.hopdeck.yaml", []byte(goodDeck))
	if err != nil {
		t.Fatal(err)
	}
	_, brokenErr := deck.Parse("broken.hopdeck.yaml", []byte("version: 2\n"))
	files := []deck.File{
		{Name: "broken.hopdeck.yaml", Err: brokenErr},
		{Name: "team.hopdeck.yaml", Deck: good},
	}

	snapshot := snapshotOf("C:/decks", files, nil)
	if snapshot.Folder != "C:/decks" || snapshot.Error != "" || len(snapshot.Decks) != 2 {
		t.Fatalf("snapshot: %+v", snapshot)
	}

	broken := snapshot.Decks[0]
	if broken.Name != "broken.hopdeck.yaml" || len(broken.Tunnels) != 0 || len(broken.Problems) != 1 ||
		broken.Problems[0].Line != 1 || broken.Problems[0].Field != "version" {
		t.Errorf("broken file: %+v", broken)
	}

	team := snapshot.Decks[1]
	if team.Name != "Team databases" || len(team.Problems) != 0 || len(team.Tunnels) != 2 {
		t.Fatalf("good file: %+v", team)
	}
	orders, web := team.Tunnels[0], team.Tunnels[1]
	if orders.ID != "team.hopdeck.yaml/orders-db" || orders.Deck != "Team databases" ||
		orders.JumpHost != "i-1234567890abcdef0" || orders.RemoteHost != "orders.db.example.internal" ||
		orders.RemotePort != 3306 || orders.LocalPort != 13306 || orders.State != "idle" || orders.Profile != nil {
		t.Errorf("orders-db: %+v", orders)
	}
	if web.JumpHost != "tag:Env=test, tag:Role=jump-host" || web.RemoteHost != "" || web.Region != "us-east-1" {
		t.Errorf("web: %+v", web)
	}
}

func TestSnapshotOfAnUnreadableFolder(t *testing.T) {
	snapshot := snapshotOf("C:/decks", nil, errors.New("access is denied"))
	if snapshot.Error != "access is denied" || snapshot.Decks == nil || len(snapshot.Decks) != 0 {
		t.Fatalf("snapshot: %+v", snapshot)
	}
}

func TestSnapshotOfAPlainError(t *testing.T) {
	snapshot := snapshotOf("C:/decks", []deck.File{{Name: "x.hopdeck.yaml", Err: errors.New("permission denied")}}, nil)
	problems := snapshot.Decks[0].Problems
	if len(problems) != 1 || problems[0].Text != "permission denied" || problems[0].Line != 0 {
		t.Fatalf("problems: %+v", problems)
	}
}
