package profiles

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every value in these files is an example: the documented AWS example account ID and
// access key, and made-up names.
const (
	exampleKeyID  = "AKIAIOSFODNN7EXAMPLE"
	exampleSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	exampleToken  = "example-session-token-value"
)

const config = `# AWS config for the tests
[default]
region = us-east-1
output = json

[profile admin]
role_arn = arn:aws:iam::123456789012:role/Admin
source_profile = default
mfa_serial = arn:aws:iam::123456789012:mfa/example-user
region = eu-west-1

[profile sso-dev]
sso_session = corp
sso_account_id = 123456789012
sso_role_name = Developer
region = us-east-2

[sso-session corp]
sso_start_url = https://example.awsapps.com/start
sso_region = us-east-1
sso_registration_scopes = sso:account:access

[profile legacy-sso]
sso_start_url = https://legacy.example.com/start
sso_region = us-west-2
sso_account_id = 123456789012
sso_role_name = ReadOnly

[profile tool]
credential_process = /usr/local/bin/get-credentials --profile tool

; a profile with only a region and nested s3 settings
[profile region-only]
region = ap-southeast-2
s3 =
  region = not-a-region-1
  max_concurrent_requests = 20

[services local]
s3 =
  endpoint_url = http://localhost:4566
`

const credentials = `[default]
aws_access_key_id = ` + exampleKeyID + `
aws_secret_access_key = ` + exampleSecret + `

[ci]
AWS_ACCESS_KEY_ID=` + exampleKeyID + `
AWS_SECRET_ACCESS_KEY=` + exampleSecret + `
aws_session_token = ` + exampleToken + `
`

func TestDiscoverKinds(t *testing.T) {
	result := discover(t, config, credentials)
	if len(result.Problems) != 0 {
		t.Fatalf("unexpected problems: %q", result.Problems)
	}

	var names []string
	for _, p := range result.Profiles {
		names = append(names, p.Name+"="+string(p.Kind))
	}
	want := "default=keys admin=role ci=keys legacy-sso=identity-center region-only=none sso-dev=identity-center tool=process"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("profiles:\n got %s\nwant %s", got, want)
	}

	byName := map[string]Profile{}
	for _, p := range result.Profiles {
		byName[p.Name] = p
	}
	check := func(name string, ok bool) {
		t.Helper()
		if !ok {
			t.Errorf("%s: %+v", name, byName[name])
		}
	}
	d := byName["default"]
	check("default", d.InConfig && d.InCredentials && d.Region == "us-east-1" && !d.Temporary)
	a := byName["admin"]
	check("admin", a.RoleARN == "arn:aws:iam::123456789012:role/Admin" && a.SourceProfile == "default" &&
		a.MFASerial == "arn:aws:iam::123456789012:mfa/example-user" && a.Region == "eu-west-1" && !a.InCredentials)
	c := byName["ci"]
	check("ci", c.Temporary && c.InCredentials && !c.InConfig)
	s := byName["sso-dev"]
	check("sso-dev", s.SSOSession == "corp" && s.SSOStartURL == "https://example.awsapps.com/start" &&
		s.SSORegion == "us-east-1" && s.SSOAccountID == "123456789012" && s.SSORoleName == "Developer" && s.Region == "us-east-2")
	l := byName["legacy-sso"]
	check("legacy-sso", l.SSOSession == "" && l.SSOStartURL == "https://legacy.example.com/start" && l.SSORegion == "us-west-2")
	r := byName["region-only"]
	check("region-only (the nested s3 region is not the profile's)", r.Region == "ap-southeast-2")
}

func TestNoCredentialValueIsKept(t *testing.T) {
	result := discover(t, config, credentials)
	dump := fmt.Sprintf("%#v", result)
	for _, secret := range []string{exampleKeyID, exampleSecret, exampleToken} {
		if strings.Contains(dump, secret) {
			t.Errorf("the result keeps a credential value (%s...)", secret[:4])
		}
	}
}

func TestProblems(t *testing.T) {
	brokenConfig := `[profile broken-role]
role_arn = arn:aws:iam::123456789012:role/X

[profile ghost-source]
role_arn = arn:aws:iam::123456789012:role/Y
source_profile = nowhere

[profile loop-a]
role_arn = arn:aws:iam::123456789012:role/A
source_profile = loop-b

[profile loop-b]
role_arn = arn:aws:iam::123456789012:role/B
source_profile = loop-a

[profile lost-session]
sso_session = missing

[myprofile]
region = us-east-1

[profile dup]
region = us-east-1
[profile dup]
output = json

[profile self]
role_arn = arn:aws:iam::123456789012:role/Self
source_profile = self

[profile]
region = us-east-1
`
	brokenCredentials := `[self]
aws_access_key_id = ` + exampleKeyID + `
aws_secret_access_key = ` + exampleSecret + `

[half]
aws_access_key_id = ` + exampleKeyID + `

` + exampleSecret + ` pasted on a line of its own
[unclosed
`
	result := discover(t, brokenConfig, brokenCredentials)
	all := strings.Join(result.Problems, "\n")
	wants := []string{
		`profile "broken-role" has role_arn but no source_profile or credential_source`,
		`profile "ghost-source" uses source_profile "nowhere", which does not exist`,
		`profile "loop-a": its source profiles go round in a circle`,
		`profile "loop-b": its source profiles go round in a circle`,
		`profile "lost-session" names sso_session "missing", which the config file does not define`,
		`config line 19: section [myprofile] is not a profile`,
		`config line 24: profile "dup" appears a second time`,
		`config line 31: a profile section without a name`,
		`profile "half" has only one of aws_access_key_id and aws_secret_access_key`,
		`credentials line 8: neither a [section], a key = value setting nor a comment`,
		`credentials line 9: a section header without its closing ]`,
	}
	for _, want := range wants {
		if !strings.Contains(all, want) {
			t.Errorf("missing problem %q", want)
		}
	}
	if strings.Contains(all, `"self"`) {
		t.Errorf("a role that uses its own stored keys is fine: %q", result.Problems)
	}
	for _, secret := range []string{exampleKeyID, exampleSecret} {
		if strings.Contains(all, secret) {
			t.Errorf("a problem repeats a credential value (%s...)", secret[:4])
		}
	}
	if len(result.Problems) != len(wants) {
		t.Errorf("got %d problems, want %d:\n%s", len(result.Problems), len(wants), all)
	}
}

