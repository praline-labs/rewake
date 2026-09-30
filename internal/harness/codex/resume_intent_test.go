package codex

import (
	"errors"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

func TestResumeIntentFollowsTheLaunch(t *testing.T) {
	const id = "019A0EE8-873A-7813-BE31-CC02D0E1B4A7"
	for _, check := range []struct {
		args []string
		want gateway.LaunchIntent
	}{
		{nil, gateway.LaunchIntent{}},
		{[]string{"-m", "resume"}, gateway.LaunchIntent{}},
		{[]string{"fork", id}, gateway.LaunchIntent{}},
		{[]string{"resume"}, gateway.LaunchIntent{Resume: true}},
		{[]string{"resume", "--last"}, gateway.LaunchIntent{Resume: true}},
		{[]string{"resume", "my-session-name"}, gateway.LaunchIntent{Resume: true}},
		{[]string{"resume", id}, gateway.LaunchIntent{Resume: true, Thread: strings.ToLower(id)}},
		{[]string{"-c", "model_reasoning_effort=low", "resume", id}, gateway.LaunchIntent{Resume: true, Thread: strings.ToLower(id)}},
		{[]string{"resume", "--last", id}, gateway.LaunchIntent{Resume: true}},
		{[]string{"resume", "--", id}, gateway.LaunchIntent{Resume: true}},
	} {
		if got := resumeIntent(check.args); got != check.want {
			t.Errorf("%q: %+v, want %+v", check.args, got, check.want)
		}
	}
}

// The person's word is not a failure: the message waits for it, and the
// sender reads why.
func TestAnUnintendedConversationKeepsTheMessagePending(t *testing.T) {
	hold := &gateway.HoldError{Hold: sessionstate.DeliveryHold{Reason: sessionstate.HoldUnintended, Expected: "X", Selected: "Y", Detail: "the launch asked to resume X, and the terminal selected Y"}}
	err := reserveRefusal(hold)
	if !errors.Is(err, inbox.ErrNotYet) || !strings.Contains(err.Error(), "resume X") || !strings.Contains(err.Error(), "selected Y") {
		t.Fatal(err)
	}
}

// A launch that asks to resume closes the run's mail before the terminal
// starts, and what the gateway calls before the hold ends opens it.
func TestAResumeLaunchHoldsTheMail(t *testing.T) {
	server := &serverSession{intent: gateway.LaunchIntent{Resume: true, Thread: "x"}, mailbox: t.TempDir(), name: "worker", epoch: "e1"}
	admit, err := server.holdMail()
	if err != nil || admit == nil {
		t.Fatal(err)
	}
	if held, detail, err := sessionstate.MailHeld(server.mailbox, "worker", "e1"); !held || err != nil || !strings.Contains(detail, "resume x") {
		t.Fatalf("%v %q %v", held, detail, err)
	}
	if err := admit(); err != nil {
		t.Fatal(err)
	}
	if held, _, err := sessionstate.MailHeld(server.mailbox, "worker", "e1"); held || err != nil {
		t.Fatal(held, err)
	}
	fresh := &serverSession{mailbox: t.TempDir(), name: "worker", epoch: "e1"}
	if admit, err := fresh.holdMail(); admit != nil || err != nil {
		t.Fatal("a fresh launch holds its mail")
	}
}
