package awssdk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/SrZafkiell/hopdeck/internal/awsapi"
	"github.com/SrZafkiell/hopdeck/internal/profiles"
)

func TestClassify(t *testing.T) {
	apiError := func(code string) error {
		return &smithy.GenericAPIError{Code: code, Message: "message from AWS"}
	}
	cases := []struct {
		name string
		err  error
		kind awsapi.Kind
	}{
		{"expired token", apiError("ExpiredToken"), awsapi.KindExpired},
		{"request expired", apiError("RequestExpired"), awsapi.KindExpired},
		{"access denied", apiError("AccessDenied"), awsapi.KindDenied},
		{"access denied exception", apiError("AccessDeniedException"), awsapi.KindDenied},
		{"throttling", apiError("Throttling"), awsapi.KindThrottled},
		{"too many requests", apiError("TooManyRequestsException"), awsapi.KindThrottled},
		{"another API error", apiError("InvalidClientTokenId"), awsapi.KindOther},
		{"a role that asks for MFA", fmt.Errorf("load: %w", config.AssumeRoleTokenProviderNotSetError{}), awsapi.KindNeedsMFA},
		{"an expired Identity Center token", fmt.Errorf("retrieve: %w", &ssocreds.InvalidTokenError{Err: errors.New("token expired")}), awsapi.KindNeedsSignIn},
		{"no Identity Center token", fmt.Errorf("failed to read cached SSO token file: %w", fs.ErrNotExist), awsapi.KindNeedsSignIn},
		{"no network", &smithyhttp.RequestSendError{Err: errors.New("dial tcp: lookup sts.example: no such host")}, awsapi.KindNetwork},
		{"anything else", errors.New("something unexpected"), awsapi.KindOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sorted *awsapi.Error
			if err := Classify("sts:GetCallerIdentity", tc.err); !errors.As(err, &sorted) || sorted.Kind != tc.kind {
				t.Fatalf("got %v", err)
			}
			if sorted.Op != "sts:GetCallerIdentity" || !errors.Is(sorted, tc.err) {
				t.Fatalf("op %q, cause kept: %v", sorted.Op, errors.Is(sorted, tc.err))
			}
		})
	}

	if Classify("op", nil) != nil {
		t.Error("nil became an error")
	}
	already := &awsapi.Error{Kind: awsapi.KindDenied, Op: "first"}
	if Classify("second", already) != error(already) {
		t.Error("an already sorted error was sorted again")
	}
	var sorted *awsapi.Error
	if errors.As(Classify("op", apiError("ExpiredToken")), &sorted); sorted.Message != "ExpiredToken: message from AWS" {
		t.Errorf("message: %q", sorted.Message)
	}
}

func TestLoadProfile(t *testing.T) {
	// The environment must not decide the region or the profile in these tests.
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_PROFILE", "")
	dir := t.TempDir()
	files := profiles.Files{Config: filepath.Join(dir, "config"), Credentials: filepath.Join(dir, "credentials")}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(files.Config, `[profile with-region]
region = eu-west-1

[profile no-region]

[profile needs-mfa]
role_arn = arn:aws:iam::123456789012:role/Admin
source_profile = with-region
mfa_serial = arn:aws:iam::123456789012:mfa/example-user
`)
	write(files.Credentials, `[with-region]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

[no-region]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
`)
	ctx := context.Background()

	cfg, err := LoadProfile(ctx, "with-region", files, "us-east-1")
	if err != nil || cfg.Region != "eu-west-1" {
		t.Fatalf("the profile's region must win: %q, %v", cfg.Region, err)
	}
	cfg, err = LoadProfile(ctx, "no-region", files, "us-east-2")
	if err != nil || cfg.Region != "us-east-2" {
		t.Fatalf("the default region must apply: %q, %v", cfg.Region, err)
	}

	_, err = LoadProfile(ctx, "needs-mfa", files, "us-east-1")
	var sorted *awsapi.Error
	if !errors.As(err, &sorted) || sorted.Kind != awsapi.KindNeedsMFA {
		t.Fatalf("a role with mfa_serial: %v", err)
	}

	if _, err := LoadProfile(ctx, "missing", files, "us-east-1"); err == nil {
		t.Fatal("a profile that does not exist loaded")
	}
}
