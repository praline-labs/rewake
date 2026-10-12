package decisionmodel

import "slices"

// The values of the dimensions, as the recovery document's table names them
// (docs/v2/stage3-decision-recovery.md#the-tests).
const (
	Landed    = "landed"
	NotLanded = "not-landed"

	MarkUnreadable      = "mark-unreadable"
	MailboxUnsearchable = "mailbox-unsearchable"
	EffectMet           = "effect-met"

	StaysUnknown    = "stays-unknown"
	ReturnsWritten  = "returns-published"
	ReturnsAbsent   = "returns-absent"
	ReturnsBeforeD  = "returns-before-decision"
	RetiredProof    = "retired-proof"
	Arrived         = "arrived"
	NotArrived      = "not-arrived"
	None            = "none"
	SameWords       = "same-words"
	OppositeWords   = "opposite-words"
	ByRecipient     = "recipient-verified"
	ByPersonRLive   = "person-r-alive"
	ByPersonREnded  = "person-r-ended"
	RReads          = "r-reads"
	SweepPromotes   = "sweep-promotes"
	NewRunDropsMark = "new-run-drops-marks"
	SenderAlive     = "s-alive"
	SenderEndsFirst = "s-ended-before-effects"
	SenderEndsAfter = "s-ended-after-effects"
)

// The resend's truth and evidence, the pairs the model allows, and its
// lifetime points.
const (
	NoneThenPublished   = "none-then-published"
	AbsentThenPublished = "absent-then-published"
	UnknownIntentLanded = "unknown-intent-landed"
	UnknownIntentNot    = "unknown-intent-not"
	UnknownLetterLanded = "unknown-letter-landed"
	UnknownLetterNot    = "unknown-letter-not"
	UnknownMarkLanded   = "unknown-mark-landed"
	UnknownMarkNot      = "unknown-mark-not"
	MootNoWrite         = "moot-no-write"

	RAlive          = "r-alive"
	REndsBeforeD    = "r-ends-before-decision"
	REndsBeforeLive = "r-ends-before-liveness"
	REndsInSection  = "r-ends-check-to-evidence"
	REndsToIntent   = "r-ends-evidence-to-intent"
	REndsToMark     = "r-ends-letter-to-mark"
	REndsToSave     = "r-ends-letter-to-save"
	REndsToTail     = "r-ends-save-to-tail"
	REndsUnknown    = "r-ends-after-unknown"
)

// The rest of the mailbox, the second report in J, and the generations.
const (
	Nothing                = "nothing-else"
	ChildUnknownEvidence   = "child-unknown-settled-by-evidence"
	ChildUnknownDecision   = "child-unknown-settled-by-decision"
	ChildUnknownLeft       = "child-unknown-left"
	UnrelatedBeforeSettled = "unrelated-before-settled"
	UnrelatedBeforeLeft    = "unrelated-before-left"
	UnrelatedAfterSettled  = "unrelated-after-settled"
	UnrelatedAfterLeft     = "unrelated-after-left"

	SecondUndecided         = "second-undecided"
	SecondDecidedArrived    = "second-decided-arrived"
	SecondDecidedNotArrived = "second-decided-not-arrived"
	OneGeneration           = "one"
	TwoLandedArrived        = "two-landed-arrived"
	TwoLandedNotArrived     = "two-landed-not-arrived"
	TwoNotLandedArrived     = "two-not-landed-arrived"
	TwoNotLandedNotArrived  = "two-not-landed-not-arrived"
	ChainOfForty            = "chain-of-40"
	unknownPrefix           = "unknown-"
)

// Partitions are the four groups of the model's schedules, each enumerated as
// a product apart from the others (the manifest names what each holds fixed).
func Partitions() []Partition {
	return []Partition{decision(), resend(), mailbox(), generations()}
}

