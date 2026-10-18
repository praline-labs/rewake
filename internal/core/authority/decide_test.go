package authority

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

var subject = mail.Subject{Sender: "api", SenderRun: "1.2", Journal: "end", Report: "r1", Kind: mail.Finished, Recipient: "web", RecipientRun: "3.4"}

// answerAll is a handler that decides whatever it is asked, as the host's
// would when the transaction goes through.
func answerAll(_ context.Context, ask mail.DecideRequest) (mail.DecideAnswer, error) {
	return mail.DecideAnswer{
		Decision: mail.Decision{ID: "d1", Choice: ask.Choice, Subject: ask.Expect, By: mail.DecidedBy{Kind: mail.ByRecipient, Name: "web", Run: "3.4", Verified: true}},
		Path:     "/decisions/r1", Before: true, Barrier: errors.New("stopped on another cause"), Owed: []string{mail.ToldMain},
	}, nil
}

func runOf(t *testing.T, pid int) Run {
	t.Helper()
	start, err := proc.StartTime(pid)
	if err != nil {
		t.Fatal(err)
	}
	boot, err := registry.CurrentBoot()
	if err != nil {
		t.Fatal(err)
	}
	run, ok := RunOf(registry.RunEpoch(pid, start, boot))
	if !ok {
		t.Fatal("the run does not parse")
	}
	return run
}

// listeners numbers the addresses a test binds, two of which may be its.
var listeners atomic.Int64

