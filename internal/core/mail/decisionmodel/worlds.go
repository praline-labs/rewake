// Package decisionmodel holds four decision schedules and the outcomes
// predicted from each report's actual landing and the operator's choice.
package decisionmodel

// Values shared by the fixed decision worlds and the focused regressions.
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
	ByPerson        = "person"
	RLivesOn        = "r-lives-on"
	REndsAfterD     = "r-ends-after-decision"
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
	TwoNotLandedNotArrived  = "two-not-landed-not-arrived"
)

// World is a fixed schedule for one decision path, not a generated product
// of dimensions. Values are named so a schedule is readable at its definition.
type World struct {
	Name, Partition string
	Values          map[string]string
}

// Value returns a named part of the schedule.
func (w World) Value(name string) string { return w.Values[name] }

// Worlds keeps one representative of each recovery path. Focused regression
// tests hold the particular bugs outside these four durability schedules.
func Worlds() []World {
	return []World{
		{
			Name: "arrived with an unreadable mark", Partition: "decision",
			Values: map[string]string{
				"truth": Landed, "at-decision": MarkUnreadable, "later": StaysUnknown,
				"choice": Arrived, "second": SameWords, "decider": ByRecipient,
				"r-at-decision": RAlive, "r-after-decision": RLivesOn,
				"activity": None, "sender": SenderAlive,
			},
		},
		{
			Name: "resend across recipient end", Partition: "resend",
			Values: map[string]string{
				"r-prime": NoneThenPublished, "second-choice": None, "decider": ByPerson,
				"r-life": REndsToSave, "sender": SenderAlive,
			},
		},
		{
			Name: "an unrelated unknown keeps the waits", Partition: "mailbox",
			Values: map[string]string{
				"rest": UnrelatedBeforeLeft, "second-report": SecondDecidedArrived,
				"choice": NotArrived,
			},
		},
		{
			Name: "two generations resend once", Partition: "generations",
			Values: map[string]string{
				"generations": TwoNotLandedNotArrived, "truth": NotLanded,
			},
		},
	}
}
