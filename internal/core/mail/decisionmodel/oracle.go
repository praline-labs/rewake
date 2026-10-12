package decisionmodel

// The expected outcome of a world (docs/v2/stage3-decision-model.md#the-expected-outcome-of-a-world),
// derived from its dimensions alone: the truth of each copy and the choices
// made, never what any record says.

// Copy is one copy of the report, R first, then each resend: whether it
// landed in the world, and the choice made on it when its outcome was unknown
// and decided — empty when it was not decided.
type Copy struct {
	Landed  bool
	Decided string
}

// Outcome is what the model predicts of a chain of copies.
type Outcome struct {
	// Copies is how many copies exist: R, and one more for each "did not
	// arrive".
	Copies int
	// Landings is how many letters were written for r: one per copy that
	// landed, counted from the write log, whatever was read or swept since.
	Landings int
	// Duplicates is how many choices of "did not arrive" were wrong.
	Duplicates int
	// Lost says the chain ends on a wrong "arrived" with no copy landed.
	Lost bool
	// Right says every choice matched its copy's truth.
	Right bool
}

// Expect predicts a chain. A copy exists only when the one before it was
// decided "did not arrive"; the copies past the last that exists are not
// read.
func Expect(chain []Copy) Outcome {
	out := Outcome{Right: true}
	for i, c := range chain {
		if i > 0 && chain[i-1].Decided != NotArrived {
			break
		}
		out.Copies++
		if c.Landed {
			out.Landings++
		}
		switch {
		case c.Decided == NotArrived && c.Landed:
			out.Duplicates++
			out.Right = false
		case c.Decided == Arrived && !c.Landed:
			out.Right = false
		}
	}
	last := chain[out.Copies-1]
	out.Lost = last.Decided == Arrived && out.Landings == 0
	return out
}

// WaitsCleared says whether R's waits are gone: only J's tail clears them,
// and it runs once every step of J is published, moot or closed and the
// mailbox is admitted — however ready J is, an unknown anywhere keeps them.
func WaitsCleared(stepsClosed, admitted bool) bool { return stepsClosed && admitted }

// Run is one run a decision may have to tell.
type Run struct {
	Name, Epoch string
	Alive       bool
}

// Told are the runs a decision owes a note: the recipient run and main's run,
// each unless it decided verified, one note when they are one run, and none
// to a run that ended.
func Told(recipient, main Run, verifiedBy string, haveMain bool) []Run {
	var told []Run
	add := func(r Run) {
		if !r.Alive || verifiedBy != "" && r.Epoch == verifiedBy {
			return
		}
		for _, t := range told {
			if t.Epoch == r.Epoch && t.Name == r.Name {
				return
			}
		}
		told = append(told, r)
	}
	add(recipient)
	if haveMain {
		add(main)
	}
	return told
}
