// Package deck reads deck files: the small YAML files in which an administrator
// describes the tunnels of a team (format: ADR 0004 in the clouddecks repository).
//
// A deck file comes from someone else, so the parser is strict. It refuses anything the
// format does not define, reports every problem with the file, the line and the field,
// and never repeats a value in a message, because a value in the wrong place may be a
// secret.
package deck

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// NewestVersion is the newest format version this build reads.
	NewestVersion = 1
	// MaxSize is the largest deck file accepted, in bytes.
	MaxSize = 64 << 10
	// MaxTunnels is the largest number of tunnels one deck file may describe.
	MaxTunnels = 100
	// Suffix ends the name of every deck file; other files in the decks folder are
	// ignored.
	Suffix = ".hopdeck.yaml"
)

// The Session Manager documents a tunnel uses.
const (
	// DocumentRemoteHost forwards to a host behind the jump host, such as a database.
	DocumentRemoteHost = "AWS-StartPortForwardingSessionToRemoteHost"
	// DocumentJumpHost forwards to a port on the jump host itself.
	DocumentJumpHost = "AWS-StartPortForwardingSession"
)

// Deck is a deck file that passed every check.
type Deck struct {
	Version int
	Name    string
	// Profile is the AWS profile the file suggests; each person maps the deck to a
	// profile of their own. Empty when the file suggests none.
	Profile string
	// Region is the file's default region; empty when every tunnel names its own.
	Region  string
	Tunnels []Tunnel
}

// Tunnel is one port forward of a deck file.
type Tunnel struct {
	Name string
	// Region is the tunnel's own region, or the file's default.
	Region   string
	JumpHost JumpHost
	Remote   Remote
	// LocalPort is the port on 127.0.0.1 where the tunnel listens.
	LocalPort int
}

// JumpHost finds the instance a tunnel goes through: by its ID, or by tags.
type JumpHost struct {
	InstanceID string
	Tags       map[string]string
}

// Remote is where a tunnel ends.
type Remote struct {
	// Host is a DNS name or an IPv4 address behind the jump host. Empty means a port
	// on the jump host itself.
	Host string
	Port int
}

// Document returns the Session Manager document the tunnel needs.
func (t Tunnel) Document() string {
	if t.Remote.Host == "" {
		return DocumentJumpHost
	}
	return DocumentRemoteHost
}

// Problem is one thing wrong with a deck file.
type Problem struct {
	// Line is the line of the file, counted from 1; 0 when the problem concerns the
	// whole file.
	Line int
	// Field is the path of the field, such as tunnels[2].remote.port, with tunnels
	// counted from 1; empty when the problem concerns the whole file.
	Field string
	Text  string
}

// Error lists the problems that make a deck file unusable.
type Error struct {
	File     string
	Problems []Problem
}

func (e *Error) Error() string {
	lines := make([]string, 0, len(e.Problems))
	for _, problem := range e.Problems {
		var b strings.Builder
		b.WriteString(e.File)
		if problem.Line > 0 {
			fmt.Fprintf(&b, ":%d", problem.Line)
		}
		if problem.Field != "" {
			b.WriteString(": " + problem.Field)
		}
		b.WriteString(": " + problem.Text)
		lines = append(lines, b.String())
	}
	return strings.Join(lines, "\n")
}

// Parse checks the content of one deck file. The name is only used in messages. When
// anything is wrong, the error is an *Error with every problem found.
func Parse(name string, data []byte) (*Deck, error) {
	c := &checker{}
	deck := c.file(data)
	if len(c.problems) > 0 {
		return nil, &Error{File: name, Problems: c.problems}
	}
	return deck, nil
}

// Load reads and checks a deck file. It reads at most one byte more than MaxSize, so a
// huge file is refused without being read whole.
func Load(path string) (*Deck, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxSize+1))
	if err != nil {
		return nil, err
	}
	return Parse(filepath.Base(path), data)
}
