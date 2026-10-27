package host

import (
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// The delivery server hears the end of a turn its publication called finished,
// and of no other: the wrapper forwards the core's word on the end rather than
// judge the adapter's (cli's completion tests hold what is called finished).
func TestOnlyAFinishedEndReachesTheReminder(t *testing.T) {
	ends := make(chan int64, 8)
	for _, outcome := range []mail.Outcome{
		{Held: "asked to confirm"},
		{Interim: true},
		{},
		{Finished: 6},
	} {
		finished(ends, outcome)
	}
	close(ends)
	var got []int64
	for ended := range ends {
		got = append(got, ended)
	}
	if len(got) != 1 || got[0] != 6 {
		t.Fatalf("ends passed on %v, want only the finished one", got)
	}
	finished(nil, mail.Outcome{Finished: 7})
}