func listenDecisions(t *testing.T, handler DecideHandler) (*Authority, string) {
	t.Helper()
	path := fmt.Sprintf("@rewake-test/%s/%d/%d", t.Name(), os.Getpid(), listeners.Add(1))
	authority, err := Listen(path, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if handler != nil {
		authority.HandleDecide(handler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go authority.Serve(ctx)
	t.Cleanup(func() { cancel(); authority.Close() })
	return authority, path
}

func ask(choice string) mail.DecideRequest {
	return mail.DecideRequest{Sender: "api", Report: "r1", Choice: choice, Expect: subject}
}

func outcomeOf(err error) mail.DecisionOutcome {
	var refusal *mail.DecisionError
	if errors.As(err, &refusal) {
		return refusal.Outcome
	}
	return ""
}

// A wrapper answers a decision through its host's handler, and the answer
// comes back whole but for the record's observations.
func TestADecisionIsMadeByTheWrapperAsked(t *testing.T) {
	var asked mail.DecideRequest
	_, path := listenDecisions(t, func(ctx context.Context, request mail.DecideRequest) (mail.DecideAnswer, error) {
		asked = request
		return answerAll(ctx, request)
	})
	answer, err := Decide(path, runOf(t, os.Getpid()), ask(mail.ChoiceNotArrived), "/decisions/r1")
	if err != nil {
		t.Fatal(err)
	}
	if asked.Sender != "api" || asked.Report != "r1" || asked.Choice != mail.ChoiceNotArrived || asked.Expect != subject || asked.By != (mail.DecidedBy{}) {
		t.Fatalf("the handler was asked %+v", asked)
	}
	d := answer.Decision
	if d.ID != "d1" || d.Choice != mail.ChoiceNotArrived || d.Subject != subject || !d.By.Verified || answer.Path != "/decisions/r1" ||
		!answer.Before || answer.Barrier == nil || len(answer.Owed) != 1 {
		t.Fatalf("answered %+v", answer)
	}
}

// A refusal comes back as the transaction answered it: its outcome, whether
// the record is on disk, and its path.
func TestARefusedDecisionComesBackAsRefused(t *testing.T) {
	_, path := listenDecisions(t, func(context.Context, mail.DecideRequest) (mail.DecideAnswer, error) {
		return mail.DecideAnswer{}, &mail.DecisionError{Outcome: mail.DecisionEffectFailed, Recorded: true, Path: "/decisions/r1", Err: errors.New("the resend did not go")}
	})
	_, err := Decide(path, runOf(t, os.Getpid()), ask(mail.ChoiceNotArrived), "")
	var refusal *mail.DecisionError
	if !errors.As(err, &refusal) || refusal.Outcome != mail.DecisionEffectFailed || !refusal.Recorded || refusal.Path != "/decisions/r1" {
		t.Fatalf("answered %v", err)
	}
}

// A wrapper takes a bounded number of decisions at once and refuses the rest
// busy; one with no handler yet, or closing, takes none; and a decision taken
// before it closed is answered.
func TestAWrapperTakesBoundedDecisionsAndNoneOnceClosing(t *testing.T) {
	if _, err := Decide(func() string { _, p := listenDecisions(t, nil); return p }(), runOf(t, os.Getpid()), ask(mail.ChoiceArrived), ""); outcomeOf(err) != mail.DecisionBusy {
		t.Fatalf("a wrapper with no handler answered %v", err)
	}
	entered, release := make(chan struct{}, maxDecisions), make(chan struct{})
	authority, path := listenDecisions(t, func(ctx context.Context, request mail.DecideRequest) (mail.DecideAnswer, error) {
		entered <- struct{}{}
		<-release
		return answerAll(ctx, request)
	})
	run := runOf(t, os.Getpid())
	var wg sync.WaitGroup
	answers := make(chan error, maxDecisions)
	for range maxDecisions {
		wg.Go(func() {
			_, err := Decide(path, run, ask(mail.ChoiceArrived), "")
			answers <- err
		})
	}
	for range maxDecisions {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("the decisions were not taken")
		}
	}
	if _, err := Decide(path, run, ask(mail.ChoiceArrived), ""); outcomeOf(err) != mail.DecisionBusy {
		t.Fatalf("a decision past the bound answered %v", err)
	}
	authority.Close()
	if _, err := Decide(path, run, ask(mail.ChoiceArrived), ""); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("a closed wrapper answered %v", err)
	}
	close(release)
	wg.Wait()
	close(answers)
	for err := range answers {
		if err != nil {
			t.Fatalf("a decision taken before the close answered %v", err)
		}
	}
}

// The asker believes only the wrapper of the run it asked: one of another
// boot, one it does not run below and a listener that is not that run's
// process are refused, the first two before anything is sent.
func TestTheAskerBelievesOnlyTheRecipientsWrapper(t *testing.T) {
	_, path := listenDecisions(t, answerAll)
	run := runOf(t, os.Getpid())
	other := run
	other.Boot = "another-boot"
	if _, err := Decide(path, other, ask(mail.ChoiceArrived), ""); outcomeOf(err) != mail.DecisionUnauthorized {
		t.Fatalf("a run of another boot answered %v", err)
	}
	// This listener at the address of this process's parent's run: the
	// asker runs below that run, and the address is served by another.
	if _, err := Decide(path, runOf(t, os.Getppid()), ask(mail.ChoiceArrived), ""); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("another process at the address answered %v", err)
	}
	address := "@rewake-test/" + t.Name() + "/helper/" + strconv.Itoa(os.Getpid())
	helper := startHelper(t, helperDecisions+"="+address)
	_, err := Decide(address, runOf(t, helper.Process.Pid), ask(mail.ChoiceArrived), "")
	if outcomeOf(err) != mail.DecisionUnauthorized || !strings.Contains(err.Error(), "does not run below") {
		t.Fatalf("a wrapper the asker does not run below answered %v", err)
	}
}

// The wrapper takes a decision only from a command below it: a process
// beside it, asking straight, is refused and nothing is decided.
func TestAWrapperTakesADecisionOnlyFromBelow(t *testing.T) {
	address := "@rewake-test/" + t.Name() + "/" + strconv.Itoa(os.Getpid())
	startHelper(t, helperDecisions+"="+address)
	conn, err := dial(address)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	request := ask(mail.ChoiceArrived)
	answer, err := exchange(conn, requestOf(request))
	if err != nil || answer.Decided == nil || answer.Decided.Outcome != mail.DecisionUnauthorized || answer.Decided.ID != "" {
		t.Fatalf("a process beside the wrapper was answered %+v, %v", answer.Decided, err)
	}
	if !strings.Contains(answer.Decided.Error, "only a command this session runs") {
		t.Fatalf("refused for another reason: %s", answer.Decided.Error)
	}
}

func requestOf(ask mail.DecideRequest) request {
	return request{Op: opDecide, Decide: &decideAsk{Sender: ask.Sender, Report: ask.Report, Choice: ask.Choice, Expect: ask.Expect}}
}
