package host

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func outcomeOf(err error) mail.DecisionOutcome {
	var refusal *mail.DecisionError
	if errors.As(err, &refusal) {
		return refusal.Outcome
	}
	return ""
}

// The wrapper decides only for its own run, and takes no decision once the
// run has begun to end.
func TestTheWrapperDecidesOnlyForItsOwnRunWhileItLives(t *testing.T) {
	life := &lifetime{keep: func() {}}
	handler := decideHandler(t.TempDir(), "web", "3.4", life)
	ask := mail.DecideRequest{Sender: "api", Report: "r1", Choice: mail.ChoiceArrived, Expect: mail.Subject{Recipient: "web", RecipientRun: "3.5"}}
	if _, err := handler(context.Background(), ask); outcomeOf(err) != mail.DecisionUnauthorized {
		t.Fatalf("a report to another run of the name answered %v", err)
	}
	ask.Expect.RecipientRun, ask.Expect.Recipient = "3.4", "ops"
	if _, err := handler(context.Background(), ask); outcomeOf(err) != mail.DecisionUnauthorized {
		t.Fatalf("a report to another session answered %v", err)
	}
	life.closeAdmission()
	ask.Expect.Recipient = "web"
	if _, err := handler(context.Background(), ask); outcomeOf(err) != mail.DecisionBusy {
		t.Fatalf("an ending run answered %v", err)
	}
}

// A transaction admitted before the run's end keeps the run's hold on the
// lease until it finishes; none is admitted once the end began.
func TestAnAdmittedTransactionKeepsTheLeaseUntilItFinishes(t *testing.T) {
	released := make(chan struct{})
	life := &lifetime{keep: func() { close(released) }}
	if !life.admit() {
		t.Fatal("a living run admitted nothing")
	}
	ended := make(chan struct{})
	go func() { life.end(); close(ended) }()
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("the end waited past its bound")
	}
	if life.admit() {
		t.Fatal("an ended run admitted a transaction")
	}
	select {
	case <-released:
		t.Fatal("the hold went while a transaction ran")
	default:
	}
	life.dismiss()
	select {
	case <-released:
	case <-time.After(10 * time.Second):
		t.Fatal("the hold did not go after the transaction")
	}
}
