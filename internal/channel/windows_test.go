package channel

import (
	"testing"
	"time"
)

// The windows (docs/mail-bridge-channel.md#notices): one notice per key per
// ten minutes and six an hour to one recipient, counting landed notices
// only. Every history of up to seven landings, each of two keys at an age on
// either side of both windows' edges, against the rule's own count.

func TestTheWindowsOverEveryLandingHistory(t *testing.T) {
	const now = int64(10 * time.Hour)
	ages := []time.Duration{0, KeyWindow - 1, KeyWindow, HourWindow - 1, HourWindow}
	var options []landing
	for _, key := range []string{"a", "b"} {
		for _, age := range ages {
			options = append(options, landing{Key: key, At: now - int64(age)})
		}
	}
	to := Recipient{Role: ToMain, Name: "m", Epoch: "1"}
	cases := 0
	var walk func(from int, history []landing)
	walk = func(from int, history []landing) {
		n := Notices{Landings: map[string][]landing{to.id(): history}}
		sameKey, recent := false, 0
		for _, landed := range history {
			age := time.Duration(now - landed.At)
			sameKey = sameKey || landed.Key == "a" && age < KeyWindow
			if age < HourWindow {
				recent++
			}
		}
		if got, want := n.allowed(to, "a", now), !sameKey && recent < 6; got != want {
			t.Fatalf("history %v: allowed %v, want %v", history, got, want)
		}
		cases++
		if len(history) == 7 {
			return
		}
		for index := from; index < len(options); index++ {
			walk(index, append(history[:len(history):len(history)], options[index]))
		}
	}
	walk(0, nil)
	t.Logf("%d landing histories", cases)
}
