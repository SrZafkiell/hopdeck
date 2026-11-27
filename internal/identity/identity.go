// Package identity checks who a profile's credentials belong to and sorts the answer
// into the states the window shows: valid, expired, needs MFA, needs sign-in, or error
// with its reason.
package identity

import (
	"context"
	"errors"
	"time"

	"github.com/SrZafkiell/hopdeck/internal/awsapi"
)

// State is what a profile's credentials can do right now.
type State string

const (
	// Valid credentials answered; Account and Caller say whose they are.
	Valid State = "valid"
	// Expired credentials need a renewal.
	Expired State = "expired"
	// NeedsMFA means the profile asks for an MFA code before it has credentials.
	NeedsMFA State = "needs-mfa"
	// NeedsSignIn means the IAM Identity Center sign-in has expired or never happened.
	NeedsSignIn State = "needs-sign-in"
	// Failed is any other failure; Reason says what.
	Failed State = "error"
)

// Result is the outcome of one check.
type Result struct {
	State State
	// Account and Caller are set when State is Valid.
	Account string
	Caller  string
	// Reason says in plain words what is wrong when State is not Valid.
	Reason string
}

// Checker asks STS who the caller is. The zero value is ready to use.
type Checker struct {
	// Attempts is how many times a throttled call is tried; 3 when zero.
	Attempts int
	// Pause waits before a retry; it sleeps, or returns early when ctx ends. Tests
	// replace it.
	Pause func(ctx context.Context, d time.Duration) error
}

// Check asks STS who the caller is. A throttled call is tried again after a growing
// pause; every other failure is sorted into a state at once.
func (c Checker) Check(ctx context.Context, sts awsapi.STS) Result {
	attempts := c.Attempts
	if attempts <= 0 {
		attempts = 3
	}
	pause := c.Pause
	if pause == nil {
		pause = sleep
	}

	wait := 250 * time.Millisecond
	for attempt := 1; ; attempt++ {
		identity, err := sts.CallerIdentity(ctx)
		if err == nil {
			return Result{State: Valid, Account: identity.Account, Caller: identity.ARN}
		}
		var apiErr *awsapi.Error
		throttled := errors.As(err, &apiErr) && apiErr.Kind == awsapi.KindThrottled
		if !throttled || attempt == attempts {
			return resultOf(err)
		}
		if pauseErr := pause(ctx, wait); pauseErr != nil {
			return resultOf(pauseErr)
		}
		wait *= 2
	}
}

// FromError sorts a failure that happened before STS could be asked, such as loading the
// profile, into the same states as Check.
func FromError(err error) Result {
	return resultOf(err)
}

// resultOf sorts a failed check into a state with a reason in plain words.
func resultOf(err error) Result {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Result{State: Failed, Reason: "The check was stopped before AWS answered."}
	}
	var apiErr *awsapi.Error
	if !errors.As(err, &apiErr) {
		return Result{State: Failed, Reason: err.Error()}
	}
	switch apiErr.Kind {
	case awsapi.KindExpired:
		return Result{State: Expired, Reason: "The credentials of this profile have expired. Renew them to continue."}
	case awsapi.KindNeedsMFA:
		return Result{State: NeedsMFA, Reason: "This profile asks for an MFA code before it can be used."}
	case awsapi.KindNeedsSignIn:
		return Result{State: NeedsSignIn, Reason: "The IAM Identity Center sign-in of this profile has expired or has not happened yet."}
	case awsapi.KindThrottled:
		return Result{State: Failed, Reason: "AWS kept asking Hopdeck to slow down. Try again in a moment."}
	case awsapi.KindDenied:
		return Result{State: Failed, Reason: "AWS refused the request: " + apiErr.Message}
	case awsapi.KindNetwork:
		return Result{State: Failed, Reason: "AWS could not be reached: " + apiErr.Message}
	default:
		return Result{State: Failed, Reason: apiErr.Error()}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