// decision is one decision on R: its truth and evidence, the choice and a
// second command, who decides, r's activity and the sender's life.
func decision() Partition {
	return Partition{
		Name: "decision",
		Dimensions: []Dimension{
			{"truth", values(Landed, NotLanded)},
			{"at-decision", values(MarkUnreadable, MailboxUnsearchable, EffectMet)},
			{"later", values(StaysUnknown, ReturnsWritten, ReturnsAbsent, ReturnsBeforeD, RetiredProof)},
			{"choice", values(Arrived, NotArrived)},
			{"second", values(None, SameWords, OppositeWords)},
			{"decider", values(ByRecipient, ByPersonRLive, ByPersonREnded)},
			{"activity", values(None, RReads, SweepPromotes, NewRunDropsMark)},
			{"sender", values(SenderAlive, SenderEndsFirst, SenderEndsAfter)},
		},
		Feasible: func(w World) bool {
			truth, later, activity := w.Value("truth"), w.Value("later"), w.Value("activity")
			rEnded := w.Value("decider") == ByPersonREnded
			written := later != ReturnsBeforeD
			switch {
			// Evidence agrees with the truth (the excluded combinations).
			case later == ReturnsWritten && truth != Landed, later == ReturnsAbsent && truth != NotLanded:
				return false
			// A proof is retired only once r ended and the next run dropped its marks.
			case later == RetiredProof && (!rEnded || activity != NewRunDropsMark):
				return false
			// Evidence before D's write leaves no decision: nothing to repeat, no effect.
			case !written && (w.Value("second") != None || w.Value("sender") != SenderAlive):
				return false
			// Only a landed letter is read or swept; a mark that does not read
			// keeps the sweep from promoting it.
			case (activity == RReads || activity == SweepPromotes) && truth != Landed:
				return false
			case activity == SweepPromotes && w.Value("at-decision") == MarkUnreadable:
				return false
			// A new run of the name means r ended.
			case activity == NewRunDropsMark && !rEnded:
				return false
			}
			return true
		},
	}
}

// resend is R decided "did not arrive" and J' running: the truth and evidence
// of R', a decision on it when unknown, r's end at each point of J”s path and
// the sender's life.
func resend() Partition {
	return Partition{
		Name: "resend",
		Dimensions: []Dimension{
			{"r-prime", values(NoneThenPublished, AbsentThenPublished, UnknownIntentLanded, UnknownIntentNot,
				UnknownLetterLanded, UnknownLetterNot, UnknownMarkLanded, UnknownMarkNot, MootNoWrite)},
			{"second-choice", values(None, Arrived, NotArrived)},
			{"r-life", values(RAlive, REndsBeforeD, REndsBeforeLive, REndsInSection, REndsToIntent,
				REndsToMark, REndsToSave, REndsToTail, REndsUnknown)},
			{"sender", values(SenderAlive, SenderEndsFirst, SenderEndsAfter)},
		},
		Feasible: func(w World) bool {
			prime, life := w.Value("r-prime"), w.Value("r-life")
			unknown := isUnknown(prime)
			if (w.Value("second-choice") != None) != unknown {
				return false
			}
			switch life {
			case REndsBeforeD, REndsBeforeLive:
				// The first attempt reads r ended: moot, nothing written.
				return prime == MootNoWrite
			case REndsInSection:
				// Admitted live: the first write is allowed, and lands.
				return prime == NoneThenPublished
			case REndsToIntent, REndsToMark, REndsToSave, REndsToTail:
				return prime == NoneThenPublished || prime == AbsentThenPublished
			case REndsUnknown:
				return unknown
			}
			return prime != MootNoWrite
		},
	}
}

// mailbox is the rest of the mailbox around a ready J: an unknown child, an
// unrelated journal that does not read in either order, and a second report
// of J, each settled or left.
func mailbox() Partition {
	return Partition{
		Name: "mailbox",
		Dimensions: []Dimension{
			{"rest", values(Nothing, ChildUnknownEvidence, ChildUnknownDecision, ChildUnknownLeft,
				UnrelatedBeforeSettled, UnrelatedBeforeLeft, UnrelatedAfterSettled, UnrelatedAfterLeft)},
			{"second-report", values(None, SecondUndecided, SecondDecidedArrived, SecondDecidedNotArrived)},
			{"choice", values(Arrived, NotArrived)},
		},
		Feasible: func(w World) bool {
			child := slices.Contains([]string{ChildUnknownEvidence, ChildUnknownDecision, ChildUnknownLeft}, w.Value("rest"))
			return !child || w.Value("choice") == NotArrived
		},
	}
}

// generations is the chain of decisions: one, two with every combination of
// the truth of R' and the second choice, and forty.
func generations() Partition {
	return Partition{
		Name: "generations",
		Dimensions: []Dimension{
			{"generations", values(OneGeneration, TwoLandedArrived, TwoLandedNotArrived, TwoNotLandedArrived,
				TwoNotLandedNotArrived, ChainOfForty)},
			{"truth", values(Landed, NotLanded)},
		},
		Feasible: func(World) bool { return true },
	}
}

func isUnknown(prime string) bool {
	return len(prime) > len(unknownPrefix) && prime[:len(unknownPrefix)] == unknownPrefix
}
