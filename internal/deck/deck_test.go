package deck

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// example is the deck file of ADR 0004, on made-up data.
const example = `version: 1
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
  - name: reports-db
    region: eu-west-1
    jump_host:
      tags:
        Role: jump-host
    remote:
      host: reports.db.example.internal
      port: 5432
    local_port: 15432
  - name: jump-host-web
    jump_host:
      tags:
        Role: jump-host
    remote:
      port: 8080
    local_port: 18080
`

// anyLine in a case accepts a problem on any line of the file.
const anyLine = -1

// minimal is the smallest valid file; the problem cases below edit it, so its line
// numbers are the ones the cases expect.
const minimal = `version: 1
name: Example
region: us-east-1
tunnels:
  - name: db
    jump_host:
      instance_id: i-1234567890abcdef0
    remote:
      host: db.example.internal
      port: 5432
    local_port: 15432
`

func TestParseExample(t *testing.T) {
	deck, err := Parse("team.hopdeck.yaml", []byte(example))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if deck.Version != 1 || deck.Name != "Team databases" || deck.Profile != "team-dev" || deck.Region != "us-east-1" {
		t.Fatalf("file fields: %+v", deck)
	}
	if len(deck.Tunnels) != 3 {
		t.Fatalf("got %d tunnels, want 3", len(deck.Tunnels))
	}

	orders, reports, web := deck.Tunnels[0], deck.Tunnels[1], deck.Tunnels[2]
	if orders.Region != "us-east-1" || reports.Region != "eu-west-1" {
		t.Errorf("regions: default %q, override %q", orders.Region, reports.Region)
	}
	if orders.JumpHost.InstanceID != "i-1234567890abcdef0" || orders.JumpHost.Tags != nil {
		t.Errorf("orders jump host: %+v", orders.JumpHost)
	}
	if reports.JumpHost.Tags["Role"] != "jump-host" || reports.JumpHost.InstanceID != "" {
		t.Errorf("reports jump host: %+v", reports.JumpHost)
	}
	if orders.Remote != (Remote{Host: "orders.db.example.internal", Port: 3306}) || orders.LocalPort != 13306 {
		t.Errorf("orders remote %+v, local port %d", orders.Remote, orders.LocalPort)
	}
	if orders.Document() != DocumentRemoteHost || web.Document() != DocumentJumpHost {
		t.Errorf("documents: %s, %s", orders.Document(), web.Document())
	}
}

func TestParseMinimal(t *testing.T) {
	deck, err := Parse("minimal.hopdeck.yaml", []byte(minimal))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if deck.Profile != "" || len(deck.Tunnels) != 1 || deck.Tunnels[0].Region != "us-east-1" {
		t.Fatalf("got %+v", deck)
	}
}

