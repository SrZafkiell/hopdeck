// Package profiles lists the AWS profiles of the standard AWS configuration files and
// says where each one gets its credentials.
//
// It reads ~/.aws/config and ~/.aws/credentials (or the files that AWS_CONFIG_FILE and
// AWS_SHARED_CREDENTIALS_FILE name) the way the AWS CLI does, opens them read-only and
// never changes them. It keeps no credential: the values of access keys, secret keys and
// session tokens are dropped while reading, so only their presence is known. Turning a
// profile into credentials is the AWS SDK's job (M03-02); the SDK has no public way to
// list profiles, which is why this package exists.
package profiles

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Kind says where a profile's credentials come from.
type Kind string

const (
	// KindKeys is an access key stored in the files, long-lived or, with a session
	// token, temporary.
	KindKeys Kind = "keys"
	// KindRole is a role assumed with the credentials of another profile or source.
	KindRole Kind = "role"
	// KindIdentityCenter is IAM Identity Center (single sign-on).
	KindIdentityCenter Kind = "identity-center"
	// KindProcess is an external program that prints credentials.
	KindProcess Kind = "process"
	// KindNone means the files give the profile no credentials, for example a profile
	// that only sets a region.
	KindNone Kind = "none"
)

// Profile is one AWS profile, without any credential.
type Profile struct {
	Name string
	Kind Kind
	// Region is the profile's default region, if it sets one.
	Region string
	// InConfig and InCredentials say which files define the profile.
	InConfig      bool
	InCredentials bool

	// RoleARN, SourceProfile and CredentialSource describe a role profile.
	RoleARN          string
	SourceProfile    string
	CredentialSource string
	// MFASerial names the MFA device when the profile asks for a code: a role with
	// mfa_serial, or an IAM user whose policy requires MFA.
	MFASerial string

	// The SSO fields describe an IAM Identity Center profile. SSOStartURL and SSORegion
	// come from the profile or from the sso-session section it names.
	SSOSession   string
	SSOStartURL  string
	SSORegion    string
	SSOAccountID string
	SSORoleName  string

	// Temporary is true when the stored access key comes with a session token.
	Temporary bool
}

// Files are the two AWS configuration files.
type Files struct {
	Config      string
	Credentials string
}

// Result is what Discover found.
type Result struct {
	Files    Files
	Profiles []Profile
	// Problems are things in the files that Hopdeck cannot use. They name the file and
	// the line or the profile, never a value, because a value in the credentials file may
	// be a secret.
	Problems []string
}

// DefaultFiles returns the files of this process's environment and user.
func DefaultFiles() (Files, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Files{}, err
	}
	return FilesFrom(os.Getenv, home), nil
}

// FilesFrom resolves the two files the way the AWS CLI does: AWS_CONFIG_FILE and
// AWS_SHARED_CREDENTIALS_FILE when they are set (a leading ~ is the home folder),
// otherwise .aws/config and .aws/credentials in the home folder.
func FilesFrom(getenv func(string) string, home string) Files {
	return Files{
		Config:      pathFrom(getenv("AWS_CONFIG_FILE"), home, "config"),
		Credentials: pathFrom(getenv("AWS_SHARED_CREDENTIALS_FILE"), home, "credentials"),
	}
}

