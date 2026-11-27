// Package awssdk adapts the AWS SDK for Go v2 to the interfaces of package awsapi and
// sorts the SDK's errors into awsapi kinds. It is the only package of the core that
// imports the SDK; everything above it is tested with the fakes of package fake.
package awssdk

import (
	"context"
	"errors"
	"io/fs"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/SrZafkiell/hopdeck/internal/awsapi"
	"github.com/SrZafkiell/hopdeck/internal/profiles"
)

// LoadProfile loads one profile from the given AWS files, the way the AWS CLI resolves
// it: stored keys, a role with its source, a credential process or IAM Identity Center.
// defaultRegion is used when neither the profile nor the environment sets a region.
// The SDK resolves credentials when a call first needs them and keeps them in memory.
func LoadProfile(ctx context.Context, profile string, files profiles.Files, defaultRegion string) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
		config.WithSharedConfigFiles([]string{files.Config}),
		config.WithSharedCredentialsFiles([]string{files.Credentials}),
		config.WithDefaultRegion(defaultRegion),
	)
	if err != nil {
		return aws.Config{}, Classify("load profile", err)
	}
	return cfg, nil
}

// STS is awsapi.STS on the AWS SDK.
type STS struct {
	client *sts.Client
}

// NewSTS returns an STS client for the configuration of one profile.
func NewSTS(cfg aws.Config) *STS {
	return &STS{client: sts.NewFromConfig(cfg)}
}

// CallerIdentity calls sts:GetCallerIdentity, which needs no permission.
func (s *STS) CallerIdentity(ctx context.Context) (awsapi.Identity, error) {
	out, err := s.client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return awsapi.Identity{}, Classify("sts:GetCallerIdentity", err)
	}
	return awsapi.Identity{
		Account: aws.ToString(out.Account),
		ARN:     aws.ToString(out.Arn),
		UserID:  aws.ToString(out.UserId),
	}, nil
}

var (
	expiredCodes = codes("ExpiredToken", "ExpiredTokenException", "RequestExpired", "TokenRefreshRequired")
	deniedCodes  = codes("AccessDenied", "AccessDeniedException", "UnauthorizedOperation", "UnauthorizedAccess")
	busyCodes    = codes("Throttling", "ThrottlingException", "RequestLimitExceeded", "TooManyRequestsException",
		"RequestThrottled", "RequestThrottledException", "SlowDown")
)

// Classify sorts an SDK error into an awsapi kind. The message kept is AWS's own error
// code and message, or the error's text; neither carries a credential. An error that is
// already an *awsapi.Error is returned as it is.
func Classify(op string, err error) error {
	if err == nil {
		return nil
	}
	var sorted *awsapi.Error
	if errors.As(err, &sorted) {
		return err
	}
	result := &awsapi.Error{Kind: awsapi.KindOther, Op: op, Message: err.Error(), Err: err}

	var needsMFA config.AssumeRoleTokenProviderNotSetError
	var ssoToken *ssocreds.InvalidTokenError
	var apiErr smithy.APIError
	var sendErr *smithyhttp.RequestSendError
	switch {
	case errors.As(err, &needsMFA):
		result.Kind, result.Message = awsapi.KindNeedsMFA, "the profile asks for an MFA code"
	case errors.As(err, &ssoToken):
		result.Kind, result.Message = awsapi.KindNeedsSignIn, "the IAM Identity Center sign-in has expired"
	case errors.Is(err, fs.ErrNotExist) && strings.Contains(strings.ToLower(err.Error()), "sso"):
		result.Kind, result.Message = awsapi.KindNeedsSignIn, "no IAM Identity Center sign-in was found for this profile"
	case errors.As(err, &apiErr):
		result.Message = apiErr.ErrorCode() + ": " + apiErr.ErrorMessage()
		switch code := apiErr.ErrorCode(); {
		case expiredCodes[code]:
			result.Kind = awsapi.KindExpired
		case deniedCodes[code]:
			result.Kind = awsapi.KindDenied
		case busyCodes[code]:
			result.Kind = awsapi.KindThrottled
		}
	case errors.As(err, &sendErr):
		result.Kind, result.Message = awsapi.KindNetwork, sendErr.Err.Error()
	}
	return result
}

func codes(list ...string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, code := range list {
		set[code] = true
	}
	return set
}

var _ awsapi.STS = (*STS)(nil)