func TestFormatting(t *testing.T) {
	// A byte order mark, Windows line endings, settings indented alike under their
	// header, upper-case keys, no spaces around =, and a comment with ;.
	content := "\xef\xbb\xbf[profile indented]\r\n  region = sa-east-1\r\n  output = json\r\n" +
		"; comment\r\n[profile Upper]\r\nREGION=ca-central-1\r\n"
	result := discover(t, content, "")
	if len(result.Problems) != 0 || len(result.Profiles) != 2 {
		t.Fatalf("got %+v", result)
	}
	if result.Profiles[0].Name != "Upper" || result.Profiles[0].Region != "ca-central-1" ||
		result.Profiles[1].Name != "indented" || result.Profiles[1].Region != "sa-east-1" {
		t.Fatalf("got %+v", result.Profiles)
	}
}

func TestFilesFrom(t *testing.T) {
	home := filepath.Join("C:", "Users", "example")
	env := map[string]string{}
	getenv := func(key string) string { return env[key] }

	files := FilesFrom(getenv, home)
	if files.Config != filepath.Join(home, ".aws", "config") || files.Credentials != filepath.Join(home, ".aws", "credentials") {
		t.Fatalf("defaults: %+v", files)
	}

	env["AWS_CONFIG_FILE"] = filepath.Join("D:", "aws", "my-config")
	env["AWS_SHARED_CREDENTIALS_FILE"] = "~/secure/creds"
	files = FilesFrom(getenv, home)
	if files.Config != filepath.Join("D:", "aws", "my-config") {
		t.Errorf("AWS_CONFIG_FILE: %s", files.Config)
	}
	if files.Credentials != filepath.Join(home, "secure", "creds") {
		t.Errorf("AWS_SHARED_CREDENTIALS_FILE with ~: %s", files.Credentials)
	}
}

func TestEnvironmentReachesDiscover(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "elsewhere-config")
	credentialsPath := filepath.Join(dir, "elsewhere-credentials")
	writeFile(t, configPath, "[profile from-env]\nregion = us-east-1\n")
	writeFile(t, credentialsPath, "[keys-from-env]\naws_access_key_id = "+exampleKeyID+"\naws_secret_access_key = "+exampleSecret+"\n")
	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)

	files, err := DefaultFiles()
	if err != nil {
		t.Fatal(err)
	}
	result, err := Discover(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Profiles) != 2 || result.Profiles[0].Name != "from-env" || result.Profiles[1].Kind != KindKeys {
		t.Fatalf("got %+v", result.Profiles)
	}
}

func TestMissingFiles(t *testing.T) {
	dir := t.TempDir()
	result, err := Discover(Files{Config: filepath.Join(dir, "none"), Credentials: filepath.Join(dir, "nothing")})
	if err != nil || len(result.Profiles) != 0 || len(result.Problems) != 0 {
		t.Fatalf("got %+v, %v", result, err)
	}
}

func TestFilesAreOpenedReadOnly(t *testing.T) {
	dir := t.TempDir()
	files := Files{Config: filepath.Join(dir, "config"), Credentials: filepath.Join(dir, "credentials")}
	writeFile(t, files.Config, config)
	writeFile(t, files.Credentials, credentials)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	for _, path := range []string{files.Config, files.Credentials} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, path := range []string{files.Config, files.Credentials} {
			_ = os.Chmod(path, 0o644)
		}
	})

	if _, err := Discover(files); err != nil {
		t.Fatalf("Discover on read-only files: %v", err)
	}
	for path, want := range map[string]string{files.Config: config, files.Credentials: credentials} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(old) {
			t.Errorf("%s was modified", filepath.Base(path))
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, []byte(want)) {
			t.Errorf("%s changed", filepath.Base(path))
		}
	}
}

func TestLimits(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "config")
	writeFile(t, big, "[default]\n"+strings.Repeat("# filler line\n", maxFileSize/14+1))
	if _, err := Discover(Files{Config: big, Credentials: filepath.Join(dir, "none")}); err == nil ||
		!strings.Contains(err.Error(), "larger than") {
		t.Errorf("an oversized file: %v", err)
	}
	if _, err := Discover(Files{Config: dir, Credentials: filepath.Join(dir, "none")}); err == nil {
		t.Error("a folder in place of the config file gave no error")
	}
}

// discover writes the two files into a temporary folder and runs Discover on them.
func discover(t *testing.T, configContent, credentialsContent string) Result {
	t.Helper()
	dir := t.TempDir()
	files := Files{Config: filepath.Join(dir, "config"), Credentials: filepath.Join(dir, "credentials")}
	writeFile(t, files.Config, configContent)
	writeFile(t, files.Credentials, credentialsContent)
	result, err := Discover(files)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return result
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
