package deck

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	// maxDepth is how deeply lists and mappings may nest. The format needs five levels:
	// the file, the tunnel list, a tunnel, its jump_host or remote, and the tags.
	maxDepth = 5
	// maxProblems caps the problems reported for one file.
	maxProblems = 20
)

var (
	tunnelName    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	profileName   = regexp.MustCompile(`^[A-Za-z0-9._+@-]{1,64}$`)
	regionName    = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]{1,2}$`)
	instanceID    = regexp.MustCompile(`^i-([0-9a-f]{8}|[0-9a-f]{17})$`)
	hostLabel     = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	decimal       = regexp.MustCompile(`^[0-9]+$`)
	yamlLine      = regexp.MustCompile(`^yaml: line ([0-9]+): (.*)$`)
	accessKeyID   = regexp.MustCompile(`(AKIA|ASIA|ABIA|ACCA)[A-Z0-9]{16}`)
	secretLikeRun = regexp.MustCompile(`[A-Za-z0-9/+=]{40,}`)
)

// checker collects the problems of one file while reading it.
type checker struct {
	problems []Problem
	full     bool
}

// entry is one key of a mapping and its value. Problems about a whole value, such as a
// missing field inside jump_host, are reported on the line of its key, where a person
// looks for it.
type entry struct {
	key, value *yaml.Node
}

func (c *checker) add(line int, field, format string, args ...any) {
	if c.full {
		return
	}
	if len(c.problems) == maxProblems {
		c.problems = append(c.problems, Problem{Text: "more problems were found; fix these first"})
		c.full = true
		return
	}
	c.problems = append(c.problems, Problem{Line: line, Field: field, Text: fmt.Sprintf(format, args...)})
}

// file reads the YAML, refuses the features a deck file does not use, then reads the
// fields. It returns nil when it found a problem.
func (c *checker) file(data []byte) *Deck {
	if len(data) > MaxSize {
		c.add(0, "", "the file is larger than %d bytes, the limit for a deck file", MaxSize)
		return nil
	}
	if !utf8.Valid(data) {
		c.add(0, "", "the file is not UTF-8 text")
		return nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			c.add(0, "", "the file is empty")
		} else {
			c.syntax(err)
		}
		return nil
	}
	var next yaml.Node
	if err := decoder.Decode(&next); !errors.Is(err, io.EOF) {
		c.add(next.Line, "", "the file holds more than one YAML document; a deck file holds one")
		return nil
	}
	if !c.plain(&document, 0) {
		return nil
	}
	if len(document.Content) != 1 {
		c.add(document.Line, "", "the file is empty")
		return nil
	}
	return c.deck(document.Content[0])
}

func (c *checker) syntax(err error) {
	message := err.Error()
	if match := yamlLine.FindStringSubmatch(message); match != nil {
		line, _ := strconv.Atoi(match[1])
		c.add(line, "", "is not valid YAML: %s", match[2])
		return
	}
	c.add(0, "", "is not valid YAML: %s", strings.TrimPrefix(message, "yaml: "))
}

// plain refuses anchors, aliases, merge keys, explicit tags and deep nesting, so that
// the fields are only read from a plain tree. It returns false when it found any.
func (c *checker) plain(node *yaml.Node, depth int) bool {
	ok := true
	if node.Kind == yaml.AliasNode {
		c.add(node.Line, "", "aliases (*%s) are not allowed in a deck file", node.Value)
		return false
	}
	if node.Anchor != "" {
		c.add(node.Line, "", "anchors (&%s) are not allowed in a deck file", node.Anchor)
		ok = false
	}
	if node.Style&yaml.TaggedStyle != 0 {
		c.add(node.Line, "", "explicit tags (%s) are not allowed in a deck file", node.Tag)
		ok = false
	}
	if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
		depth++
		if depth > maxDepth {
			c.add(node.Line, "", "lists and mappings nest deeper than a deck file can")
			return false
		}
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if key := node.Content[i]; key.ShortTag() == "!!merge" {
				c.add(key.Line, "", "merge keys (<<) are not allowed in a deck file")
				ok = false
			}
		}
	}
	for _, child := range node.Content {
		if !c.plain(child, depth) {
			ok = false
		}
	}
	return ok
}

func (c *checker) deck(root *yaml.Node) *Deck {
	fields := c.mapping(root, root.Line, "", "version", "name", "profile", "region", "tunnels")
	if fields == nil {
		return nil
	}

	// The version says how the rest is read, so nothing else is checked when it is
	// missing, wrong or newer than this build.
	versionNode := c.required(fields, root.Line, "", "version")
	if versionNode == nil {
		return nil
	}
	version, ok := c.integer(versionNode, "version", 1, 1<<20)
	if !ok {
		return nil
	}
	if version > NewestVersion {
		c.add(versionNode.Line, "version",
			"the file uses format version %d and this Hopdeck reads up to version %d; update Hopdeck to open it",
			version, NewestVersion)
		return nil
	}

	deck := &Deck{Version: version}
	if node := c.required(fields, root.Line, "", "name"); node != nil {
		deck.Name = c.match(node, "name", validDeckName, "1 to 64 characters with no control characters")
	}
	if found, ok := fields["profile"]; ok {
		deck.Profile = c.match(found.value, "profile", profileName.MatchString,
			"an AWS profile name: 1 to 64 letters, digits, '.', '_', '+', '@' or '-'")
	}
	if found, ok := fields["region"]; ok {
		deck.Region = c.region(found.value, "region")
	}

	list := c.required(fields, root.Line, "", "tunnels")
	if list == nil {
		return deck
	}
	if list.Kind != yaml.SequenceNode {
		c.add(fields["tunnels"].key.Line, "tunnels", "must be a list of tunnels, not %s", describe(list))
		return deck
	}
	if count := len(list.Content); count == 0 || count > MaxTunnels {
		c.add(fields["tunnels"].key.Line, "tunnels", "must list between 1 and %d tunnels", MaxTunnels)
		return deck
	}

	names := map[string]int{}
	ports := map[int]int{}
	for i, item := range list.Content {
		field := fmt.Sprintf("tunnels[%d]", i+1)
		tunnel, nameLine, portLine := c.tunnel(item, field, deck.Region)
		if tunnel.Name != "" {
			if first, seen := names[tunnel.Name]; seen {
				c.add(nameLine, field+".name", "repeats the name of the tunnel on line %d", first)
			} else {
				names[tunnel.Name] = nameLine
			}
		}
		if tunnel.LocalPort != 0 {
			if first, seen := ports[tunnel.LocalPort]; seen {
				c.add(portLine, field+".local_port", "repeats the local port of the tunnel on line %d", first)
			} else {
				ports[tunnel.LocalPort] = portLine
			}
		}
		deck.Tunnels = append(deck.Tunnels, tunnel)
	}
	return deck
}

// tunnel reads one tunnel and returns it with the lines of its name and its local port,
// for the duplicate checks.
func (c *checker) tunnel(node *yaml.Node, field, defaultRegion string) (Tunnel, int, int) {
	var tunnel Tunnel
	fields := c.mapping(node, node.Line, field, "name", "region", "jump_host", "remote", "local_port")
	if fields == nil {
		return tunnel, 0, 0
	}

	var nameLine, portLine int
	if name := c.required(fields, node.Line, field, "name"); name != nil {
		nameLine = name.Line
		tunnel.Name = c.match(name, field+".name", tunnelName.MatchString,
			"1 to 64 letters, digits, '.', '_' or '-'")
	}

	tunnel.Region = defaultRegion
	if found, ok := fields["region"]; ok {
		tunnel.Region = c.region(found.value, field+".region")
	} else if defaultRegion == "" {
		c.add(node.Line, field+".region", "is required, because the file sets no region")
	}

	if found, ok := c.present(fields, node.Line, field, "jump_host"); ok {
		tunnel.JumpHost = c.jumpHost(found, field+".jump_host")
	}
	if found, ok := c.present(fields, node.Line, field, "remote"); ok {
		tunnel.Remote = c.remote(found, field+".remote")
	}
	if port := c.required(fields, node.Line, field, "local_port"); port != nil {
		portLine = port.Line
		tunnel.LocalPort, _ = c.integer(port, field+".local_port", MinLocalPort, MaxLocalPort)
	}
	return tunnel, nameLine, portLine
}

func (c *checker) jumpHost(at entry, field string) JumpHost {
	var jumpHost JumpHost
	fields := c.mapping(at.value, at.key.Line, field, "instance_id", "tags")
	if fields == nil {
		return jumpHost
	}
	id, hasID := fields["instance_id"]
	tags, hasTags := fields["tags"]
	switch {
	case !hasID && !hasTags:
		c.add(at.key.Line, field, "needs instance_id or tags")
	case hasID && hasTags:
		c.add(at.key.Line, field, "takes instance_id or tags, not both")
	case hasID:
		jumpHost.InstanceID = c.match(id.value, field+".instance_id", instanceID.MatchString,
			"an instance ID: i- followed by 8 or 17 hexadecimal digits")
	default:
		jumpHost.Tags = c.tags(tags, field+".tags")
	}
	return jumpHost
}

func (c *checker) tags(at entry, field string) map[string]string {
	node := at.value
	if node.Kind != yaml.MappingNode {
		c.add(at.key.Line, field, "must be a mapping of tag names to values, not %s", describe(node))
		return nil
	}
	count := len(node.Content) / 2
	if count == 0 || count > 10 {
		c.add(at.key.Line, field, "must hold between 1 and 10 tags")
		return nil
	}
	tags := make(map[string]string, count)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode, valueNode := node.Content[i], node.Content[i+1]
		key, ok := c.text(keyNode, field)
		if !ok {
			continue
		}
		path := field + "." + key
		if n := utf8.RuneCountInString(key); n == 0 || n > 128 {
			c.add(keyNode.Line, path, "a tag name must be 1 to 128 characters")
			continue
		}
		if _, seen := tags[key]; seen {
			c.add(keyNode.Line, path, "appears twice")
			continue
		}
		value, ok := c.text(valueNode, path)
		if !ok {
			continue
		}
		if utf8.RuneCountInString(value) > 256 {
			c.add(valueNode.Line, path, "a tag value must be at most 256 characters")
			continue
		}
		tags[key] = value
	}
	return tags
}

func (c *checker) remote(at entry, field string) Remote {
	var remote Remote
	fields := c.mapping(at.value, at.key.Line, field, "host", "port")
	if fields == nil {
		return remote
	}
	if found, ok := fields["host"]; ok {
		remote.Host = c.match(found.value, field+".host", validHost, "a DNS name or an IPv4 address")
	}
	if port := c.required(fields, at.key.Line, field, "port"); port != nil {
		remote.Port, _ = c.integer(port, field+".port", 1, 65535)
	}
	return remote
}

func (c *checker) region(node *yaml.Node, field string) string {
	return c.match(node, field, regionName.MatchString, "an AWS region such as us-east-1")
}

// mapping checks that node is a mapping that only holds the allowed keys, each once, and
// returns its entries by key. It returns nil when node is not a mapping; line is where
// that is reported.
func (c *checker) mapping(node *yaml.Node, line int, field string, allowed ...string) map[string]entry {
	if node.Kind != yaml.MappingNode {
		c.add(line, field, "must be a mapping of keys and values, not %s", describe(node))
		return nil
	}
	entries := make(map[string]entry, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			c.add(key.Line, field, "keys must be plain names, not %s", describe(key))
			continue
		}
		path := join(field, key.Value)
		_, seen := entries[key.Value]
		switch {
		case !contains(allowed, key.Value):
			c.add(key.Line, path, "is not a field of a deck file here (allowed: %s)", strings.Join(allowed, ", "))
		case seen:
			c.add(key.Line, path, "appears twice")
		default:
			entries[key.Value] = entry{key: key, value: value}
		}
	}
	return entries
}

// present returns the entry of a required key, or reports it missing on line.
func (c *checker) present(fields map[string]entry, line int, field, key string) (entry, bool) {
	found, ok := fields[key]
	if !ok {
		c.add(line, join(field, key), "is required")
	}
	return found, ok
}

// required returns the value of a required key, or reports it missing on line.
func (c *checker) required(fields map[string]entry, line int, field, key string) *yaml.Node {
	found, ok := c.present(fields, line, field, key)
	if !ok {
		return nil
	}
	return found.value
}

func (c *checker) integer(node *yaml.Node, field string, low, high int) (int, bool) {
	if node.Kind != yaml.ScalarNode || node.ShortTag() != "!!int" || !decimal.MatchString(node.Value) {
		c.add(node.Line, field, "must be a whole number, not %s", describe(node))
		return 0, false
	}
	value, err := strconv.Atoi(node.Value)
	if err != nil || value < low || value > high {
		c.add(node.Line, field, "must be between %d and %d", low, high)
		return 0, false
	}
	return value, true
}

// text reads a scalar as text and refuses anything that looks like a credential. The
// value itself never appears in a message.
func (c *checker) text(node *yaml.Node, field string) (string, bool) {
	if node.Kind != yaml.ScalarNode || node.ShortTag() == "!!null" {
		c.add(node.Line, field, "must be text, not %s", describe(node))
		return "", false
	}
	if looksLikeCredential(node.Value) {
		c.add(node.Line, field,
			"looks like an AWS credential; a deck file never holds one, so remove it and share credentials another way")
		return "", false
	}
	return node.Value, true
}

// match reads text and checks it with valid; rule says in words what is expected.
func (c *checker) match(node *yaml.Node, field string, valid func(string) bool, rule string) string {
	value, ok := c.text(node, field)
	if !ok {
		return ""
	}
	if !valid(value) {
		c.add(node.Line, field, "must be %s", rule)
		return ""
	}
	return value
}

// describe names the kind of a node for a message, without its value.
func describe(node *yaml.Node) string {
	switch node.Kind {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	}
	switch node.ShortTag() {
	case "!!null":
		return "an empty value"
	case "!!int", "!!float":
		return "a number"
	case "!!bool":
		return "true or false"
	default:
		return "text"
	}
}

// looksLikeCredential spots an AWS access key ID, or a run of 40 or more characters of
// the kind a secret access key or a session token is made of, mixing upper case, lower
// case and digits.
func looksLikeCredential(value string) bool {
	if accessKeyID.MatchString(value) {
		return true
	}
	for _, run := range secretLikeRun.FindAllString(value, -1) {
		if hasMixedCase(run) {
			return true
		}
	}
	return false
}

func hasMixedCase(value string) bool {
	var upper, lower, digit bool
	for _, r := range value {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	return upper && lower && digit
}

func validDeckName(value string) bool {
	count := utf8.RuneCountInString(value)
	if count == 0 || count > 64 {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

func validHost(value string) bool {
	if ip := net.ParseIP(value); ip != nil {
		return ip.To4() != nil
	}
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(value, "."), ".") {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return true
}

func join(field, key string) string {
	if field == "" {
		return key
	}
	return field + "." + key
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
