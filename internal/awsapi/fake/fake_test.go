package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/SrZafkiell/hopdeck/internal/awsapi"
)

func TestScriptsAnswerInOrderAndRepeatTheLast(t *testing.T) {
	sts := NewSTS(
		Answer[awsapi.Identity]{Err: Expired("sts:GetCallerIdentity")},
		Answer[awsapi.Identity]{Value: awsapi.Identity{Account: "123456789012"}},
	)
	ctx := context.Background()
	if _, err := sts.CallerIdentity(ctx); kindOf(err) != awsapi.KindExpired {
		t.Fatalf("first answer: %v", err)
	}
	for i := 0; i < 2; i++ {
		if identity, err := sts.CallerIdentity(ctx); err != nil || identity.Account != "123456789012" {
			t.Fatalf("answer %d: %+v, %v", i+2, identity, err)
		}
	}
	if sts.Calls() != 3 {
		t.Fatalf("calls: %d", sts.Calls())
	}
}

func TestEveryKindCanBeScripted(t *testing.T) {
	for want, err := range map[awsapi.Kind]error{
		awsapi.KindDenied:      Denied("op"),
		awsapi.KindExpired:     Expired("op"),
		awsapi.KindThrottled:   Throttled("op"),
		awsapi.KindNeedsMFA:    NeedsMFA("op"),
		awsapi.KindNeedsSignIn: NeedsSignIn("op"),
		awsapi.KindNetwork:     Unreachable("op"),
	} {
		if got := kindOf(err); got != want {
			t.Errorf("%v: got kind %v", err, got)
		}
	}
}

func TestAnEmptyScriptSaysSo(t *testing.T) {
	if _, err := NewSTS().CallerIdentity(context.Background()); err == nil {
		t.Fatal("an empty script answered")
	}
}

func TestEC2RecordsItsFilters(t *testing.T) {
	ec2 := NewEC2(Answer[[]awsapi.Instance]{Value: []awsapi.Instance{{ID: "i-1234567890abcdef0", State: "running"}}})
	filter := awsapi.InstanceFilter{Tags: map[string]string{"Role": "jump-host"}}
	instances, err := ec2.Instances(context.Background(), filter)
	if err != nil || len(instances) != 1 || instances[0].State != "running" {
		t.Fatalf("got %+v, %v", instances, err)
	}
	if filters := ec2.Filters(); len(filters) != 1 || filters[0].Tags["Role"] != "jump-host" {
		t.Fatalf("filters: %+v", filters)
	}
}

func TestSSMRecordsSessions(t *testing.T) {
	ssm := NewSSM(Answer[awsapi.Session]{Value: awsapi.Session{ID: "example-session-1"}})
	request := awsapi.SessionRequest{Target: "i-1234567890abcdef0", Document: "AWS-StartPortForwardingSession",
		Parameters: map[string][]string{"portNumber": {"22"}, "localPortNumber": {"10022"}}}
	session, err := ssm.StartSession(context.Background(), request)
	if err != nil || session.ID != "example-session-1" {
		t.Fatalf("got %+v, %v", session, err)
	}
	if err := ssm.TerminateSession(context.Background(), session.ID); err != nil {
		t.Fatal(err)
	}
	if got := ssm.Requests(); len(got) != 1 || got[0].Target != request.Target {
		t.Fatalf("requests: %+v", got)
	}
	if got := ssm.Terminated(); len(got) != 1 || got[0] != "example-session-1" {
		t.Fatalf("terminated: %v", got)
	}
}

func TestFakesRespectACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sts := NewSTS(Answer[awsapi.Identity]{})
	if _, err := sts.CallerIdentity(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if sts.Calls() != 0 {
		t.Fatal("a cancelled call reached the script")
	}
}

func kindOf(err error) awsapi.Kind {
	var apiErr *awsapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Kind
	}
	return -1
}