func pathFrom(value, home, name string) string {
	switch {
	case value == "":
		return filepath.Join(home, ".aws", name)
	case value == "~":
		return home
	case strings.HasPrefix(value, "~/"), strings.HasPrefix(value, `~\`):
		return filepath.Join(home, value[2:])
	default:
		return value
	}
}

// Discover reads both files and lists their profiles, the default profile first and the
// others by name. A missing file holds no profiles; a file that exists but cannot be
// read is an error.
func Discover(files Files) (Result, error) {
	result := Result{Files: files, Profiles: []Profile{}}
	var problems []string

	configSections, err := readINI(files.Config, "config", &problems)
	if err != nil {
		return result, err
	}
	credentialSections, err := readINI(files.Credentials, "credentials", &problems)
	if err != nil {
		return result, err
	}

	found := map[string]*entry{}
	sessions := map[string]map[string]string{}
	get := func(name string) *entry {
		if found[name] == nil {
			found[name] = &entry{config: map[string]string{}, credentials: map[string]string{}}
		}
		return found[name]
	}

	for _, s := range configSections {
		name, kind := configSectionName(s.name)
		switch kind {
		case sectionProfile:
			if name == "" {
				problems = append(problems, fmt.Sprintf("config line %d: a profile section without a name", s.line))
				continue
			}
			e := get(name)
			if e.inConfig {
				problems = append(problems, fmt.Sprintf("config line %d: profile %q appears a second time; its settings are merged", s.line, name))
			}
			e.inConfig = true
			merge(e.config, s.keys)
		case sectionSSOSession:
			if sessions[name] == nil {
				sessions[name] = map[string]string{}
			}
			merge(sessions[name], s.keys)
		case sectionOther:
			problems = append(problems, fmt.Sprintf(
				"config line %d: section [%s] is not a profile; in the config file a profile is written [profile name]", s.line, s.name))
		}
	}
	for _, s := range credentialSections {
		if s.name == "" {
			problems = append(problems, fmt.Sprintf("credentials line %d: a profile section without a name", s.line))
			continue
		}
		e := get(s.name)
		if e.inCredentials {
			problems = append(problems, fmt.Sprintf("credentials line %d: profile %q appears a second time; its settings are merged", s.line, s.name))
		}
		e.inCredentials = true
		merge(e.credentials, s.keys)
	}

	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "default" || names[j] == "default" {
			return names[i] == "default"
		}
		return names[i] < names[j]
	})

	for _, name := range names {
		profile, profileProblems := build(name, found[name], sessions)
		result.Profiles = append(result.Profiles, profile)
		problems = append(problems, profileProblems...)
	}
	problems = append(problems, sourceProblems(result.Profiles)...)
	result.Problems = problems
	return result, nil
}

// entry gathers one profile's settings from both files before it is classified.
type entry struct {
	config, credentials     map[string]string
	inConfig, inCredentials bool
}

// setting returns a setting, the credentials file first, as the AWS CLI reads them.
func (e *entry) setting(key string) string {
	if value := e.credentials[key]; value != "" {
		return value
	}
	return e.config[key]
}

type sectionKind int

const (
	sectionProfile sectionKind = iota
	sectionSSOSession
	sectionIgnored
	sectionOther
)

// configSectionName reads a config file section header: [default], [profile name],
// [sso-session name], and the CLI's own sections, which hold no profile.
func configSectionName(header string) (string, sectionKind) {
	if header == "default" {
		return "default", sectionProfile
	}
	if word, rest, ok := strings.Cut(header, " "); ok {
		rest = strings.TrimSpace(rest)
		switch word {
		case "profile":
			return rest, sectionProfile
		case "sso-session":
			return rest, sectionSSOSession
		case "services":
			return rest, sectionIgnored
		}
	}
	switch header {
	case "profile":
		return "", sectionProfile
	case "plugins", "preview", "cli":
		return header, sectionIgnored
	}
	return header, sectionOther
}

func merge(into, from map[string]string) {
	for key, value := range from {
		into[key] = value
	}
}

// build classifies one profile. A role comes first, because the AWS CLI assumes the role
// whatever gives it its first credentials; then Identity Center, a credential process and
// stored keys.
func build(name string, e *entry, sessions map[string]map[string]string) (Profile, []string) {
	var problems []string
	profile := Profile{
		Name:             name,
		InConfig:         e.inConfig,
		InCredentials:    e.inCredentials,
		Region:           e.setting("region"),
		RoleARN:          e.setting("role_arn"),
		SourceProfile:    e.setting("source_profile"),
		CredentialSource: e.setting("credential_source"),
		MFASerial:        e.setting("mfa_serial"),
		SSOSession:       e.setting("sso_session"),
		SSOStartURL:      e.setting("sso_start_url"),
		SSORegion:        e.setting("sso_region"),
		SSOAccountID:     e.setting("sso_account_id"),
		SSORoleName:      e.setting("sso_role_name"),
	}
	hasKeyID := e.setting("aws_access_key_id") != ""
	hasSecret := e.setting("aws_secret_access_key") != ""
	profile.Temporary = hasKeyID && hasSecret && e.setting("aws_session_token") != ""

	if profile.SSOSession != "" {
		session, ok := sessions[profile.SSOSession]
		if !ok {
			problems = append(problems, fmt.Sprintf("profile %q names sso_session %q, which the config file does not define", name, profile.SSOSession))
		} else {
			if profile.SSOStartURL == "" {
				profile.SSOStartURL = session["sso_start_url"]
			}
			if profile.SSORegion == "" {
				profile.SSORegion = session["sso_region"]
			}
		}
	}

	switch {
	case profile.RoleARN != "":
		profile.Kind = KindRole
		if profile.SourceProfile == "" && profile.CredentialSource == "" && e.setting("web_identity_token_file") == "" {
			problems = append(problems, fmt.Sprintf("profile %q has role_arn but no source_profile or credential_source", name))
		}
	case profile.SSOSession != "" || profile.SSOStartURL != "":
		profile.Kind = KindIdentityCenter
	case e.setting("credential_process") != "":
		profile.Kind = KindProcess
	case hasKeyID && hasSecret:
		profile.Kind = KindKeys
	default:
		profile.Kind = KindNone
		if hasKeyID != hasSecret {
			problems = append(problems, fmt.Sprintf("profile %q has only one of aws_access_key_id and aws_secret_access_key", name))
		}
	}
	return profile, problems
}

// sourceProblems checks the source_profile of every role: it must exist, and following
// the sources must end, unless a profile names itself and stores keys of its own, which
// the AWS CLI allows.
func sourceProblems(profiles []Profile) []string {
	byName := make(map[string]Profile, len(profiles))
	for _, p := range profiles {
		byName[p.Name] = p
	}
	var problems []string
	for _, p := range profiles {
		if p.Kind != KindRole || p.SourceProfile == "" {
			continue
		}
		seen := map[string]bool{p.Name: true}
		current := p
		for current.Kind == KindRole && current.SourceProfile != "" {
			next, ok := byName[current.SourceProfile]
			if !ok {
				problems = append(problems, fmt.Sprintf("profile %q uses source_profile %q, which does not exist", current.Name, current.SourceProfile))
				break
			}
			if next.Name == current.Name {
				break // a role that uses its own stored keys
			}
			if seen[next.Name] {
				problems = append(problems, fmt.Sprintf("profile %q: its source profiles go round in a circle", p.Name))
				break
			}
			seen[next.Name] = true
			current = next
		}
	}
	return problems
}
