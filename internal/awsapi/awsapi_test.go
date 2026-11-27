package awsapi

import (
	"errors"
	"testing"
)

func TestErrorTextAndUnwrap(t *testing.T) {
	cause := errors.New("connection reset")
	err := &Error{Kind: KindNetwork, Op: "ssm:StartSession", Message: "no route", Err: cause}
	if got := err.Error(); got != "ssm:StartSession: network: no route" {
		t.Errorf("Error(): %q", got)
	}
	if !errors.Is(err, cause) {
		t.Error("the cause is not reachable through errors.Is")
	}
	if got := (&Error{Kind: KindDenied, Op: "ec2:DescribeInstances"}).Error(); got != "ec2:DescribeInstances: denied" {
		t.Errorf("Error() without a message: %q", got)
	}
	if got := Kind(99).String(); got != "kind(99)" {
		t.Errorf("unknown kind: %q", got)
	}
}
