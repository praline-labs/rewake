package mail

// Recorded is what stands for a letter once a result was recorded for it, or
// its status read.
type Recorded struct {
	// Result is the outcome that stands: the result given, or the final one
	// on record, which a later word about the notice does not change.
	Result Result
	// Kept says a final status was on record and stands in place of the
	// result given; nothing was written.
	Kept bool
	// Settled says the letter now stands where its outcome puts it. Only
	// what settles sets it (SettleWith, SettleOnRecord).
	Settled bool
}

// Record writes what delivery did with a letter as its status, unless the
// status on record is final (Status.Final): the agent has the text, or its
// sender took it back, so whatever the harness said about the notice
// afterwards — pending, failed, even delivered — must not undo that, or the
// task is handed out again or its sender told it was refused. A status that
// cannot be read may be final, so nothing is written over it, and the error
// leaves the result to be recorded later.
//
// It moves and removes nothing, so it may run under the mailbox lock or
// without it, when the lock cannot be used (the delivery server's
// lockOrAlone).
func (b Box) Record(id string, result Result) (Recorded, error) {
	current, known, err := ReadStatus(b.dir, b.name, id)
	if err != nil {
		return Recorded{}, err
	}
	if known && current.Final() {
		return Recorded{Result: current.Result(), Kept: true}, nil
	}
	if err := WriteStatus(b.dir, b.name, id, result); err != nil {
		return Recorded{}, err
	}
	return Recorded{Result: result}, nil
}

// SettleWith settles a letter with a result: it records the result (Record),
// and settles the letter by what stands. A final status on record settles it
// as read or withdrawn, whatever the result; otherwise the result does, a
// refused report made readable all the same as delivered. A recording that
// fails settles nothing.
func (l Locked) SettleWith(id string, result Result) (Recorded, error) {
	recorded, err := l.box.Record(id, result)
	if err != nil {
		return Recorded{}, err
	}
	stage := recorded.Result.stage()
	if !recorded.Kept {
		// A result written is placed by its state, whatever withdrawn flag
		// it carries: settling as withdrawn keeps the readable copy for a
		// tombstone, and only Withdraw writes one.
		stage = Result{State: result.State, ReportAvailable: result.ReportAvailable}.stage()
	}
	recorded.Settled = settleLocked(l.box.dir, l.box.name, id, stage)
	return recorded, nil
}

// SettleOnRecord settles a letter by its status as it stands under the lock,
// not as it was read before the lock was taken: a withdrawal, a read or a
// claim that came while the lock was waited for decides instead. decided is
// false, with nothing settled, for a status that holds no outcome yet
// (Status.Decided) or none at all; an error says the status could not be read,
// and may be an outcome.
func (l Locked) SettleOnRecord(id string) (recorded Recorded, decided bool, err error) {
	status, known, err := ReadStatus(l.box.dir, l.box.name, id)
	if err != nil || !known || !status.Decided() {
		return Recorded{}, false, err
	}
	return Recorded{Result: status.Result(), Settled: settleLocked(l.box.dir, l.box.name, id, status.stage())}, true, nil
}

// Decided says the status holds an outcome. Pending is still to be decided,
// and so is held: one on disk and not in a server's memory was held by a run
// that died without saying how it ended — a wrapper killed outright.
func (s Status) Decided() bool { return s.State != Pending && s.State != Held }

// Archives says the status settles its letter into done/: a refusal, neither
// withdrawn — its tombstone stays to be read — nor a report left readable for
// its recipient. Every other outcome leaves a letter in unread/ readable on
// purpose.
func (s Status) Archives() bool { return s.stage() == Failed }

func (s Status) stage() State { return s.Result().stage() }

// stage is where the outcome puts its letter (settleLocked): a withdrawal
// where its tombstone is read, a report made readable after its notice
// failed where a delivered one is.
func (r Result) stage() State {
	switch {
	case r.Withdrawn:
		return withdrawn
	case r.State == Failed && r.ReportAvailable:
		return Delivered
	}
	return r.State
}
