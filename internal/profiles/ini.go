package profiles

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

// maxFileSize caps what is read of each file; real AWS configuration files are a few
// kilobytes.
const maxFileSize = 1 << 20

// present stands in for the value of a credential setting: only its presence is kept.
const present = "(present)"

// credentialKeys are the settings whose values are never kept.
var credentialKeys = map[string]bool{
	"aws_access_key_id":     true,
	"aws_secret_access_key": true,
	"aws_session_token":     true,
	"aws_security_token":    true,
}

// section is one [header] of an INI file with its settings, keys in lower case.
type section struct {
	name string
	line int
	keys map[string]string
}

// readINI reads an AWS configuration file the way the AWS CLI does: [section] headers,
// key = value settings and whole-line comments starting with # or ;. A line indented
// deeper than the setting above it continues that setting (such as the nested settings
// under s3 =), which no profile setting uses, so it is skipped; settings that are all
// indented alike under their header are read as settings. The file is opened read-only.
// A missing file gives no sections. Problems name the file and the line, never the
// content of the line.
func readINI(path, label string, problems *[]string) ([]section, error) {
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the AWS %s file cannot be opened: %w", label, err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("the AWS %s file cannot be read: %w", label, err)
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("the AWS %s file is larger than %d bytes", label, maxFileSize)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	var sections []section
	current := -1
	keyIndent, haveKey := 0, false
	for i, raw := range strings.Split(string(data), "\n") {
		number := i + 1
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] == '#' || trimmed[0] == ';' {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if haveKey && indent > keyIndent {
			continue
		}
		if trimmed[0] == '[' {
			end := strings.IndexByte(trimmed, ']')
			haveKey = false
			if end < 0 {
				*problems = append(*problems, fmt.Sprintf("%s line %d: a section header without its closing ]", label, number))
				current = -1
				continue
			}
			sections = append(sections, section{
				name: strings.TrimSpace(trimmed[1:end]),
				line: number,
				keys: map[string]string{},
			})
			current = len(sections) - 1
			continue
		}

		equals := strings.IndexByte(trimmed, '=')
		if equals <= 0 {
			*problems = append(*problems, fmt.Sprintf("%s line %d: neither a [section], a key = value setting nor a comment", label, number))
			continue
		}
		if current < 0 {
			*problems = append(*problems, fmt.Sprintf("%s line %d: a setting before the first [section]", label, number))
			continue
		}
		key := strings.ToLower(strings.TrimSpace(trimmed[:equals]))
		value := strings.TrimSpace(trimmed[equals+1:])
		keyIndent, haveKey = indent, true
		if credentialKeys[key] {
			if value == "" {
				continue
			}
			value = present
		}
		sections[current].keys[key] = value
	}
	return sections, nil
}
