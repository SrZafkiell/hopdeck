// Package fake implements the interfaces of package awsapi with scripted answers, so the
// logic above them is tested without an AWS account or a network. Each fake answers its
// calls in order from a script; the last answer repeats once the script runs out.
package fake

import (
	"context"
	"sync"

	"github.com/SrZafkiell/hopdeck/internal/awsapi"
)

// Denied returns the error of a call refused for lack of a permission.
func Denied(op string) error {
	return &awsapi.Error{Kind: awsapi.KindDenied, Op: op, Message: "not authorized to perform " + op}
}

// Expired returns the error of a call made with expired credentials.
func Expired(op string) error {
	return &awsapi.Error{Kind: awsapi.KindExpired, Op: op, Message: "the security token included in the request is expired"}
}

// Throttled returns the error of a call AWS asked to slow down.
func Throttled(op string) error {
	return &awsapi.Error{Kind: awsapi.KindThrottled, Op: op, Message: "rate exceeded"}
}

// NeedsMFA returns the error of a profile that asks for an MFA code.
func NeedsMFA(op string) error {
	return &awsapi.Error{Kind: awsapi.KindNeedsMFA, Op: op, Message: "the profile asks for an MFA code"}
}

// NeedsSignIn returns the error of an expired IAM Identity Center sign-in.
func NeedsSignIn(op string) error {
	return &awsapi.Error{Kind: awsapi.KindNeedsSignIn, Op: op, Message: "the IAM Identity Center sign-in has expired"}
}

// Unreachable returns the error of a call that never reached AWS.
func Unreachable(op string) error {
	return &awsapi.Error{Kind: awsapi.KindNetwork, Op: op, Message: "no route to the AWS endpoint"}
}

// Answer is one scripted answer: a value, or an error.
type Answer[T any] struct {
	Value T
	Err   error
}

// script hands out answers in order and counts the calls.
type script[T any] struct {
	mu      sync.Mutex
	answers []Answer[T]
	calls   int
}

func (s *script[T]) next() (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if len(s.answers) == 0 {
		var zero T
		return zero, &awsapi.Error{Kind: awsapi.KindOther, Op: "fake", Message: "no answer scripted"}
	}
	index := s.calls - 1
	if index >= len(s.answers) {
		index = len(s.answers) - 1
	}
	return s.answers[index].Value, s.answers[index].Err
}

func (s *script[T]) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// STS is a scripted awsapi.STS.
type STS struct {
	script[awsapi.Identity]
}

// NewSTS returns an STS that gives the answers in order.
func NewSTS(answers ...Answer[awsapi.Identity]) *STS {
	return &STS{script[awsapi.Identity]{answers: answers}}
}

// CallerIdentity answers with the next scripted answer.
func (f *STS) CallerIdentity(ctx context.Context) (awsapi.Identity, error) {
	if err := ctx.Err(); err != nil {
		return awsapi.Identity{}, err
	}
	return f.next()
}

// Calls is how many times CallerIdentity was called.
func (f *STS) Calls() int { return f.count() }

// EC2 is a scripted awsapi.EC2 that also records the filters it was asked for.
type EC2 struct {
	script[[]awsapi.Instance]
	mu      sync.Mutex
	filters []awsapi.InstanceFilter
}

// NewEC2 returns an EC2 that gives the answers in order.
func NewEC2(answers ...Answer[[]awsapi.Instance]) *EC2 {
	return &EC2{script: script[[]awsapi.Instance]{answers: answers}}
}

// Instances records the filter and answers with the next scripted answer.
func (f *EC2) Instances(ctx context.Context, filter awsapi.InstanceFilter) ([]awsapi.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.filters = append(f.filters, filter)
	f.mu.Unlock()
	return f.next()
}

// Filters returns the filters Instances was called with, in order.
func (f *EC2) Filters() []awsapi.InstanceFilter {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]awsapi.InstanceFilter(nil), f.filters...)
}

// SSM is a scripted awsapi.SSM. It answers StartSession from its script, records the
// requests, and records which sessions were terminated.
type SSM struct {
	script[awsapi.Session]
	mu         sync.Mutex
	requests   []awsapi.SessionRequest
	terminated []string
	// TerminateErr, when set, is returned by every TerminateSession.
	TerminateErr error
}

// NewSSM returns an SSM whose StartSession gives the answers in order.
func NewSSM(answers ...Answer[awsapi.Session]) *SSM {
	return &SSM{script: script[awsapi.Session]{answers: answers}}
}

// StartSession records the request and answers with the next scripted answer.
func (f *SSM) StartSession(ctx context.Context, request awsapi.SessionRequest) (awsapi.Session, error) {
	if err := ctx.Err(); err != nil {
		return awsapi.Session{}, err
	}
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.mu.Unlock()
	return f.next()
}

// TerminateSession records the session ID.
func (f *SSM) TerminateSession(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminated = append(f.terminated, sessionID)
	return f.TerminateErr
}

// Requests returns the StartSession requests in order.
func (f *SSM) Requests() []awsapi.SessionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]awsapi.SessionRequest(nil), f.requests...)
}

// Terminated returns the IDs of the sessions TerminateSession was called for, in order.
func (f *SSM) Terminated() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.terminated...)
}

var (
	_ awsapi.STS = (*STS)(nil)
	_ awsapi.EC2 = (*EC2)(nil)
	_ awsapi.SSM = (*SSM)(nil)
)
