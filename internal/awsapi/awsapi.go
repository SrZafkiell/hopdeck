// Package awsapi holds the small interfaces through which Hopdeck calls AWS: STS for the
// identity of a profile, EC2 to find a jump host, and Systems Manager to open and end
// sessions. They use Hopdeck's own types, so the logic above them is tested with the
// fakes of package fake and no network, and only the adapters touch the AWS SDK.
package awsapi

import (
	"context"
	"fmt"
)

// Identity is who a profile's credentials belong to.
type Identity struct {
	// Account is the 12-digit AWS account ID.
	Account string
	// ARN is the caller: a user, an assumed role or a federated identity.
	ARN    string
	UserID string
}

// STS answers who the caller is.
type STS interface {
	CallerIdentity(ctx context.Context) (Identity, error)
}

// Instance is an EC2 instance as Hopdeck needs it.
type Instance struct {
	ID string
	// State is the EC2 state name: pending, running, stopping, stopped, and so on.
	State string
}

// InstanceFilter finds instances by ID or by tags; exactly one of the two is set.
type InstanceFilter struct {
	ID   string
	Tags map[string]string
}

// EC2 finds the instances a jump host description matches.
type EC2 interface {
	Instances(ctx context.Context, filter InstanceFilter) ([]Instance, error)
}

// SessionRequest asks Systems Manager for a port forward through an instance.
type SessionRequest struct {
	Target string
	// Document is AWS-StartPortForwardingSessionToRemoteHost or
	// AWS-StartPortForwardingSession.
	Document   string
	Parameters map[string][]string
}

// Session is a session Systems Manager opened. StreamURL and Token are what the data
// channel connects with; Token is a secret and is never logged.
type Session struct {
	ID        string
	StreamURL string
	Token     string
}

// SSM opens and ends Session Manager sessions.
type SSM interface {
	StartSession(ctx context.Context, request SessionRequest) (Session, error)
	TerminateSession(ctx context.Context, sessionID string) error
}

// Kind says what went wrong in a call to AWS, in terms Hopdeck acts on.
type Kind int

const (
	// KindOther is anything else; the message says what.
	KindOther Kind = iota
	// KindDenied means the caller lacks a permission.
	KindDenied
	// KindExpired means the credentials have expired.
	KindExpired
	// KindThrottled means AWS asked to slow down.
	KindThrottled
	// KindNeedsMFA means a profile asks for an MFA code that was not given.
	KindNeedsMFA
	// KindNeedsSignIn means the IAM Identity Center sign-in has expired or never happened.
	KindNeedsSignIn
	// KindNetwork means AWS could not be reached.
	KindNetwork
)

var kindNames = map[Kind]string{
	KindOther:       "other",
	KindDenied:      "denied",
	KindExpired:     "expired",
	KindThrottled:   "throttled",
	KindNeedsMFA:    "needs MFA",
	KindNeedsSignIn: "needs sign-in",
	KindNetwork:     "network",
}

func (k Kind) String() string {
	if name, ok := kindNames[k]; ok {
		return name
	}
	return fmt.Sprintf("kind(%d)", int(k))
}

// Error is a failed call to AWS, sorted into a Kind. Message is AWS's own message or a
// description of the failure; it never holds a credential.
type Error struct {
	Kind Kind
	// Op is the operation that failed, such as sts:GetCallerIdentity.
	Op      string
	Message string
	// Err is the underlying error, when there is one.
	Err error
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("%s: %s", e.Op, e.Kind)
	}
	return fmt.Sprintf("%s: %s: %s", e.Op, e.Kind, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}