func TestParseRefuses(t *testing.T) {
	cases := []struct {
		name  string
		input string
		line  int
		field string
		text  string
	}{
		{"unknown key in the file", minimal + "owner: ops\n", 12, "owner", "is not a field"},
		{"unknown key in a tunnel", edit(minimal, "    local_port: 15432\n", "    local_port: 15432\n    bind: 0.0.0.0\n"),
			12, "tunnels[1].bind", "is not a field"},
		{"port written as text", edit(minimal, "port: 5432", `port: "5432"`), 10, "tunnels[1].remote.port", "must be a whole number, not text"},
		{"port out of range", edit(minimal, "port: 5432", "port: 70000"), 10, "tunnels[1].remote.port", "between 1 and 65535"},
		{"local port under 1024", edit(minimal, "local_port: 15432", "local_port: 80"), 11, "tunnels[1].local_port", "between 1024 and 65535"},
		{"local port in hexadecimal", edit(minimal, "local_port: 15432", "local_port: 0x3C48"), 11, "tunnels[1].local_port", "must be a whole number"},
		{"duplicate key", edit(minimal, "name: Example\n", "name: Example\nname: Again\n"), 3, "name", "appears twice"},
		{"missing version", edit(minimal, "version: 1\n", ""), 1, "version", "is required"},
		{"version as text", edit(minimal, "version: 1", `version: "1"`), 1, "version", "must be a whole number"},
		{"newer version", edit(minimal, "version: 1", "version: 2"), 1, "version", "update Hopdeck"},
		{"missing tunnels", strings.SplitAfter(minimal, "region: us-east-1\n")[0], 1, "tunnels", "is required"},
		{"empty tunnel list", strings.SplitAfter(minimal, "region: us-east-1\n")[0] + "tunnels: []\n", 4, "tunnels", "between 1 and 100"},
		{"tunnels not a list", strings.SplitAfter(minimal, "region: us-east-1\n")[0] + "tunnels: db\n", 4, "tunnels", "must be a list"},
		{"no region anywhere", edit(minimal, "region: us-east-1\n", ""), 4, "tunnels[1].region", "the file sets no region"},
		{"bad region", edit(minimal, "region: us-east-1", "region: virginia"), 3, "region", "an AWS region"},
		{"bad tunnel name", edit(minimal, "name: db", "name: my db"), 5, "tunnels[1].name", "letters, digits"},
		{"bad instance ID", edit(minimal, "i-1234567890abcdef0", "i-12345"), 7, "tunnels[1].jump_host.instance_id", "an instance ID"},
		{"instance ID and tags", edit(minimal, "      instance_id: i-1234567890abcdef0\n", "      instance_id: i-1234567890abcdef0\n      tags:\n        Role: jump\n"),
			6, "tunnels[1].jump_host", "not both"},
		{"jump host without a target", edit(minimal, "      instance_id: i-1234567890abcdef0\n", "      {}\n"), 6, "tunnels[1].jump_host", "needs instance_id or tags"},
		{"jump host as text", edit(minimal, "    jump_host:\n      instance_id: i-1234567890abcdef0\n", "    jump_host: i-1234567890abcdef0\n"),
			6, "tunnels[1].jump_host", "must be a mapping"},
		{"empty tag value", edit(minimal, "      instance_id: i-1234567890abcdef0\n", "      tags:\n        Role:\n"),
			8, "tunnels[1].jump_host.tags.Role", "must be text, not an empty value"},
		{"IPv6 remote host", edit(minimal, "host: db.example.internal", `host: "2001:db8::1"`), 9, "tunnels[1].remote.host", "IPv4"},
		{"missing remote port", edit(minimal, "      port: 5432\n", ""), 8, "tunnels[1].remote.port", "is required"},
		{"duplicate tunnel name", minimal + "  - name: db\n    jump_host:\n      instance_id: i-1234567890abcdef0\n    remote:\n      port: 22\n    local_port: 10022\n",
			12, "tunnels[2].name", "repeats the name of the tunnel on line 5"},
		{"duplicate local port", minimal + "  - name: db2\n    jump_host:\n      instance_id: i-1234567890abcdef0\n    remote:\n      port: 22\n    local_port: 15432\n",
			17, "tunnels[2].local_port", "repeats the local port of the tunnel on line 11"},
		{"anchor", edit(minimal, "    jump_host:\n", "    jump_host: &jh\n"), 6, "", "anchors (&jh)"},
		{"alias", edit(minimal, "    jump_host:\n", "    jump_host: &jh\n") + "  - name: db2\n    jump_host: *jh\n    remote:\n      port: 22\n    local_port: 10022\n",
			13, "", "aliases (*jh)"},
		{"merge key", edit(minimal, "    local_port: 15432\n", "    local_port: 15432\n    <<: {name: other}\n"), 12, "", "merge keys"},
		{"explicit tag", edit(minimal, "port: 5432", "port: !!str 5432"), 10, "", "explicit tags"},
		{"nesting deeper than the format", edit(minimal, "      instance_id: i-1234567890abcdef0\n", "      tags:\n        Role: [[x]]\n"),
			8, "", "nest deeper"},
		{"second document", minimal + "---\nversion: 1\n", 12, "", "more than one YAML document"},
		{"not a mapping", "- version: 1\n", 1, "", "must be a mapping"},
		// The YAML library reports some syntax errors one line early, so any line will do.
		{"syntax error", "version: 1\nname: [unclosed\n", anyLine, "", "is not valid YAML"},
		{"empty file", "", 0, "", "the file is empty"},
		{"comments only", "# nothing here\n", 0, "", "the file is empty"},
		{"access key ID in a tag", edit(minimal, "      instance_id: i-1234567890abcdef0\n", "      tags:\n        Key: AKIAIOSFODNN7EXAMPLE\n"),
			8, "tunnels[1].jump_host.tags.Key", "looks like an AWS credential"},
		{"secret access key as a host", edit(minimal, "host: db.example.internal", "host: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
			9, "tunnels[1].remote.host", "looks like an AWS credential"},
		{"over the size limit", strings.Repeat("#", MaxSize+1), 0, "", "larger than"},
		{"not UTF-8", "name: \xff\xfe\n", 0, "", "not UTF-8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := refused(t, tc.input)
			for _, p := range problems {
				lineMatches := p.Line == tc.line || (tc.line == anyLine && p.Line > 0)
				if lineMatches && p.Field == tc.field && strings.Contains(p.Text, tc.text) {
					return
				}
			}
			t.Errorf("no problem at line %d, field %q, containing %q; got %+v", tc.line, tc.field, tc.text, problems)
		})
	}
}

