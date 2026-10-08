package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
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
