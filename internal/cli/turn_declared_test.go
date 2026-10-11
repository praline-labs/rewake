package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// unproven is the answer of an attempt that cannot show it runs in its mark's
// turn (pending.go, markUnproven).
const unproven = "not marked: this call cannot show it runs in the turn the mark was for"

// eachDeclaration runs a test on a transport that declares its turn ids are
// never reused, and again on one that does not: declared, a later attempt in
// the operation's turn is in that turn; absent, only the first attempt is
// (docs/v2/stage3-steps-adapters.md, S7).
func eachDeclaration(t *testing.T, test func(t *testing.T, reused bool)) {
	for _, reused := range []bool{false, true} {
		name := "declared"
		if reused {
			name = "absent"
		}
		t.Run(name, func(t *testing.T) { test(t, reused) })
	}
}

// An attempt that is neither the first nor shown to run in its operation's
// turn is markUnproven; the first attempt, and one in its own turn, mark.
func TestAnAttemptNotProvenInItsTurnIsMarkUnproven(t *testing.T) {
	dir, self, _ := toolSession(t)
	at := markAt
	for _, test := range []struct {
		scope inbox.AttemptScope
		want  markVerdict
	}{
		{inbox.AttemptScope{}, markUnproven},
		{inbox.AttemptScope{First: true}, markWrite},
		{inbox.AttemptScope{InOwnTurn: true}, markWrite},
	} {
		got, err := judgeMark(dir, self, self.Epoch(), "mark-file", at, test.scope, 0)
		if err != nil || got != test.want {
			t.Fatalf("%+v: %v %v, want %v", test.scope, got, err, test.want)
		}
	}
	// A later start proves the turn ended, whoever asks.
	if got, _ := judgeMark(dir, self, self.Epoch(), "mark-file", at, inbox.AttemptScope{}, at+1); got != markTurnEnded {
		t.Fatalf("a later start: %v", got)
	}
}

// A pending call in turn A expires, A's end is lost, and rewake retry runs in
// turn B: the answer is "not marked", no mark is written, the receipt is
// finished as not made, and the task shows again as owed — on either
// transport, since turn B is not A whatever the declaration says.
func TestALostEndWithARetryFromAnotherTurnMarksNothing(t *testing.T) {
	eachDeclaration(t, func(t *testing.T, reused bool) {
		dir, self, web := toolSession(t)
		readFrom(t, dir, web)
		turnStarted(t, dir, self, markAt-1)
		call, ctx, _, errOut := expiredCall(t, "pending", "still waiting")
		ctx.scope.ticket.TurnsNeverReused = !reused
		if err := handlePending(ctx, call); err == nil {
			t.Fatal("an expired mark succeeded")
		}
		called := ctx.scope.ticket.CalledBoot
		records := unresolved(t, dir, self.Epoch(), ctx.scope.digest)
		if len(records) != 1 || !strings.Contains(errOut.String(), "rewake retry "+records[0].Token) {
			t.Fatalf("records %+v, said %s", records, errOut)
		}
		// No end of turn A is on record: it was lost.
		tool := newToolCaller(t)
		tool.turn, tool.reused = "turn-2", reused
		retried := tool.run("retry", records[0].Token)
		if retried.code != ExitFailed || !strings.Contains(retried.errOut, unproven) {
			t.Fatalf("a retry from another turn: %+v", retried)
		}
		if _, held, err := markWithin(dir, "api", self.Epoch(), called-1, called+1); held || err != nil {
			t.Fatalf("a retry from another turn marked turn A: %v %v", held, err)
		}
		if left := unresolved(t, dir, self.Epoch(), ctx.scope.digest); len(left) != 0 {
			t.Fatalf("the receipt is not finished: %+v", left)
		}
		again := tool.run("retry", records[0].Token)
		if again.code != ExitFailed || !strings.Contains(again.errOut, unproven) {
			t.Fatalf("the finished receipt's answer: %+v", again)
		}
		if code, out, errOut := run("inbox", "--owed"); code != ExitOK || !strings.Contains(out, "rerun") {
			t.Fatalf("the task is not shown again: %d %s %s", code, out, errOut)
		}
	})
}

// The declaration is both ends': the receipt's, written by the attempt that
// made the record, and the retrying ticket's. A record made without it is not
// made declared by a later ticket that carries it, and a declared record does
// not lend it to a ticket without it; either way the retry in the same turn
// cannot show it runs there, and the mark is finished as not made.
func TestADeclarationOnOneEndOnlyProvesNoTurn(t *testing.T) {
	for _, test := range []struct {
		name          string
		made, retried bool // whether each end withholds the declaration
	}{
		{"record without, ticket with", true, false},
		{"record with, ticket without", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir, self, web := toolSession(t)
			readFrom(t, dir, web)
			turnStarted(t, dir, self, markAt-1)
			waiter := filepath.Join(state.AwaitingPath(dir, "api"), self.Epoch(), "web")
			if err := os.Chmod(waiter, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(waiter, 0o600) })
			tool := newToolCaller(t)
			tool.reused = test.made
			blind := tool.run("pending", "the suite is running")
			if blind.code != ExitFailed || !strings.Contains(blind.errOut, "rewake retry ") {
				t.Fatalf("a mark past a waiter it could not read: %+v", blind)
			}
			token := blind.errOut[strings.Index(blind.errOut, "rewake retry ")+len("rewake retry "):][:24]
			if err := os.Chmod(waiter, 0o600); err != nil {
				t.Fatal(err)
			}
			tool.reused = test.retried
			retried := tool.run("retry", token)
			if retried.code != ExitFailed || !strings.Contains(retried.errOut, unproven) {
				t.Fatalf("a retry the declaration of one end only let in: %+v", retried)
			}
			if again := tool.run("retry", token); again.code != ExitFailed || !strings.Contains(again.errOut, unproven) {
				t.Fatalf("the finished receipt's answer: %+v", again)
			}
		})
	}
}

// A pending call in turn A expires and A's end is lost; the next turn's start
// is on record, as the host records what a TurnBoundary reports, tool or none
// (internal/wrap/turn_start_test.go). A retry from A's own turn, which would
// mark without that start, is answered that the turn ended first and marks
// nothing: the start alone is the evidence where no end is.
func TestALostEndWithALaterStartOnRecordMarksNothing(t *testing.T) {
	for _, later := range []bool{false, true} {
		t.Run(map[bool]string{false: "no later start", true: "a later start"}[later], func(t *testing.T) {
			dir, self, web := toolSession(t)
			readFrom(t, dir, web)
			turnStarted(t, dir, self, markAt-1)
			call, ctx, _, errOut := expiredCall(t, "pending", "still waiting")
			if err := handlePending(ctx, call); err == nil {
				t.Fatal("an expired mark succeeded")
			}
			called := ctx.scope.ticket.CalledBoot
			records := unresolved(t, dir, self.Epoch(), ctx.scope.digest)
			if len(records) != 1 || !strings.Contains(errOut.String(), "rewake retry "+records[0].Token) {
				t.Fatalf("records %+v, said %s", records, errOut)
			}
			if later {
				turnStarted(t, dir, self, boottime.Now())
			}
			retried := newToolCaller(t).run("retry", records[0].Token)
			_, held, err := markWithin(dir, "api", self.Epoch(), called-1, called+1)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case !later && (retried.code != ExitOK || !held):
				t.Fatalf("a retry in its own turn with no later start: %+v, marked %t", retried, held)
			case later && (retried.code != ExitFailed || !strings.Contains(retried.errOut, "the turn ended first") || held):
				t.Fatalf("a retry after a later start: %+v, marked %t", retried, held)
			}
		})
	}
}
