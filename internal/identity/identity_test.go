package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SrZafkiell/hopdeck/internal/awsapi"
	"github.com/SrZafkiell/hopdeck/internal/awsapi/fake"
)

const op = "sts:GetCallerIdentity"

// example is the documented example account with made-up names.
var example = awsapi.Identity{
	Account: "123456789012",
	ARN:     "arn:aws:sts::123456789012:assumed-role/Developer/example-user",
	UserID:  "AROAEXAMPLEID:example-user",
}

// noPause makes retries instant and counts them.
type noPause struct{ pauses []time.Duration }

func (p *noPause) pause(_ context.Context, d time.Duration) error {
	p.pauses = append(p.pauses, d)
	return nil
}

func TestStates(t *testing.T) {
	cases := []struct {
		name   string
		answer error
		state  State
		reason string
	}{
		{"expired credentials", fake.Expired(op), Expired, "have expired"},
		{"a role that asks for MFA", fake.NeedsMFA(op), NeedsMFA, "MFA code"},
		{"an expired Identity Center sign-in", fake.NeedsSignIn(op), NeedsSignIn, "IAM Identity Center sign-in"},
		{"a refusal", fake.Denied(op), Failed, "AWS refused the request: not authorized"},
		{"no network", fake.Unreachable(op), Failed, "could not be reached"},
		{"an error from outside the AWS layer", errors.New("disk full"), Failed, "disk full"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sts := fake.NewSTS(fake.Answer[awsapi.Identity]{Err: tc.answer})
			result := Checker{}.Check(context.Background(), sts)
			if result.State != tc.state || !strings.Contains(result.Reason, tc.reason) || result.Account != "" {
				t.Fatalf("got %+v", result)
			}
			if sts.Calls() != 1 {
				t.Fatalf("a %s was tried %d times", tc.name, sts.Calls())
			}
		})
	}
}

func TestValid(t *testing.T) {
	sts := fake.NewSTS(fake.Answer[awsapi.Identity]{Value: example})
	result := Checker{}.Check(context.Background(), sts)
	if result.State != Valid || result.Account != example.Account || result.Caller != example.ARN || result.Reason != "" {
		t.Fatalf("got %+v", result)
	}
}

func TestThrottlingIsRetried(t *testing.T) {
	pauses := &noPause{}
	sts := fake.NewSTS(
		fake.Answer[awsapi.Identity]{Err: fake.Throttled(op)},
		fake.Answer[awsapi.Identity]{Err: fake.Throttled(op)},
		fake.Answer[awsapi.Identity]{Value: example},
	)
	result := Checker{Pause: pauses.pause}.Check(context.Background(), sts)
	if result.State != Valid || sts.Calls() != 3 {
		t.Fatalf("got %+v after %d calls", result, sts.Calls())
	}
	if len(pauses.pauses) != 2 || pauses.pauses[1] != 2*pauses.pauses[0] {
		t.Fatalf("pauses %v should grow", pauses.pauses)
	}
}

func TestThrottlingGivesUp(t *testing.T) {
	pauses := &noPause{}
	sts := fake.NewSTS(fake.Answer[awsapi.Identity]{Err: fake.Throttled(op)})
	result := Checker{Attempts: 4, Pause: pauses.pause}.Check(context.Background(), sts)
	if result.State != Failed || !strings.Contains(result.Reason, "slow down") || sts.Calls() != 4 || len(pauses.pauses) != 3 {
		t.Fatalf("got %+v after %d calls and %d pauses", result, sts.Calls(), len(pauses.pauses))
	}
}

func TestACancelledCheckStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sts := fake.NewSTS(fake.Answer[awsapi.Identity]{Value: example})
	result := Checker{}.Check(ctx, sts)
	if result.State != Failed || !strings.Contains(result.Reason, "stopped") {
		t.Fatalf("got %+v", result)
	}
}

func TestThePauseEndsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	sts := fake.NewSTS(fake.Answer[awsapi.Identity]{Err: fake.Throttled(op)})
	result := Checker{Attempts: 10}.Check(ctx, sts)
	if result.State != Failed || time.Since(start) > 2*time.Second {
		t.Fatalf("got %+v after %v", result, time.Since(start))
	}
}

func TestFromError(t *testing.T) {
	if result := FromError(fake.NeedsMFA("load profile")); result.State != NeedsMFA {
		t.Fatalf("got %+v", result)
	}
}
