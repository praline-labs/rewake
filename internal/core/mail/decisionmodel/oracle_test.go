package decisionmodel

import "testing"

// The outcomes the model's table of two generations gives, with x the
// landing of R.
func TestTwoGenerationsCountByTheirTruth(t *testing.T) {
	for _, x := range []bool{true, false} {
		base := 0
		if x {
			base = 1
		}
		cases := []struct {
			prime   bool
			second  string
			landing int
			right   bool
		}{
			{true, Arrived, base + 1, true},
			{true, NotArrived, base + 2, false},
			{false, Arrived, base, false},
			{false, NotArrived, base + 1, true},
		}
		for _, c := range cases {
			chain := []Copy{{Landed: x, Decided: NotArrived}, {Landed: c.prime, Decided: c.second}}
			if c.second == NotArrived {
				chain = append(chain, Copy{Landed: true})
			}
			out := Expect(chain)
			right := c.right && !x
			if out.Landings != c.landing || out.Right != right {
				t.Errorf("x=%v R' %v %s: %+v, want %d landings, right %v", x, c.prime, c.second, out, c.landing, right)
			}
		}
	}
	if out := Expect([]Copy{{Landed: false, Decided: Arrived}}); !out.Lost || out.Copies != 1 {
		t.Errorf("a wrong arrived over nothing landed: %+v", out)
	}
}

// A note goes to each run that did not decide verified and still lives, once
// when the recipient is main.
func TestNotesGoToThoseWhoDidNotDecide(t *testing.T) {
	lead := Run{Name: "lead", Epoch: "r", Alive: true}
	main := Run{Name: "boss", Epoch: "m", Alive: true}
	if got := Told(lead, main, "r", true); len(got) != 1 || got[0] != main {
		t.Errorf("verified by r: %v", got)
	}
	if got := Told(lead, lead, "", true); len(got) != 1 {
		t.Errorf("the person, r is main: %v", got)
	}
	if got := Told(Run{Name: "lead", Epoch: "r"}, main, "", false); len(got) != 0 {
		t.Errorf("r ended, no main: %v", got)
	}
}