func TestErrorNamesFileLineAndField(t *testing.T) {
	_, err := Parse("team.hopdeck.yaml", []byte(edit(minimal, "port: 5432", "port: 70000")))
	want := "team.hopdeck.yaml:10: tunnels[1].remote.port: must be between 1 and 65535"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func TestMessagesNeverRepeatACredential(t *testing.T) {
	secrets := []string{"AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	for _, secret := range secrets {
		inputs := []string{
			edit(minimal, "name: Example", "name: "+secret),
			edit(minimal, "host: db.example.internal", "host: "+secret),
			minimal + "secret: " + secret + "\n",
		}
		for _, input := range inputs {
			_, err := Parse("x.hopdeck.yaml", []byte(input))
			if err == nil {
				t.Fatalf("a file with %s in it was accepted", secret[:4])
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("the message repeats the credential: %v", err)
			}
		}
	}
}

func TestProblemsAreCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString(minimal)
	for i := 0; i < 50; i++ {
		b.WriteString("unknown" + strings.Repeat("x", i) + ": 1\n")
	}
	problems := refused(t, b.String())
	if len(problems) != maxProblems+1 || !strings.Contains(problems[maxProblems].Text, "more problems") {
		t.Fatalf("got %d problems, last %+v", len(problems), problems[len(problems)-1])
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "team"+Suffix)
	if err := os.WriteFile(good, []byte(minimal), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(good); err != nil {
		t.Fatalf("Load: %v", err)
	}

	huge := filepath.Join(dir, "huge"+Suffix)
	if err := os.WriteFile(huge, []byte(strings.Repeat("#", 4*MaxSize)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(huge)
	var deckErr *Error
	if !errors.As(err, &deckErr) || deckErr.File != "huge"+Suffix || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("got %v", err)
	}

	if _, err := Load(filepath.Join(dir, "missing"+Suffix)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: got %v", err)
	}
}

// refused parses input, expects an *Error and returns its problems.
func refused(t *testing.T, input string) []Problem {
	t.Helper()
	_, err := Parse("case.hopdeck.yaml", []byte(input))
	var deckErr *Error
	if !errors.As(err, &deckErr) {
		t.Fatalf("expected the file to be refused, got %v", err)
	}
	return deckErr.Problems
}

// edit replaces one exact piece of a test file, and fails loudly when the piece is not
// there exactly once, so a case cannot silently test the original file.
func edit(input, old, new string) string {
	if strings.Count(input, old) != 1 {
		panic("edit: " + old + " is not in the input exactly once")
	}
	return strings.Replace(input, old, new, 1)
}
