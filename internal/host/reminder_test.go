package host

import (
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// Only a turn that finished with an answer is looked at for the reminder: a
// failed, refused or stopped one keeps the rule that a session is woken only
// by new mail (docs/turn-outcomes.md), and an end of no known time has no
// window.
func TestOnlyAFinishedEndReachesTheReminder(t *testing.T) {
	ends := make(chan int64, 8)
	for _, end := range []adapter.TurnEnd{
		{Reason: adapter.Failed, Answer: "broke", Ended: 1},
		{Reason: adapter.Refused, Answer: "no", Ended: 2},
		{Reason: adapter.Aborted, Answer: "stopped", Ended: 3},
		{Reason: adapter.Answered, Answer: " ", Ended: 4},
		{Reason: adapter.Answered, Answer: "done"},
		{Reason: adapter.Answered, Answer: "done", Ended: 6},
	} {
		finished(ends, end)
	}
	close(ends)
	var got []int64
	for ended := range ends {
		got = append(got, ended)
	}
	if len(got) != 1 || got[0] != 6 {
		t.Fatalf("ends passed on %v, want only the finished one", got)
	}
	finished(nil, adapter.TurnEnd{Reason: adapter.Answered, Answer: "done", Ended: 7})
}
